package main

import (
	"log"
	"net/http"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env if present (ignored in production where env vars are set externally)
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Printf("Warning: could not load .env file: %v", err)
	}

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}

	db, err := openDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("DB error: %v", err)
	}
	log.Printf("Database opened: %s", cfg.DBPath)

	// Build the HTTP client (with or without proxy) before connecting to Telegram.
	// NewBotAPIWithClient calls getMe internally, so the proxy must be in place first.
	var httpClient *http.Client
	if cfg.ProxyURL != "" {
		pxyClient, pxyErr := newProxyHTTPClient(cfg.ProxyURL)
		if pxyErr != nil {
			log.Fatalf("Proxy error: %v", pxyErr)
		}
		httpClient = pxyClient
		log.Printf("Using SOCKS5 proxy: %s", proxyAddr(cfg.ProxyURL))
	} else {
		httpClient = &http.Client{}
		log.Printf("No proxy configured (set PROXY_URL to use one)")
	}

	api, err := tgbotapi.NewBotAPIWithClient(cfg.BotToken, tgbotapi.APIEndpoint, httpClient)
	if err != nil {
		log.Fatalf("Telegram API error: %v", err)
	}
	log.Printf("Logged in as @%s", api.Self.UserName)

	bot := NewBot(api, db, cfg)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := api.GetUpdatesChan(u)
	log.Printf("Listening for updates … (admins: %d configured)", len(cfg.AdminIDs))

	for update := range updates {
		go bot.HandleUpdate(update) // each update in its own goroutine
	}
}