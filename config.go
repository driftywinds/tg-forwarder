package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	BotToken string
	AdminIDs map[int64]bool
	DBPath   string
	PageSize int
}

func loadConfig() (*Config, error) {
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("BOT_TOKEN is required")
	}

	rawIDs := os.Getenv("ADMIN_IDS") // comma-separated telegram user IDs
	if rawIDs == "" {
		return nil, fmt.Errorf("ADMIN_IDS is required (comma-separated Telegram user IDs)")
	}

	adminIDs := make(map[int64]bool)
	for _, part := range strings.Split(rawIDs, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid admin ID %q: %w", part, err)
		}
		adminIDs[id] = true
	}
	if len(adminIDs) == 0 {
		return nil, fmt.Errorf("ADMIN_IDS must contain at least one valid ID")
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "filestore.db"
	}

	pageSize := 8
	if ps := os.Getenv("PAGE_SIZE"); ps != "" {
		if n, err := strconv.Atoi(ps); err == nil && n > 0 {
			pageSize = n
		}
	}

	return &Config{
		BotToken: token,
		AdminIDs: adminIDs,
		DBPath:   dbPath,
		PageSize: pageSize,
	}, nil
}

func (c *Config) IsAdmin(userID int64) bool {
	return c.AdminIDs[userID]
}