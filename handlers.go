package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api      *tgbotapi.BotAPI
	db       *DB
	cfg      *Config
	sessions *SessionStore
}

func NewBot(api *tgbotapi.BotAPI, db *DB, cfg *Config) *Bot {
	return &Bot{
		api:      api,
		db:       db,
		cfg:      cfg,
		sessions: NewSessionStore(),
	}
}

// ── Dispatch ──────────────────────────────────────────────────────────────────

func (b *Bot) HandleUpdate(u tgbotapi.Update) {
	switch {
	case u.CallbackQuery != nil:
		b.handleCallback(u.CallbackQuery)
	case u.Message != nil:
		b.handleMessage(u.Message)
	}
}

// ── Message handler ───────────────────────────────────────────────────────────

func (b *Bot) handleMessage(m *tgbotapi.Message) {
	uid := m.From.ID

	if m.IsCommand() {
		switch m.Command() {
		case "start", "help":
			b.cmdHelp(m)
		case "send":
			b.cmdSend(m)
		case "begin":
			if b.requireAdmin(m) {
				b.cmdBegin(m)
			}
		case "done":
			if b.requireAdmin(m) {
				b.cmdDone(m)
			}
		case "cancel":
			if b.requireAdmin(m) {
				b.cmdCancel(m)
			}
		case "listfiles":
			if b.requireAdmin(m) {
				b.cmdListFiles(m, 0)
			}
		case "delete":
			if b.requireAdmin(m) {
				b.cmdDelete(m)
			}
		case "stats":
			if b.requireAdmin(m) {
				b.cmdStats(m)
			}
		default:
			if b.cfg.IsAdmin(uid) {
				b.reply(m, "Unknown command\\. Use /help\\.")
			}
		}
		return
	}

	// File received from admin
	if b.cfg.IsAdmin(uid) && hasFile(m) {
		b.handleFileFromAdmin(m)
		return
	}
}

// ── /help ─────────────────────────────────────────────────────────────────────

func (b *Bot) cmdHelp(m *tgbotapi.Message) {
	// Deep link: /start <code> → deliver files directly
	if arg := strings.TrimSpace(m.CommandArguments()); arg != "" {
		b.cmdSend(m)
		return
	}

	if b.cfg.IsAdmin(m.From.ID) {
		b.reply(m,
			"*FileStore Bot — Admin*\n\n"+
				"*Creating a bundle*\n"+
				"`/begin` — start a new bundle\n"+
				"_Send as many files as you want_\n"+
				"`/done` — save the bundle and get a code\n"+
				"`/cancel` — discard the current bundle\n\n"+
				"*Managing files*\n"+
				"`/listfiles` — browse & delete bundles\n"+
				"`/delete <code> [code …]` — delete by code\\(s\\)\n"+
				"`/stats` — storage summary\n\n"+
				"*Retrieving*\n"+
				"`/send <code>` — retrieve a bundle by code",
		)
	} else {
		b.reply(m,
			"*FileStore Bot*\n\n"+
				"Use `/send <code>` to retrieve files\\.\n"+
				"Example: `/send abcd56789`",
		)
	}
}

// ── /begin ────────────────────────────────────────────────────────────────────

func (b *Bot) cmdBegin(m *tgbotapi.Message) {
	if !b.sessions.Begin(m.From.ID) {
		count := b.sessions.Count(m.From.ID)
		b.reply(m, fmt.Sprintf(
			"⚠️ You already have an active bundle with *%d* file%s queued\\.\n"+
				"Send more files, `/done` to save, or `/cancel` to discard\\.",
			count, pluralS(count),
		))
		return
	}
	b.reply(m,
		"📦 *Bundle started\\!*\n\n"+
			"Send me all the files for this bundle now\\.\n"+
			"When you're done, send `/done` to save and get a code\\.\n"+
			"Send `/cancel` to discard without saving\\.",
	)
}

// ── /done ─────────────────────────────────────────────────────────────────────

