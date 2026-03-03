package main

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const codeCharset = "abcdefghijklmnopqrstuvwxyz0123456789"
const codeLength = 9

// FileRecord holds one stored file entry.
type FileRecord struct {
	Code      string
	ChatID    int64
	MessageID int
	FileName  string
	FileType  string
	CreatedAt time.Time
}

type DB struct {
	conn *sql.DB
}

func openDB(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
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
		CREATE TABLE IF NOT EXISTS files (
			code        TEXT    PRIMARY KEY,
			chat_id     INTEGER NOT NULL,
			message_id  INTEGER NOT NULL,
			file_name   TEXT    NOT NULL DEFAULT 'file',
			file_type   TEXT    NOT NULL DEFAULT 'document',
			created_at  TEXT    NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_files_created ON files(created_at DESC);
	`)
	return err
}

// generateCode produces a cryptographically random 9-char alphanumeric code
// that does not already exist in the database.
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
			"SELECT COUNT(*) FROM files WHERE code = ?", code,
		).Scan(&exists); err != nil {
			return "", err
		}
		if exists == 0 {
			return code, nil
		}
	}
	return "", fmt.Errorf("could not generate unique code after 10 attempts")
}

// Insert stores a new file record and returns its generated code.
func (db *DB) Insert(chatID int64, messageID int, fileName, fileType string) (string, error) {
	code, err := db.generateCode()
	if err != nil {
		return "", err
	}
	_, err = db.conn.Exec(
		`INSERT INTO files (code, chat_id, message_id, file_name, file_type, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		code, chatID, messageID, fileName, fileType,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return "", fmt.Errorf("insert: %w", err)
	}
	return code, nil
}

// Get looks up a file record by code. Returns nil, nil if not found.
func (db *DB) Get(code string) (*FileRecord, error) {
	row := db.conn.QueryRow(
		`SELECT code, chat_id, message_id, file_name, file_type, created_at
		 FROM files WHERE code = ?`, code,
	)
	return scanRecord(row)
}

// Count returns the total number of stored files.
func (db *DB) Count() (int, error) {
	var n int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM files").Scan(&n)
	return n, err
}

// Page returns one page of records ordered by created_at DESC.
func (db *DB) Page(offset, limit int) ([]*FileRecord, error) {
	rows, err := db.conn.Query(
		`SELECT code, chat_id, message_id, file_name, file_type, created_at
		 FROM files ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*FileRecord
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// Delete removes one record by code. Returns true if a row was deleted.
func (db *DB) Delete(code string) (bool, error) {
	res, err := db.conn.Exec("DELETE FROM files WHERE code = ?", code)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeleteMany removes multiple records by code. Returns count deleted.
func (db *DB) DeleteMany(codes []string) (int64, error) {
	if len(codes) == 0 {
		return 0, nil
	}
	tx, err := db.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare("DELETE FROM files WHERE code = ?")
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

// DeleteAll removes every record. Returns count deleted.
func (db *DB) DeleteAll() (int64, error) {
	res, err := db.conn.Exec("DELETE FROM files")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CodesOnPage returns just the codes for a given page (used for bulk-delete page).
func (db *DB) CodesOnPage(offset, limit int) ([]string, error) {
	rows, err := db.conn.Query(
		`SELECT code FROM files ORDER BY created_at DESC LIMIT ? OFFSET ?`,
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

// ── helpers ───────────────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...any) error
}

func scanRecord(s scanner) (*FileRecord, error) {
	var r FileRecord
	var createdAt string
	err := s.Scan(&r.Code, &r.ChatID, &r.MessageID, &r.FileName, &r.FileType, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &r, nil
}