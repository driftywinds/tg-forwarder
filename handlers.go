package main

import (
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot wraps the API client, database and config together.
type Bot struct {
	api  *tgbotapi.BotAPI
	db   *DB
	cfg  *Config
}

func NewBot(api *tgbotapi.BotAPI, db *DB, cfg *Config) *Bot {
	return &Bot{api: api, db: db, cfg: cfg}
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

	// Commands
	if m.IsCommand() {
		switch m.Command() {
		case "start", "help":
			b.cmdHelp(m)
		case "send":
			b.cmdSend(m)
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

	// File upload (admins only)
	if b.cfg.IsAdmin(uid) && hasFile(m) {
		b.handleFileUpload(m)
		return
	}
}

// ── /help ─────────────────────────────────────────────────────────────────────

func (b *Bot) cmdHelp(m *tgbotapi.Message) {
	if b.cfg.IsAdmin(m.From.ID) {
		b.reply(m,
			"*FileStore Bot — Admin*\n\n"+
				"*Upload a file*: just send it here → you get a 9\\-char code\\.\n\n"+
				"*Commands*\n"+
				"`/send <code>` — retrieve a file by code\n"+
				"`/listfiles` — browse & delete stored files\n"+
				"`/delete <code> [code …]` — delete one or more files by code\n"+
				"`/stats` — storage summary\n"+
				"`/help` — this message",
		)
	} else {
		b.reply(m,
			"*FileStore Bot*\n\n"+
				"Use `/send <code>` to retrieve a file\\.\n"+
				"Example: `/send abcd56789`",
		)
	}
}

// ── /send <code> ──────────────────────────────────────────────────────────────

func (b *Bot) cmdSend(m *tgbotapi.Message) {
	code := strings.TrimSpace(m.CommandArguments())
	if code == "" {
		b.reply(m, "Usage: `/send <code>`")
		return
	}

	rec, err := b.db.Get(code)
	if err != nil {
		log.Printf("db.Get error: %v", err)
		b.reply(m, "⚠️ Internal error\\. Please try again\\.")
		return
	}
	if rec == nil {
		b.reply(m, fmt.Sprintf("❌ Code `%s` not found\\.", escMD(code)))
		return
	}

	// CopyMessage sends the file without exposing the source chat/user.
	copy := tgbotapi.NewCopyMessage(m.Chat.ID, rec.ChatID, rec.MessageID)
	if _, err := b.api.CopyMessage(copy); err != nil {
		log.Printf("CopyMessage error: %v", err)
		b.reply(m, "⚠️ Could not deliver the file\\. It may have been deleted from the source chat\\.")
	}
}

// ── File upload handler ───────────────────────────────────────────────────────

func (b *Bot) handleFileUpload(m *tgbotapi.Message) {
	fileName, fileType := extractFileInfo(m)

	code, err := b.db.Insert(m.Chat.ID, m.MessageID, fileName, fileType)
	if err != nil {
		log.Printf("db.Insert error: %v", err)
		b.reply(m, "⚠️ Failed to store file\\. Please try again\\.")
		return
	}

	emoji := fileTypeEmoji(fileType)
	text := fmt.Sprintf(
		"%s File stored\\!\n\n"+
			"Code: `%s`\n"+
			"Name: %s\n\n"+
			"Share this code with users — they can retrieve it with `/send %s`",
		emoji, code, escMD(fileName), code,
	)
	b.reply(m, text)
}

// ── /listfiles ────────────────────────────────────────────────────────────────

func (b *Bot) cmdListFiles(m *tgbotapi.Message, page int) {
	total, err := b.db.Count()
	if err != nil {
		b.reply(m, "⚠️ DB error\\.")
		return
	}
	if total == 0 {
		b.reply(m, "📂 No files stored yet\\.")
		return
	}

	pageSize   := b.cfg.PageSize
	totalPages := (total + pageSize - 1) / pageSize
	if page >= totalPages {
		page = totalPages - 1
	}

	records, err := b.db.Page(page*pageSize, pageSize)
	if err != nil {
		b.reply(m, "⚠️ DB error\\.")
		return
	}

	text := ListMessageText(total, page, pageSize)
	kb   := BuildListKeyboard(records, page, totalPages)

	msg := tgbotapi.NewMessage(m.Chat.ID, text)
	msg.ParseMode    = tgbotapi.ModeMarkdownV2
	msg.ReplyMarkup  = kb
	b.api.Send(msg) //nolint:errcheck
}

// ── /delete <code> [code …] ───────────────────────────────────────────────────

func (b *Bot) cmdDelete(m *tgbotapi.Message) {
	args := strings.Fields(m.CommandArguments())
	if len(args) == 0 {
		b.reply(m, "Usage: `/delete <code> [code …]`")
		return
	}

	deleted, err := b.db.DeleteMany(args)
	if err != nil {
		log.Printf("db.DeleteMany error: %v", err)
		b.reply(m, "⚠️ DB error during deletion\\.")
		return
	}

	notFound := int64(len(args)) - deleted
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🗑 Deleted *%d* file%s\\.", deleted, pluralS(int(deleted))))
	if notFound > 0 {
		sb.WriteString(fmt.Sprintf("\n_%d code%s not found\\._", notFound, pluralS(int(notFound))))
	}
	b.reply(m, sb.String())
}

// ── /stats ────────────────────────────────────────────────────────────────────

func (b *Bot) cmdStats(m *tgbotapi.Message) {
	total, err := b.db.Count()
	if err != nil {
		b.reply(m, "⚠️ DB error\\.")
		return
	}

	var newest, oldest *FileRecord
	if total > 0 {
		recs, _ := b.db.Page(0, 1)
		if len(recs) > 0 {
			newest = recs[0]
		}
		recs, _ = b.db.Page(total-1, 1)
		if len(recs) > 0 {
			oldest = recs[0]
		}
	}
	b.reply(m, StatsText(total, newest, oldest))
}

// ── Callback handler ──────────────────────────────────────────────────────────

func (b *Bot) handleCallback(cq *tgbotapi.CallbackQuery) {
	uid  := cq.From.ID
	data := cq.Data

	// Always answer the callback to clear the loading spinner.
	defer func() {
		b.api.Request(tgbotapi.NewCallback(cq.ID, "")) //nolint:errcheck
	}()

	if !b.cfg.IsAdmin(uid) {
		b.api.Request(tgbotapi.NewCallbackWithAlert(cq.ID, "⛔ Admins only")) //nolint:errcheck
		return
	}

	switch {
	case data == "noop":
		return

	case data == "close":
		del := tgbotapi.NewDeleteMessage(cq.Message.Chat.ID, cq.Message.MessageID)
		b.api.Request(del) //nolint:errcheck

	// ── page navigation ────────────────────────────────────────────────────────
	case strings.HasPrefix(data, "page:"):
		page := parseTrailingInt(data, "page:")
		b.refreshListMessage(cq.Message, page)

	// ── single delete: show confirm ────────────────────────────────────────────
	case strings.HasPrefix(data, "del:"):
		code := strings.TrimPrefix(data, "del:")
		rec, err := b.db.Get(code)
		if err != nil || rec == nil {
			b.editText(cq.Message, "❌ Record not found — it may already be deleted\\.")
			return
		}
		emoji := fileTypeEmoji(rec.FileType)
		text  := fmt.Sprintf(
			"❓ Delete `%s`?\n%s *%s*",
			code, emoji, escMD(truncate(rec.FileName, 48)),
		)
		b.editTextAndKeyboard(cq.Message, text, BuildConfirmDeleteOne(rec))

	// ── single delete: confirmed ───────────────────────────────────────────────
	case strings.HasPrefix(data, "delok:"):
		code := strings.TrimPrefix(data, "delok:")
		deleted, err := b.db.Delete(code)
		if err != nil {
			log.Printf("db.Delete error: %v", err)
			b.editText(cq.Message, "⚠️ DB error\\.")
			return
		}
		if !deleted {
			b.editText(cq.Message, fmt.Sprintf("⚠️ Code `%s` was already gone\\.", escMD(code)))
			return
		}
		// Refresh at page 0
		b.refreshListMessage(cq.Message, 0)

	// ── delete page: show confirm ──────────────────────────────────────────────
	case strings.HasPrefix(data, "delpage:"):
		page := parseTrailingInt(data, "delpage:")
		text := fmt.Sprintf("❓ Delete *all files on page %d*?", page+1)
		b.editTextAndKeyboard(cq.Message, text, BuildConfirmDeletePage(page))

	// ── delete page: confirmed ─────────────────────────────────────────────────
	case strings.HasPrefix(data, "delpageok:"):
		page     := parseTrailingInt(data, "delpageok:")
		codes, err := b.db.CodesOnPage(page*b.cfg.PageSize, b.cfg.PageSize)
		if err != nil {
			b.editText(cq.Message, "⚠️ DB error\\.")
			return
		}
		deleted, err := b.db.DeleteMany(codes)
		if err != nil {
			b.editText(cq.Message, "⚠️ DB error during deletion\\.")
			return
		}
		_ = deleted
		b.refreshListMessage(cq.Message, 0)

	// ── delete all: show confirm ───────────────────────────────────────────────
	case data == "delall":
		total, _ := b.db.Count()
		text := fmt.Sprintf("❓ Delete *all %d files*? This cannot be undone\\.", total)
		b.editTextAndKeyboard(cq.Message, text, BuildConfirmDeleteAll())

	// ── delete all: confirmed ──────────────────────────────────────────────────
	case data == "delallok":
		deleted, err := b.db.DeleteAll()
		if err != nil {
			log.Printf("db.DeleteAll error: %v", err)
			b.editText(cq.Message, "⚠️ DB error\\.")
			return
		}
		b.editText(cq.Message,
			fmt.Sprintf("💣 Deleted all *%d* file%s\\.", deleted, pluralS(int(deleted))),
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
	b.api.Request(edit) //nolint:errcheck
}

func (b *Bot) editTextAndKeyboard(m *tgbotapi.Message, text string, kb tgbotapi.InlineKeyboardMarkup) {
	edit := tgbotapi.NewEditMessageText(m.Chat.ID, m.MessageID, text)
	edit.ParseMode   = tgbotapi.ModeMarkdownV2
	edit.ReplyMarkup = &kb
	b.api.Request(edit) //nolint:errcheck
}

func (b *Bot) refreshListMessage(m *tgbotapi.Message, page int) {
	total, err := b.db.Count()
	if err != nil {
		b.editText(m, "⚠️ DB error\\.")
		return
	}

	if total == 0 {
		b.editText(m, "📂 No files stored yet\\.")
		return
	}

	pageSize   := b.cfg.PageSize
	totalPages := (total + pageSize - 1) / pageSize
	if page >= totalPages {
		page = totalPages - 1
	}

	records, err := b.db.Page(page*pageSize, pageSize)
	if err != nil {
		b.editText(m, "⚠️ DB error\\.")
		return
	}

	text := ListMessageText(total, page, pageSize)
	kb   := BuildListKeyboard(records, page, totalPages)
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