func (b *Bot) cmdDone(m *tgbotapi.Message) {
	msgs := b.sessions.End(m.From.ID)
	if msgs == nil {
		b.reply(m, "⚠️ No active bundle\\. Start one with `/begin`\\.")
		return
	}
	if len(msgs) == 0 {
		b.reply(m, "⚠️ Bundle is empty — you didn't send any files\\. Use `/begin` to start again\\.")
		return
	}

	// Sort files alphabetically / numerically before persisting so that
	// users always receive them in a predictable order regardless of the
	// sequence Telegram forwarded them in. Old bundles already in the DB
	// are not affected — their position values remain unchanged.
	sortBundleMessages(msgs)

	code, err := b.db.InsertBundle(msgs)
	if err != nil {
		log.Printf("InsertBundle error: %v", err)
		b.reply(m, "⚠️ Failed to save bundle\\. Please try again\\.")
		return
	}

	b.reply(m, fmt.Sprintf(
		"✅ *Bundle saved\\!*\n\n"+
			"Code: `%s`\n"+
			"Files: *%d*\n\n"+
			"Share this code or link:\n"+
			"`/send %s`\n"+
			"🔗 `https://t\\.me/%s?start=%s`",
		code, len(msgs), code,
		escMD(b.api.Self.UserName), code,
	))
}

// ── /cancel ───────────────────────────────────────────────────────────────────

func (b *Bot) cmdCancel(m *tgbotapi.Message) {
	if !b.sessions.Cancel(m.From.ID) {
		b.reply(m, "No active bundle to cancel\\.")
		return
	}
	b.reply(m, "🗑 Bundle discarded\\.")
}

// ── File received from admin ──────────────────────────────────────────────────

func (b *Bot) handleFileFromAdmin(m *tgbotapi.Message) {
	uid := m.From.ID

	if !b.sessions.IsRecording(uid) {
		b.reply(m,
			"ℹ️ Start a bundle first with `/begin`, then send your files\\.",
		)
		return
	}

	fileName, fileType := extractFileInfo(m)
	msg := BundleMessage{
		ChatID:    m.Chat.ID,
		MessageID: m.MessageID,
		FileName:  fileName,
		FileType:  fileType,
	}

	chatID := m.Chat.ID

	// Append and (re)start the 1-second debounce window. No per-file reply is
	// sent here. Instead, once a full second passes with no new file arriving,
	// onFlush fires once with the complete sorted list — giving the admin a
	// single, clean summary instead of N noisy "Added …" messages.
	b.sessions.AppendAndDebounce(uid, msg, time.Second, func(sorted []BundleMessage) {
		b.sendSortedList(chatID, sorted)
	})
}

// sendSortedList sends the admin a single message listing all queued files in
// sorted order. Called by the debounce callback after the 1-second quiet window.
func (b *Bot) sendSortedList(chatID int64, msgs []BundleMessage) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(
		"\U0001f4e6 *%d file%s queued \u2014 sorted order:*\n\n",
		len(msgs), pluralS(len(msgs)),
	))
	for i, m := range msgs {
		sb.WriteString(fmt.Sprintf(
			"%d\\. `%s`\n",
			i+1, escMD(truncate(m.FileName, 50)),
		))
	}
	sb.WriteString("\n_Send more files, or `/done` to save\\._")

	out := tgbotapi.NewMessage(chatID, sb.String())
	out.ParseMode = tgbotapi.ModeMarkdownV2
	if _, err := b.api.Send(out); err != nil {
		log.Printf("sendSortedList error: %v", err)
	}
}

// ── /send <code> ──────────────────────────────────────────────────────────────

func (b *Bot) cmdSend(m *tgbotapi.Message) {
	code := strings.TrimSpace(m.CommandArguments())
	if code == "" {
		b.reply(m, "Usage: `/send <code>`")
		return
	}

	msgs, err := b.db.GetBundle(code)
	if err != nil {
		log.Printf("GetBundle error: %v", err)
		b.reply(m, "⚠️ Internal error\\. Please try again\\.")
		return
	}
	if msgs == nil {
		b.reply(m, fmt.Sprintf("❌ Code `%s` not found\\.", escMD(code)))
		return
	}

	for _, msg := range msgs {
		cp := tgbotapi.NewCopyMessage(m.Chat.ID, msg.ChatID, msg.MessageID)
		if _, err := b.api.CopyMessage(cp); err != nil {
			log.Printf("CopyMessage error for %s msg %d: %v", code, msg.MessageID, err)
			b.reply(m, fmt.Sprintf(
				"⚠️ Could not deliver file %d of %d — it may have been deleted from the source\\.",
				msg.Position+1, len(msgs),
			))
		}
	}
}

// ── /listfiles ────────────────────────────────────────────────────────────────

