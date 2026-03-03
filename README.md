# FileStore Bot

A Telegram bot that lets admins store files behind anonymous 9-character codes.
Users retrieve files with `/send <code>` — they never see who uploaded the file or from which chat.

---

## Features

| Feature | Detail |
|---|---|
| **Anonymous delivery** | Uses `copyMessage` (not `forwardMessage`), so no source info leaks |
| **Random codes** | Cryptographically random 9-char alphanumeric (`a–z`, `0–9`), collision-resistant |
| **Supported file types** | Documents, photos, videos, audio, voice notes, video notes, animations, stickers |
| **Paginated admin list** | `/listfiles` with inline keyboard — handles hundreds of files cleanly |
| **Bulk delete** | Delete by code(s), delete whole page, or wipe everything, all with confirm prompts |
| **SQLite storage** | Single-file database, WAL mode for safe concurrent access |
| **Docker ready** | Single-binary, minimal Alpine image |

---

## Quick Start

### 1. Prerequisites

- Go 1.23+ with CGO enabled (needed for `go-sqlite3`)
- `gcc` / `musl-dev` (Linux) or Xcode CLT (macOS)
- A bot token from [@BotFather](https://t.me/BotFather)
- Your Telegram user ID (get it from [@userinfobot](https://t.me/userinfobot))

### 2. Clone & configure

```bash
git clone <your-repo>
cd filestore-bot
cp .env.example .env
# Edit .env — set BOT_TOKEN and ADMIN_IDS
```

### 3. Run

```bash
make run
# or build then run:
make build
./filestore-bot
```

### 4. Docker

```bash
docker build -t filestore-bot .

docker run -d \
  --name filestore-bot \
  --restart unless-stopped \
  -e BOT_TOKEN=your_token_here \
  -e ADMIN_IDS=123456789,987654321 \
  -v $(pwd)/data:/app/data \
  filestore-bot
```

---

## Configuration

All config is via environment variables (or `.env` file):

| Variable | Required | Default | Description |
|---|---|---|---|
| `BOT_TOKEN` | ✅ | — | Token from @BotFather |
| `ADMIN_IDS` | ✅ | — | Comma-separated Telegram user IDs |
| `DB_PATH` | ❌ | `filestore.db` | Path to SQLite database |
| `PAGE_SIZE` | ❌ | `8` | Files per page in `/listfiles` |

---

## Commands

### User commands
| Command | Description |
|---|---|
| `/send <code>` | Retrieve a file by its 9-char code |
| `/help` | Show help |

### Admin commands
| Command | Description |
|---|---|
| `/listfiles` | Browse all stored files with inline pagination |
| `/delete code1 [code2 …]` | Delete one or more files by code |
| `/stats` | Show total count, newest/oldest file |
| `/help` | Show admin help |

Admins can also **just send any file** to the bot — it replies with the code.

---

## Admin UI — `/listfiles`

```
📂 File Store — 247 files
Page 1 of 31 · showing 1–8 · tap a row to delete

🗑  k4j2m9xwp  📄  quarterly-report.pdf      12 Jan 25
🗑  r7nt3bqz8  🖼  banner.jpg                 11 Jan 25
🗑  x1c6pvd4y  🎬  demo-video.mp4             11 Jan 25
...

[ ◀ Prev ]  [ 1 / 31 ]  [ Next ▶ ]
[ 🗑 Delete This Page ]  [ 💣 Delete ALL ]  [ ✖ Close ]
```

Tapping any file row shows a **confirm/cancel** dialog before deletion.
Bulk actions also require confirmation.

---

## Code format

- Length: **9 characters**
- Charset: `a–z` + `0–9` (36 characters)
- Generated with `crypto/rand` — not sequential, not predictable
- Collision space: 36⁹ ≈ 101 billion possible codes

---

## Project structure

```
filestore-bot/
├── main.go       ← entry point, update loop
├── config.go     ← env var loading & admin check
├── db.go         ← SQLite CRUD + code generation
├── handlers.go   ← all command & callback handlers
├── keyboard.go   ← inline keyboard builders & text formatters
├── go.mod
├── Makefile
├── Dockerfile
└── .env.example
```