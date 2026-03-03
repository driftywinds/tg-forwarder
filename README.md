# FileStore Bot

> [!IMPORTANT]
> ## ⚠️ AI-Generated Project
> **This entire project — every line of code, every config file, and this README — was written by [Claude](https://claude.ai) (Anthropic). It was not written by a human developer.**
>
> No code was manually authored. The bot was built iteratively through a conversation with Claude, including debugging, fixing driver compatibility issues, resolving SQLite concurrency bugs, and UI tweaks — all AI-generated.
>
> Use in production at your own discretion.

---

A Telegram bot that lets admins store files behind anonymous 9-character codes.
Users retrieve files with `/send <code>` — they never see who uploaded the file or from which chat.

---

## Features

| Feature | Detail |
|---|---|
| **Anonymous delivery** | Uses `CopyMessage` (not `ForwardMessage`), so no source chat or uploader is ever revealed |
| **Random codes** | Cryptographically random 9-char alphanumeric (`a–z`, `0–9`) via `crypto/rand`, collision-checked |
| **Supported file types** | Documents, photos, videos, audio, voice notes, video notes, animations, stickers |
| **Paginated admin list** | `/listfiles` with inline keyboard — handles hundreds of files cleanly |
| **Bulk delete** | Delete by code(s), delete whole page, or wipe everything — all with confirm prompts |
| **Concurrent-safe writes** | `sync.Mutex` on all DB writes so bulk uploads (10+ files at once) never deadlock |
| **Pure-Go SQLite** | Uses `modernc.org/sqlite` — no CGO, no gcc, works on Windows out of the box |
| **Docker ready** | Single binary, minimal Alpine image, `compose.yaml` included |

---

## Quick Start

### Prerequisites

- Go 1.23+
- A bot token from [@BotFather](https://t.me/BotFather)
- Your Telegram user ID — get it from [@userinfobot](https://t.me/userinfobot)

### 1. Clone & configure

```bash
git clone <your-repo>
cd filestore-bot
copy .env.example .env
# Edit .env — fill in BOT_TOKEN and ADMIN_IDS
```

### 2. Run

```bash
go mod tidy
go run .
```

### 3. Build a binary

```bash
go build -o filestore-bot.exe .
```

---

## Configuration

All config is via `.env` file (or plain environment variables):

| Variable | Required | Default | Description |
|---|---|---|---|
| `BOT_TOKEN` | ✅ | — | Token from @BotFather |
| `ADMIN_IDS` | ✅ | — | Comma-separated Telegram user IDs |
| `DB_PATH` | ❌ | `filestore.db` | Path to SQLite database file |
| `PAGE_SIZE` | ❌ | `8` | Files shown per page in `/listfiles` |

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

Admins can also **just send any file** to the bot — it replies with the generated code.

---

## Admin UI — `/listfiles`

```
📂 File Store — 10 files
Page 1 of 2 · showing 1–8 · tap a row to delete

🗑  z449nia67  📄  Ben_10_2005_S04E10_Good...  03 Mar 26
🗑  r6ldh81sf  📄  Ben_10_2005_S04E01_Perf...  03 Mar 26
🗑  pf9vmysdi  📄  Ben_10_2005_S04E05_Ben_...  03 Mar 26
...

[ 1 / 2 ]  [ Next ▶ ]
[ 🗑 Delete This Page ]  [ 💣 Delete ALL ]  [ ✖ Close ]
```

- Tapping a file row shows a **confirm / cancel** prompt before deleting
- Bulk actions (delete page, delete all) also require a confirmation tap
- Filenames are truncated at 32 characters in the list view

---

## Docker

```bash
docker compose up -d
```

The `compose.yaml` reads your `.env` file automatically and mounts `./data/` for the database, so it survives container rebuilds and updates.

```
./data/filestore.db   ← persisted on the host
```

---

## Code format

- Length: **9 characters**
- Charset: `a–z` + `0–9` (36 characters)
- Generated with `crypto/rand` — not sequential, not predictable
- Collision space: 36⁹ ≈ 101 billion possible codes
- Each new code is checked against the DB before use

---

## Project structure

```
filestore-bot/
├── main.go        ← entry point, update loop
├── config.go      ← env var loading & admin check
├── db.go          ← SQLite CRUD, code generation, write mutex
├── handlers.go    ← all command & callback handlers
├── keyboard.go    ← inline keyboard builders & text formatters
├── go.mod
├── compose.yaml
├── Dockerfile
└── .env.example
```

---

## Known limitations

- Telegram button labels max out at ~64 characters — filenames are truncated at 32 chars in the list view to fit the code, emoji, and date alongside
- `CopyMessage` will fail silently if the original source message is deleted from the upload chat — the bot will notify the user if this happens
- SQLite is single-writer; concurrent writes are serialised via mutex (this is fine for any realistic bot load)