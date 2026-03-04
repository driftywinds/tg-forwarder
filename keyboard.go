package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max-1]) + "…"
}

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

// BuildListKeyboard builds the paginated inline keyboard for /listfiles.
// Each row shows one bundle: code · N files · date.
func BuildListKeyboard(bundles []*Bundle, page, totalPages int) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	for _, bun := range bundles {
		date  := bun.CreatedAt.Format("02 Jan 06")
		files := fmt.Sprintf("%d file%s", bun.FileCount, pluralS(bun.FileCount))
		label := fmt.Sprintf("🗑  %s  ·  %-10s  ·  %s", bun.Code, files, date)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, "del:"+bun.Code),
		))
	}

	// Navigation row
	nav := []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("  %d / %d  ", page+1, totalPages), "noop",
		),
	}
	if page > 0 {
		nav = append([]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("◀ Prev", fmt.Sprintf("page:%d", page-1)),
		}, nav...)
	}
	if page < totalPages-1 {
		nav = append(nav,
			tgbotapi.NewInlineKeyboardButtonData("Next ▶", fmt.Sprintf("page:%d", page+1)),
		)
	}
	rows = append(rows, nav)

	// Bulk row
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🗑 Delete This Page", fmt.Sprintf("delpage:%d", page)),
		tgbotapi.NewInlineKeyboardButtonData("💣 Delete ALL",       "delall"),
		tgbotapi.NewInlineKeyboardButtonData("✖ Close",             "close"),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func BuildConfirmDeleteOne(code string, fileCount int) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Yes, delete", "delok:"+code),
			tgbotapi.NewInlineKeyboardButtonData("❌ Cancel",      "noop"),
		),
	)
}

func BuildConfirmDeletePage(page int) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Delete this page", fmt.Sprintf("delpageok:%d", page)),
			tgbotapi.NewInlineKeyboardButtonData("❌ Cancel",           "noop"),
		),
	)
}

func BuildConfirmDeleteAll() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💣 Yes, wipe everything", "delallok"),
			tgbotapi.NewInlineKeyboardButtonData("❌ Cancel",               "noop"),
		),
	)
}

func ListMessageText(total, page, pageSize int) string {
	if total == 0 {
		return "📂 *Bundle Store* — no bundles stored yet\\."
	}
	start := page*pageSize + 1
	end   := (page+1)*pageSize
	if end > total {
		end = total
	}
	totalPages := (total + pageSize - 1) / pageSize
	return fmt.Sprintf(
		"📂 *Bundle Store* — %d bundle%s\n"+
			"_Page %d of %d · showing %d–%d · tap a row to delete_",
		total, pluralS(total),
		page+1, totalPages,
		start, end,
	)
}

func StatsText(total int, newest, oldest *Bundle) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 *Storage Stats*\n\nTotal bundles: *%d*\n", total))
	if newest != nil {
		sb.WriteString(fmt.Sprintf(
			"Newest: `%s` \\(%d file%s, %s\\)\n",
			newest.Code, newest.FileCount, pluralS(newest.FileCount),
			escMD(newest.CreatedAt.UTC().Format("02 Jan 2006, 15:04 UTC")),
		))
	}
	if oldest != nil && oldest.Code != newest.Code {
		sb.WriteString(fmt.Sprintf(
			"Oldest: `%s` \\(%d file%s, %s\\)\n",
			oldest.Code, oldest.FileCount, pluralS(oldest.FileCount),
			escMD(oldest.CreatedAt.UTC().Format("02 Jan 2006, 15:04 UTC")),
		))
	}
	return sb.String()
}

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

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}