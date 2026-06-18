package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/proxy"
)

// parseProxyURL parses a proxy URL string into a host:port pair.
// Supported formats:
//   - https://t.me/socks?server=HOST&port=PORT
//   - socks5://HOST:PORT
//   - socks5://USER:PASS@HOST:PORT
func parseProxyURL(raw string) (host, port, user, pass string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", "", "", fmt.Errorf("empty proxy URL")
	}

	// Telegram-style deep link: https://t.me/socks?server=...&port=...
	if strings.Contains(raw, "t.me/socks") || strings.Contains(raw, "t.me/proxy") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", "", "", "", fmt.Errorf("invalid proxy URL: %w", err)
		}
		host = u.Query().Get("server")
		port = u.Query().Get("port")
		if host == "" || port == "" {
			return "", "", "", "", fmt.Errorf("t.me proxy link missing server or port")
		}
		return host, port, "", "", nil
	}

	// Standard socks5:// URL
	if strings.HasPrefix(raw, "socks5://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", "", "", "", fmt.Errorf("invalid proxy URL: %w", err)
		}
		host = u.Hostname()
		port = u.Port()
		if host == "" || port == "" {
			return "", "", "", "", fmt.Errorf("socks5 URL missing host or port")
		}
		if u.User != nil {
			user = u.User.Username()
			pass, _ = u.User.Password()
		}
		return host, port, user, pass, nil
	}

	return "", "", "", "", fmt.Errorf("unsupported proxy URL format: %s", raw)
}

// newProxyHTTPClient creates an *http.Client that routes through a SOCKS5 proxy.
func newProxyHTTPClient(proxyURL string) (*http.Client, error) {
	host, port, user, pass, err := parseProxyURL(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("parse proxy: %w", err)
	}

	addr := net.JoinHostPort(host, port)

	var auth *proxy.Auth
	if user != "" {
		auth = &proxy.Auth{User: user, Password: pass}
	}

	dialer, err := proxy.SOCKS5("tcp", addr, auth, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("socks5 dialer: %w", err)
	}

	type ctxDialer interface {
		DialContext(context.Context, string, string) (net.Conn, error)
	}
	cd, ok := dialer.(ctxDialer)
	if !ok {
		return nil, fmt.Errorf("socks5 dialer does not support DialContext")
	}

	transport := &http.Transport{DialContext: cd.DialContext}

	return &http.Client{Transport: transport}, nil
}

// proxyAddr returns the human-readable proxy address for logging.
func proxyAddr(proxyURL string) string {
	host, port, _, _, err := parseProxyURL(proxyURL)
	if err != nil {
		return proxyURL
	}
	return net.JoinHostPort(host, port)
}
