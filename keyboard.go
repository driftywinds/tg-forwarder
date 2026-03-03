package main

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ── Callback data format ──────────────────────────────────────────────────────
//
//   page:<n>              → navigate to page n
//   del:<code>            → single-delete prompt
//   delok:<code>          → single-delete confirmed
//   delpage:<n>           → delete-this-page prompt
//   delpageok:<n>         → delete-this-page confirmed
//   delall                → delete-all prompt
//   delallok              → delete-all confirmed
//   noop                  → no-op (display-only button)
//   close                 → remove the list message
//
// ─────────────────────────────────────────────────────────────────────────────

func pageCallbackData(n int) string       { return fmt.Sprintf("page:%d", n) }
func delCallbackData(code string) string  { return "del:" + code }
func delOKCallbackData(code string) string { return "delok:" + code }
func delPageCallbackData(n int) string    { return fmt.Sprintf("delpage:%d", n) }
func delPageOKCallbackData(n int) string  { return fmt.Sprintf("delpageok:%d", n) }

// fileTypeEmoji maps the stored file type to a small emoji.
func fileTypeEmoji(ft string) string {
	switch ft {
	case "photo":
		return "🖼"
	case "video":
		return "🎬"
	case "audio":
		return "🎵"
	case "voice":
		return "🎤"
	case "video_note":
		return "📹"
	case "animation":
		return "🎞"
	case "sticker":
		return "🎭"
	default:
		return "📄"
	}
}

// truncate cuts a string to max runes, appending "…" if needed.
func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max-1]) + "…"
}

// BuildListKeyboard returns the inline keyboard for the main file list page.
func BuildListKeyboard(records []*FileRecord, page, totalPages int) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	for _, r := range records {
		emoji := fileTypeEmoji(r.FileType)
		date  := r.CreatedAt.Format("02 Jan 06")
		name  := truncate(r.FileName, 24)
		label := fmt.Sprintf("🗑  %s  %s  %-24s  %s", r.Code, emoji, name, date)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, delCallbackData(r.Code)),
		))
	}

	// ── navigation row ────────────────────────────────────────────────────────
	nav := []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("  %d / %d  ", page+1, totalPages), "noop",
		),
	}
	if page > 0 {
		nav = append([]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("◀ Prev", pageCallbackData(page-1)),
		}, nav...)
	}
	if page < totalPages-1 {
		nav = append(nav,
			tgbotapi.NewInlineKeyboardButtonData("Next ▶", pageCallbackData(page+1)),
		)
	}
	rows = append(rows, nav)

	// ── bulk / close row ──────────────────────────────────────────────────────
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🗑 Delete This Page", delPageCallbackData(page)),
		tgbotapi.NewInlineKeyboardButtonData("💣 Delete ALL",       "delall"),
		tgbotapi.NewInlineKeyboardButtonData("✖ Close",             "close"),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// BuildConfirmDeleteOne returns a two-button confirm keyboard for a single file.
func BuildConfirmDeleteOne(r *FileRecord) tgbotapi.InlineKeyboardMarkup {
	emoji := fileTypeEmoji(r.FileType)
	label := fmt.Sprintf("%s %s  ·  %s", emoji, r.Code, truncate(r.FileName, 28))
	_ = label
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Yes, delete", delOKCallbackData(r.Code)),
			tgbotapi.NewInlineKeyboardButtonData("❌ Cancel",      "noop"),
		),
	)
}

// BuildConfirmDeletePage returns confirm keyboard for deleting a whole page.
func BuildConfirmDeletePage(page int) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Delete this page", delPageOKCallbackData(page)),
			tgbotapi.NewInlineKeyboardButtonData("❌ Cancel",           "noop"),
		),
	)
}

// BuildConfirmDeleteAll returns confirm keyboard for deleting everything.
func BuildConfirmDeleteAll() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💣 Yes, wipe everything", "delallok"),
			tgbotapi.NewInlineKeyboardButtonData("❌ Cancel",               "noop"),
		),
	)
}

// ListMessageText returns the header text for the list view.
func ListMessageText(total, page, pageSize int) string {
	if total == 0 {
		return "📂 *File Store* — no files stored yet\\."
	}
	start := page*pageSize + 1
	end   := (page+1)*pageSize
	if end > total {
		end = total
	}
	totalPages := (total + pageSize - 1) / pageSize
	return fmt.Sprintf(
		"📂 *File Store* — %d file%s\n"+
			"_Page %d of %d · showing %d–%d · tap a row to delete_",
		total, pluralS(total),
		page+1, totalPages,
		start, end,
	)
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// FormatTimestamp is a small helper used in handler messages.
func FormatTimestamp(t time.Time) string {
	return t.UTC().Format("02 Jan 2006, 15:04 UTC")
}

// StatsText builds the text for /stats.
func StatsText(total int, newest, oldest *FileRecord) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 *Storage Stats*\n\nTotal files: *%d*\n", total))
	if newest != nil {
		sb.WriteString(fmt.Sprintf("Newest: `%s` \\(%s\\)\n", newest.Code, escMD(FormatTimestamp(newest.CreatedAt))))
	}
	if oldest != nil && oldest.Code != newest.Code {
		sb.WriteString(fmt.Sprintf("Oldest: `%s` \\(%s\\)\n", oldest.Code, escMD(FormatTimestamp(oldest.CreatedAt))))
	}
	return sb.String()
}

// escMD escapes MarkdownV2 special characters in plain text.
func escMD(s string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"_", "\\_", "*", "\\*", "[", "\\[", "]", "\\]",
		"(", "\\(", ")", "\\)", "~", "\\~", "`", "\\`",
		">", "\\>", "#", "\\#", "+", "\\+", "-", "\\-",
		"=", "\\=", "|", "\\|", "{", "\\{", "}", "\\}",
		".", "\\.", "!", "\\!",
	)
	return replacer.Replace(s)
}