func (b *Bot) cmdListFiles(m *tgbotapi.Message, page int) {
	total, err := b.db.CountBundles()
	if err != nil {
		b.reply(m, "⚠️ DB error\\.")
		return
	}
	if total == 0 {
		b.reply(m, "📂 No bundles stored yet\\.")
		return
	}

	pageSize   := b.cfg.PageSize
	totalPages := (total + pageSize - 1) / pageSize
	if page >= totalPages {
		page = totalPages - 1
	}

	bundles, err := b.db.PageBundles(page*pageSize, pageSize)
	if err != nil {
		b.reply(m, "⚠️ DB error\\.")
		return
	}

	text := ListMessageText(total, page, pageSize)
	kb   := BuildListKeyboard(bundles, page, totalPages)

	msg := tgbotapi.NewMessage(m.Chat.ID, text)
	msg.ParseMode   = tgbotapi.ModeMarkdownV2
	msg.ReplyMarkup = kb
	b.api.Send(msg)
}

// ── /delete <code> [code …] ───────────────────────────────────────────────────

func (b *Bot) cmdDelete(m *tgbotapi.Message) {
	args := strings.Fields(m.CommandArguments())
	if len(args) == 0 {
		b.reply(m, "Usage: `/delete <code> [code …]`")
		return
	}

	deleted, err := b.db.DeleteBundles(args)
	if err != nil {
		log.Printf("DeleteBundles error: %v", err)
		b.reply(m, "⚠️ DB error during deletion\\.")
		return
	}

	notFound := int64(len(args)) - deleted
	text := fmt.Sprintf("🗑 Deleted *%d* bundle%s\\.", deleted, pluralS(int(deleted)))
	if notFound > 0 {
		text += fmt.Sprintf("\n_%d code%s not found\\._", notFound, pluralS(int(notFound)))
	}
	b.reply(m, text)
}

// ── /stats ────────────────────────────────────────────────────────────────────

func (b *Bot) cmdStats(m *tgbotapi.Message) {
	total, err := b.db.CountBundles()
	if err != nil {
		b.reply(m, "⚠️ DB error\\.")
		return
	}

	var newest, oldest *Bundle
	if total > 0 {
		if bundles, _ := b.db.PageBundles(0, 1); len(bundles) > 0 {
			newest = bundles[0]
		}
		if bundles, _ := b.db.PageBundles(total-1, 1); len(bundles) > 0 {
			oldest = bundles[0]
		}
	}
	b.reply(m, StatsText(total, newest, oldest))
}

// ── Callback handler ──────────────────────────────────────────────────────────

