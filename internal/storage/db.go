package storage

import (
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const (
	DBFileName = "user.db"
)

// ============================================================
// DATA MODELS
// ============================================================

type Contact struct {
	Hash      string    `json:"hash"`
	PublicKey []byte    `json:"publicKey"`
	Nickname  string    `json:"nickname"`
	CreatedAt time.Time `json:"createdAt"`
}

type Message struct {
	ID          int64     `json:"id"`
	ContactHash string    `json:"contactHash"`
	RoomHash    string    `json:"roomHash"`
	Direction   string    `json:"direction"` // "in" or "out"
	Text        string    `json:"text"`
	Timestamp   time.Time `json:"timestamp"`
	Status      string    `json:"status"`
}

type FileRecord struct {
	FileHash   string    `json:"fileHash"`
	RoomHash   string    `json:"roomHash"`
	FileName   string    `json:"fileName"`
	FileSize   int64     `json:"fileSize"`
	UploadedAt time.Time `json:"uploadedAt"`
}

// ============================================================
// STORAGE
// ============================================================

type Storage struct {
	db *sql.DB
}

func NewStorage(storagePath string) (*Storage, error) {
	dbPath := filepath.Join(storagePath, DBFileName)
	//DSN for modernc.org/sqlite (pure Go, no CGO)
	//WAL mode for concurrent access
	//busy_timeout for waiting for locks

	dsn := dbPath + "?_journal_mode=WAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Checking the connection
	if err := db.Ping(); err != nil {
		if err := db.Close(); err != nil {
			log.Printf("Error closing DB after failed ping: %v", err)
		}
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Setting up a connection pool
	//SQLite only supports one connection per record
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	storage := &Storage{db: db}

	// Создаём таблицы
	if err := storage.InitTables(); err != nil {
		if err := db.Close(); err != nil {
			log.Printf("Error closing DB after failed ping: %v", err)
		}
		return nil, err
	}

	return storage, nil
}

// ============================================================
// ИНИЦИАЛИЗАЦИЯ ТАБЛИЦ
// ============================================================

func (s *Storage) InitTables() error {
	// Включаем foreign keys (по умолчанию выключены в SQLite)
	if _, err := s.db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}

	// Contact table
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS contacts (
			hash TEXT PRIMARY KEY,
			public_key BLOB NOT NULL,
			nickname TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		return fmt.Errorf("create contacts table: %w", err)
	}

	// Room table (for fast search by nickname)
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS rooms (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			contact_hash TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(contact_hash) REFERENCES contacts(hash)
		)
	`); err != nil {
		return fmt.Errorf("create rooms table: %w", err)
	}

	// Message table
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			contact_hash TEXT NOT NULL,
			room_hash TEXT NOT NULL,
			direction TEXT NOT NULL,
			text TEXT NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			status TEXT DEFAULT 'delivered',
			FOREIGN KEY(contact_hash) REFERENCES contacts(hash),
			FOREIGN KEY(room_hash) REFERENCES contacts(hash)
		)
	`); err != nil {
		return fmt.Errorf("create messages table: %w", err)
	}

	// File table
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS files (
			file_hash TEXT PRIMARY KEY,
			room_hash TEXT NOT NULL,
			file_name TEXT NOT NULL,
			file_size INTEGER NOT NULL,
			uploaded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(room_hash) REFERENCES contacts(hash)
		)
	`); err != nil {
		return fmt.Errorf("create files table: %w", err)
	}

	// Indices
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_messages_contact ON messages(contact_hash)",
		"CREATE INDEX IF NOT EXISTS idx_messages_room ON messages(room_hash)",
		"CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_files_room ON files(room_hash)",
		"CREATE INDEX IF NOT EXISTS idx_files_uploaded ON files(uploaded_at)",
	}

	for _, idx := range indexes {
		if _, err := s.db.Exec(idx); err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}

	return nil
}

// ============================================================
// CONTACTS
// ============================================================

// AddContact adds a new contact or updates an existing one.
func (s *Storage) AddContact(hash string, publicKey []byte, nickname string) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO contacts (hash, public_key, nickname) VALUES (?, ?, ?)",
		hash, publicKey, nickname,
	)
	return err
}

// GetContact gets a contact by hash.
func (s *Storage) GetContact(hash string) (*Contact, error) {
	var contact Contact
	err := s.db.QueryRow(
		"SELECT hash, public_key, nickname, created_at FROM contacts WHERE hash = ?",
		hash,
	).Scan(&contact.Hash, &contact.PublicKey, &contact.Nickname, &contact.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("contact not found: %s", hash[:16])
		}
		return nil, fmt.Errorf("query contact: %w", err)
	}
	return &contact, nil
}

// GetAllContacts receives all contacts (new ones first).
func (s *Storage) GetAllContacts() ([]Contact, error) {
	rows, err := s.db.Query(
		"SELECT hash, public_key, nickname, created_at FROM contacts ORDER BY created_at DESC",
	)
	if err != nil {
		return nil, fmt.Errorf("query contacts: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()
	var contacts []Contact
	for rows.Next() {
		var c Contact
		if err := rows.Scan(&c.Hash, &c.PublicKey, &c.Nickname, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan contact: %w", err)
		}
		contacts = append(contacts, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contacts: %w", err)
	}

	return contacts, nil
}

// GetContactHashByNickname searches for a hash of a contact by nickname.
// First checks the rooms table, thencontacts.
func (s *Storage) GetContactHashByNickname(nickname string) (string, error) {
	var hash string

	// First we look in rooms
	err := s.db.QueryRow(
		"SELECT contact_hash FROM rooms WHERE name = ?", nickname,
	).Scan(&hash)
	if err == nil {
		return hash, nil
	}

	// Fallback: search in contacts
	err = s.db.QueryRow(
		"SELECT hash FROM contacts WHERE nickname = ?", nickname,
	).Scan(&hash)
	if err == nil {
		// Automatically create a record in rooms for future searches
		_, _ = s.db.Exec(
			"INSERT OR IGNORE INTO rooms (name, contact_hash) VALUES (?, ?)",
			nickname, hash,
		)
		return hash, nil
	}

	return "", fmt.Errorf("contact not found: %s", nickname)
}

// EnsureRoom ensures that the room exists in the rooms table.
func (s *Storage) EnsureRoom(nickname, hash string) {
	_, _ = s.db.Exec(
		"INSERT OR IGNORE INTO rooms (name, contact_hash) VALUES (?, ?)",
		nickname, hash,
	)
}

// ============================================================
// MESSAGES
// ============================================================

// SaveMessageWithRoom saves a message with a specified room_hash.
func (s *Storage) SaveMessageWithRoom(contactHash, direction, roomHash, text string) error {
	_, err := s.db.Exec(
		"INSERT INTO messages (contact_hash, direction, room_hash, text, status) VALUES (?, ?, ?, ?, ?)",
		contactHash, direction, roomHash, text, "delivered",
	)
	return err
}

// SaveMessage saves a message (for backward compatibility).
func (s *Storage) SaveMessage(contactHash, direction, text string) error {
	return s.SaveMessageWithRoom(contactHash, direction, contactHash, text)
}

// GetRoomMessages returns messages for a room (new ones first).
func (s *Storage) GetRoomMessages(roomHash string, limit int) ([]Message, error) {
	rows, err := s.db.Query(`
		SELECT id, contact_hash, direction, text, timestamp, status
		FROM messages
		WHERE room_hash = ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, roomHash, limit)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()
	var messages []Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(
			&msg.ID, &msg.ContactHash, &msg.Direction,
			&msg.Text, &msg.Timestamp, &msg.Status,
		); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		msg.RoomHash = roomHash
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}

	return messages, nil
}

