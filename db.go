package main

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const codeCharset = "abcdefghijklmnopqrstuvwxyz0123456789"
const codeLength = 9

// BundleMessage is one message within a bundle.
type BundleMessage struct {
	ID        int64
	Code      string
	ChatID    int64
	MessageID int
	FileName  string
	FileType  string
	Position  int
}

// Bundle is the top-level record for a code.
type Bundle struct {
	Code      string
	FileCount int
	CreatedAt time.Time
}

type DB struct {
	conn *sql.DB
	mu   sync.Mutex
}

func openDB(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func (db *DB) migrate() error {
	_, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS bundles (
			code       TEXT PRIMARY KEY,
			created_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS bundle_messages (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			code       TEXT    NOT NULL REFERENCES bundles(code) ON DELETE CASCADE,
			chat_id    INTEGER NOT NULL,
			message_id INTEGER NOT NULL,
			file_name  TEXT    NOT NULL DEFAULT 'file',
			file_type  TEXT    NOT NULL DEFAULT 'document',
			position   INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_bm_code ON bundle_messages(code, position);
		PRAGMA foreign_keys = ON;
	`)
	return err
}

func (db *DB) generateCode() (string, error) {
	charsetLen := big.NewInt(int64(len(codeCharset)))
	for attempts := 0; attempts < 10; attempts++ {
		buf := make([]byte, codeLength)
		for i := range buf {
			n, err := rand.Int(rand.Reader, charsetLen)
			if err != nil {
				return "", fmt.Errorf("rand: %w", err)
			}
			buf[i] = codeCharset[n.Int64()]
		}
		code := string(buf)
		var exists int
		if err := db.conn.QueryRow(
			"SELECT COUNT(*) FROM bundles WHERE code = ?", code,
		).Scan(&exists); err != nil {
			return "", err
		}
		if exists == 0 {
			return code, nil
		}
	}
	return "", fmt.Errorf("could not generate unique code after 10 attempts")
}

// InsertBundle saves a completed bundle and all its messages atomically.
func (db *DB) InsertBundle(messages []BundleMessage) (string, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	code, err := db.generateCode()
	if err != nil {
		return "", err
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		"INSERT INTO bundles (code, created_at) VALUES (?, ?)",
		code, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return "", fmt.Errorf("insert bundle: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO bundle_messages (code, chat_id, message_id, file_name, file_type, position)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return "", err
	}
	defer stmt.Close()

	for i, m := range messages {
		if _, err := stmt.Exec(code, m.ChatID, m.MessageID, m.FileName, m.FileType, i); err != nil {
			return "", fmt.Errorf("insert message %d: %w", i, err)
		}
	}

	return code, tx.Commit()
}

// GetBundle returns all messages for a code ordered by position. nil,nil if not found.
func (db *DB) GetBundle(code string) ([]BundleMessage, error) {
	var exists int
	if err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM bundles WHERE code = ?", code,
	).Scan(&exists); err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, nil
	}

	rows, err := db.conn.Query(`
		SELECT id, code, chat_id, message_id, file_name, file_type, position
		FROM bundle_messages WHERE code = ? ORDER BY position ASC
	`, code)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []BundleMessage
	for rows.Next() {
		var m BundleMessage
		if err := rows.Scan(&m.ID, &m.Code, &m.ChatID, &m.MessageID, &m.FileName, &m.FileType, &m.Position); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func (db *DB) CountBundles() (int, error) {
	var n int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM bundles").Scan(&n)
	return n, err
}

func (db *DB) PageBundles(offset, limit int) ([]*Bundle, error) {
	rows, err := db.conn.Query(`
		SELECT b.code, COUNT(m.id) as file_count, b.created_at
		FROM bundles b
		LEFT JOIN bundle_messages m ON m.code = b.code
		GROUP BY b.code
		ORDER BY b.created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bundles []*Bundle
	for rows.Next() {
		var bun Bundle
		var createdAt string
		if err := rows.Scan(&bun.Code, &bun.FileCount, &createdAt); err != nil {
			return nil, err
		}
		bun.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		bundles = append(bundles, &bun)
	}
	return bundles, rows.Err()
}

func (db *DB) DeleteBundle(code string) (bool, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.conn.Exec("PRAGMA foreign_keys = ON")
	res, err := db.conn.Exec("DELETE FROM bundles WHERE code = ?", code)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (db *DB) DeleteBundles(codes []string) (int64, error) {
	if len(codes) == 0 {
		return 0, nil
	}
	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	tx.Exec("PRAGMA foreign_keys = ON")

	stmt, err := tx.Prepare("DELETE FROM bundles WHERE code = ?")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	var total int64
	for _, code := range codes {
		res, err := stmt.Exec(code)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, tx.Commit()
}

func (db *DB) DeleteAllBundles() (int64, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.conn.Exec("PRAGMA foreign_keys = ON")
	res, err := db.conn.Exec("DELETE FROM bundles")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (db *DB) CodesOnPage(offset, limit int) ([]string, error) {
	rows, err := db.conn.Query(
		"SELECT code FROM bundles ORDER BY created_at DESC LIMIT ? OFFSET ?",
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var codes []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		codes = append(codes, c)
	}
	return codes, rows.Err()
}