func (b *Bot) handleCallback(cq *tgbotapi.CallbackQuery) {
	uid  := cq.From.ID
	data := cq.Data

	defer func() {
		b.api.Request(tgbotapi.NewCallback(cq.ID, ""))
	}()

	if !b.cfg.IsAdmin(uid) {
		b.api.Request(tgbotapi.NewCallbackWithAlert(cq.ID, "⛔ Admins only"))
		return
	}

	switch {
	case data == "noop":
		return

	case data == "close":
		b.api.Request(tgbotapi.NewDeleteMessage(cq.Message.Chat.ID, cq.Message.MessageID))

	case strings.HasPrefix(data, "page:"):
		page := parseTrailingInt(data, "page:")
		b.refreshListMessage(cq.Message, page)

	case strings.HasPrefix(data, "del:"):
		code := strings.TrimPrefix(data, "del:")
		msgs, err := b.db.GetBundle(code)
		if err != nil || msgs == nil {
			b.editText(cq.Message, "❌ Bundle not found — it may already be deleted\\.")
			return
		}
		text := fmt.Sprintf(
			"❓ Delete bundle `%s`?\n_%d file%s will be removed\\._",
			code, len(msgs), pluralS(len(msgs)),
		)
		b.editTextAndKeyboard(cq.Message, text, BuildConfirmDeleteOne(code, len(msgs)))

	case strings.HasPrefix(data, "delok:"):
		code := strings.TrimPrefix(data, "delok:")
		deleted, err := b.db.DeleteBundle(code)
		if err != nil {
			b.editText(cq.Message, "⚠️ DB error\\.")
			return
		}
		if !deleted {
			b.editText(cq.Message, fmt.Sprintf("⚠️ Code `%s` was already gone\\.", escMD(code)))
			return
		}
		b.refreshListMessage(cq.Message, 0)

	case strings.HasPrefix(data, "delpage:"):
		page := parseTrailingInt(data, "delpage:")
		text := fmt.Sprintf("❓ Delete *all bundles on page %d*?", page+1)
		b.editTextAndKeyboard(cq.Message, text, BuildConfirmDeletePage(page))

	case strings.HasPrefix(data, "delpageok:"):
		page     := parseTrailingInt(data, "delpageok:")
		codes, err := b.db.CodesOnPage(page*b.cfg.PageSize, b.cfg.PageSize)
		if err != nil {
			b.editText(cq.Message, "⚠️ DB error\\.")
			return
		}
		if _, err := b.db.DeleteBundles(codes); err != nil {
			b.editText(cq.Message, "⚠️ DB error during deletion\\.")
			return
		}
		b.refreshListMessage(cq.Message, 0)

	case data == "delall":
		total, _ := b.db.CountBundles()
		text := fmt.Sprintf("❓ Delete *all %d bundles*? This cannot be undone\\.", total)
		b.editTextAndKeyboard(cq.Message, text, BuildConfirmDeleteAll())

	case data == "delallok":
		deleted, err := b.db.DeleteAllBundles()
		if err != nil {
			b.editText(cq.Message, "⚠️ DB error\\.")
			return
		}
		b.editText(cq.Message,
			fmt.Sprintf("💣 Deleted all *%d* bundle%s\\.", deleted, pluralS(int(deleted))),
		)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (b *Bot) requireAdmin(m *tgbotapi.Message) bool {
	if b.cfg.IsAdmin(m.From.ID) {
		return true
	}
	b.reply(m, "⛔ This command is for admins only\\.")
	return false
}

func (b *Bot) reply(m *tgbotapi.Message, text string) {
	msg := tgbotapi.NewMessage(m.Chat.ID, text)
	msg.ParseMode = tgbotapi.ModeMarkdownV2
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("send error: %v", err)
	}
}

func (b *Bot) editText(m *tgbotapi.Message, text string) {
	edit := tgbotapi.NewEditMessageText(m.Chat.ID, m.MessageID, text)
	edit.ParseMode = tgbotapi.ModeMarkdownV2
	b.api.Request(edit)
}

func (b *Bot) editTextAndKeyboard(m *tgbotapi.Message, text string, kb tgbotapi.InlineKeyboardMarkup) {
	edit := tgbotapi.NewEditMessageText(m.Chat.ID, m.MessageID, text)
	edit.ParseMode   = tgbotapi.ModeMarkdownV2
	edit.ReplyMarkup = &kb
	b.api.Request(edit)
}

func (b *Bot) refreshListMessage(m *tgbotapi.Message, page int) {
	total, err := b.db.CountBundles()
	if err != nil {
		b.editText(m, "⚠️ DB error\\.")
		return
	}
	if total == 0 {
		b.editText(m, "📂 No bundles stored yet\\.")
		return
	}

	pageSize   := b.cfg.PageSize
	totalPages := (total + pageSize - 1) / pageSize
	if page >= totalPages {
		page = totalPages - 1
	}

	bundles, err := b.db.PageBundles(page*pageSize, pageSize)
	if err != nil {
		b.editText(m, "⚠️ DB error\\.")
		return
	}

	text := ListMessageText(total, page, pageSize)
	kb   := BuildListKeyboard(bundles, page, totalPages)
	b.editTextAndKeyboard(m, text, kb)
}

// ── File-detection helpers ────────────────────────────────────────────────────

func hasFile(m *tgbotapi.Message) bool {
	return m.Document != nil || m.Photo != nil || m.Video != nil ||
		m.Audio != nil || m.Voice != nil || m.VideoNote != nil ||
		m.Animation != nil || m.Sticker != nil
}

func extractFileInfo(m *tgbotapi.Message) (fileName, fileType string) {
	switch {
	case m.Document != nil:
		name := m.Document.FileName
		if name == "" {
			name = "document"
		}
		return name, "document"
	case m.Photo != nil:
		return "photo.jpg", "photo"
	case m.Video != nil:
		name := m.Video.FileName
		if name == "" {
			name = "video.mp4"
		}
		return name, "video"
	case m.Audio != nil:
		name := m.Audio.FileName
		if name == "" {
			name = "audio"
		}
		return name, "audio"
	case m.Voice != nil:
		return "voice.ogg", "voice"
	case m.VideoNote != nil:
		return "video_note.mp4", "video_note"
	case m.Animation != nil:
		name := m.Animation.FileName
		if name == "" {
			name = "animation.gif"
		}
		return name, "animation"
	case m.Sticker != nil:
		return "sticker", "sticker"
	}
	return "file", "unknown"
}

func parseTrailingInt(s, prefix string) int {
	var n int
	fmt.Sscanf(strings.TrimPrefix(s, prefix), "%d", &n)
	return n
}