// GetMessages returns messages (for backward compatibility).
func (s *Storage) GetMessages(contactHash string, limit int) ([]Message, error) {
	return s.GetRoomMessages(contactHash, limit)
}

// ============================================================
// FILES
// ============================================================

// SaveFileRecord saves information about an uploaded file.
func (s *Storage) SaveFileRecord(fileHash, roomHash, fileName string, fileSize int64) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO files (file_hash, room_hash, file_name, file_size, uploaded_at)
		VALUES (?, ?, ?, ?, ?)
	`, fileHash, roomHash, fileName, fileSize, time.Now())
	return err
}

// GetFileRecord gets information about a file by hash.
func (s *Storage) GetFileRecord(fileHash string) (*FileRecord, error) {
	var fr FileRecord
	err := s.db.QueryRow(`
		SELECT file_hash, room_hash, file_name, file_size, uploaded_at
		FROM files WHERE file_hash = ?
	`, fileHash).Scan(
		&fr.FileHash, &fr.RoomHash, &fr.FileName, &fr.FileSize, &fr.UploadedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("file record not found: %s", fileHash[:16])
		}
		return nil, fmt.Errorf("query file record: %w", err)
	}
	return &fr, nil
}

// GetRoomFiles returns all files for a room.
func (s *Storage) GetRoomFiles(roomHash string) ([]FileRecord, error) {
	rows, err := s.db.Query(`
		SELECT file_hash, room_hash, file_name, file_size, uploaded_at
		FROM files WHERE room_hash = ?
		ORDER BY uploaded_at DESC
	`, roomHash)
	if err != nil {
		return nil, fmt.Errorf("query room files: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var files []FileRecord
	for rows.Next() {
		var fr FileRecord
		if err := rows.Scan(
			&fr.FileHash, &fr.RoomHash, &fr.FileName, &fr.FileSize, &fr.UploadedAt,
		); err != nil {
			return nil, fmt.Errorf("scan file record: %w", err)
		}
		files = append(files, fr)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate files: %w", err)
	}

	return files, nil
}

// ============================================================
// CLOSE
// ============================================================

// Close closes the connection to the database.
func (s *Storage) Close() error {
	return s.db.Close()
}

// UpdateContactNickname changes the stored nickname for a contact.
func (s *Storage) UpdateContactNickname(contactHash, newNickname string) error {
	result, err := s.db.Exec(
		"UPDATE contacts SET nickname = ? WHERE hash = ?",
		newNickname, contactHash,
	)
	if err != nil {
		return fmt.Errorf("update contact nickname: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("contact not found: %s", contactHash[:16])
	}
	return nil
}
