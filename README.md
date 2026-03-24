# Telegram Forwarder Bot

> [!IMPORTANT]
> ## ⚠️ AI-Generated Project
> **This entire project — every line of code, every config file, and this README — was written by [Claude](https://claude.ai) (Anthropic). It was not written by a human developer.**
>
> No code was manually authored. The bot was built iteratively through a conversation with Claude, including debugging, fixing driver compatibility issues, resolving SQLite concurrency bugs, schema refactors, and UI tweaks — all AI-generated.
>
> Use in production at your own discretion.

---

A Telegram bot that lets admins store bundles of files behind anonymous 9-character codes.
Users retrieve files with `/send <code>` — they never see who uploaded the files or from which chat.

---

## Features

| Feature | Detail |
|---|---|
| **Bundle support** | One code can deliver multiple files, sent to the user in order |
| **Automatic sorting** | Files in a bundle are always sorted in natural alphanumeric order at save time — `S01E02` before `S01E10`, regardless of the order Telegram forwarded them |
| **Debounced bulk feedback** | When forwarding many files at once, the bot waits 1 second after the last file arrives before sending a single sorted summary — no per-file message spam |
| **Anonymous delivery** | Uses `CopyMessage` (not `ForwardMessage`), so no source chat or uploader is ever revealed |
| **Random codes** | Cryptographically random 9-char alphanumeric (`a–z`, `0–9`) via `crypto/rand`, collision-checked |
| **Supported file types** | Documents, photos, videos, audio, voice notes, video notes, animations, stickers |
| **Paginated admin list** | `/listfiles` with inline keyboard — handles hundreds of bundles cleanly |
| **Bulk delete** | Delete by code(s), delete whole page, or wipe everything — all with confirm prompts |
| **Concurrent-safe writes** | `sync.Mutex` on all DB writes so bulk uploads never deadlock |
| **Pure-Go SQLite** | Uses `modernc.org/sqlite` — no CGO, no gcc, works on Windows out of the box |
| **Deep links** | Every bundle gets a `t.me/yourbot?start=<code>` link for easy sharing |
| **Docker ready** | Single binary, minimal Alpine image, `compose.yaml` included |

---

## Quick Start

### Prerequisites

- Go 1.23+
- A bot token from [@BotFather](https://t.me/BotFather)
- Your Telegram user ID — get it from [@userinfobot](https://t.me/userinfobot)

### 1. Clone & configure

```bash
git clone https://github.com/driftywinds/tg-forwarder
cd tg-forwarder
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
go build -o tg-forwarder.exe .
```

---

## Configuration

All config is via `.env` file (or plain environment variables):

| Variable | Required | Default | Description |
|---|---|---|---|
| `BOT_TOKEN` | ✅ | — | Token from @BotFather |
| `ADMIN_IDS` | ✅ | — | Comma-separated Telegram user IDs |
| `DB_PATH` | ❌ | `filestore.db` | Path to SQLite database file |
| `PAGE_SIZE` | ❌ | `8` | Bundles shown per page in `/listfiles` |

---

## Commands

### User commands
| Command | Description |
|---|---|
| `/send <code>` | Retrieve all files in a bundle by its 9-char code |
| `/help` | Show help |

### Admin commands
| Command | Description |
|---|---|
| `/begin` | Start a new bundle recording session |
| `/done` | End the session, save the bundle, receive the code |
| `/cancel` | Discard the current bundle without saving |
| `/listfiles` | Browse all stored bundles with inline pagination |
| `/delete code1 [code2 …]` | Delete one or more bundles by code |
| `/stats` | Show total count, newest/oldest bundle |
| `/help` | Show admin help |

---

## Admin workflow

Creating a bundle is a three-step process:

```
/begin
```
The bot confirms the session is open. Now send as many files as you want — one at a time or in bulk.

For single files the bot responds immediately. For bulk forwards (e.g. 20 files forwarded at once), the bot stays silent while files are arriving and sends **one consolidated message** 1 second after the last file lands, showing the full sorted list:
```
📦 20 files queued — sorted order:

1. `Heated_Rivalry_S01E01_Rookies_1080p...`
2. `Heated_Rivalry_S01E02_Olympians_108...`
3. `Heated_Rivalry_S01E03_1080p_AMZN_WE...`
...
20. `Heated_Rivalry_S01E20_...`

Send more files, or /done to save.
```
Files are sorted using natural alphanumeric order — dot, dash and underscore separators are treated as equivalent, and numeric segments are compared as integers so `S01E09` always sorts before `S01E10`.

When all files are sent:
```
/done
```
The bot saves the bundle and replies with the code and a ready-to-share deep link:
```
✅ Bundle saved!

Code: abcd56789
Files: 10

Share this code or link:
/send abcd56789
🔗 https://t.me/yourbotname?start=abcd56789
```

To discard a bundle mid-session without saving:
```
/cancel
```

> Sending a file to the bot **outside** of a `/begin` session will not create a code — the bot will prompt you to use `/begin` first.

---

## Deep links

Every bundle gets a shareable Telegram deep link:

```
https://t.me/yourbotname?start=<code>
```

When a user clicks the link, Telegram opens the bot and automatically sends `/start <code>`, which delivers all files in the bundle. If the user has never opened the bot before, Telegram shows a **Start** button they must tap first — this is a Telegram platform restriction and cannot be bypassed.

---

## Admin UI — `/listfiles`

```
📂 Bundle Store — 10 bundles
Page 1 of 2 · showing 1–8 · tap a row to delete

🗑  z449nia67  ·  10 files     ·  03 Mar 26
🗑  r6ldh81sf  ·  4 files      ·  03 Mar 26
🗑  pf9vmysdi  ·  1 file       ·  03 Mar 26
...

[ 1 / 2 ]  [ Next ▶ ]
[ 🗑 Delete This Page ]  [ 💣 Delete ALL ]  [ ✖ Close ]
```

Tapping a bundle row shows a confirm/cancel prompt before deleting. Bulk actions also require a confirmation tap.

---

## Docker

```bash
docker compose up -d
```

The `compose.yaml` reads your `.env` file automatically and mounts `./data/` on the host for the database, so it survives container rebuilds.

```
./data/filestore.db   ← persisted on the host
```

---

## Database schema

Two tables, with cascade deletes so removing a bundle cleans up all its messages automatically:

```sql
bundles (code, created_at)
bundle_messages (id, code → bundles, chat_id, message_id, file_name, file_type, position)
```

> ⚠️ If you were using an earlier version of this bot with the single-file schema, delete your `filestore.db` and let it recreate — the schemas are not compatible.

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
tg-forwarder/
├── main.go        ← entry point, update loop
├── config.go      ← env var loading & admin check
├── db.go          ← SQLite CRUD, bundle schema, code generation, write mutex
├── session.go     ← in-memory recording session store (thread-safe)
├── handlers.go    ← all command & callback handlers
├── sort.go        ← natural alphanumeric sort for bundle filenames
├── keyboard.go    ← inline keyboard builders & text formatters
├── go.mod
├── compose.yaml
├── Dockerfile
└── .env.example
```

---

## Known limitations

- `CopyMessage` will fail silently if the original source message is deleted from the upload chat — the bot will notify the user per-file if this happens
- SQLite is single-writer; concurrent writes are serialised via mutex (fine for any realistic bot load)
- Telegram button labels have a ~64 character cap; bundle list rows show code, file count, and date only