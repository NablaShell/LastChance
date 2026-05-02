package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"messenger-wails/crypto"
	"messenger-wails/identity"
	"messenger-wails/network"
	"messenger-wails/storage"

	"github.com/dustin/go-humanize"
	"github.com/gen2brain/beeep"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ============================================================
// СТРУКТУРА ПРИЛОЖЕНИЯ
// ============================================================

type App struct {
	ctx    context.Context
	logger *slog.Logger

	// Хранилище
	storage   *storage.Storage
	idManager *identity.IdentityManager
	identity  *identity.Identity

	// E2EE
	sharedSecrets map[string][]byte
	sessionID     uint64
	sessionMutex  sync.Mutex

	// Сеть
	sender   *network.Sender
	listener *network.Listener

	// Ed25519 для авторизации Pull
	ed25519PrivKey   ed25519.PrivateKey
	ed25519PubKeyHex string

	// Отслеживание фокуса окна
	windowActive bool

	// UI state
	currentRoomHash string

	safeFS *storage.SafeFSOps // безопасные операции с ФС
}

func NewApp() *App {
	return &App{
		sharedSecrets: make(map[string][]byte),
	}
}

// ============================================================
// ЖИЗНЕННЫЙ ЦИКЛ
// ============================================================

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// --- Инициализация логгера ---
	a.logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// --- Путь к данным ---
	storagePath := os.Getenv("STORAGE_PATH")
	if storagePath == "" {
		storagePath = "./person_data"
	}

	// Инициализация безопасной ФС
	safeFS, err := storage.NewSafeFSOps(storagePath)
	if err != nil {
		log.Fatal("Failed to initialize safe filesystem:", err)
	}
	a.safeFS = safeFS

	// Создаём хранилище через SafeFSOps
	if err := safeFS.EnsureDir("."); err != nil {
		log.Fatal("Cannot create storage directory:", err)
	}

	// --- Загрузка или создание идентичности ---
	a.idManager = identity.NewIdentityManager(storagePath)
	id, err := a.idManager.LoadOrCreate("")
	if err != nil {
		log.Fatal("Failed to load/create identity:", err)
	}
	a.identity = id

	// Проверка Ed25519 ключей
	if len(a.identity.Ed25519PrivateKey) != ed25519.PrivateKeySize {
		log.Fatal("Invalid Ed25519 private key size — identity may be corrupted")
	}
	a.ed25519PrivKey = ed25519.PrivateKey(a.identity.Ed25519PrivateKey)
	a.ed25519PubKeyHex = a.identity.GetEd25519PublicKeyHex()

	// --- Инициализация хранилища ---
	store, err := storage.NewStorage(storagePath)
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}
	a.storage = store

	// --- Загрузка общих секретов ---
	contacts, _ := a.storage.GetAllContacts()
	for _, c := range contacts {
		secret, err := crypto.GenerateSharedKey(a.identity.PrivateKey, c.PublicKey)
		if err == nil {
			a.sharedSecrets[c.Hash] = secret
		} else {
			a.logger.Warn("Failed to generate shared secret",
				"hash", c.Hash[:16],
				"error", err,
			)
		}
	}

	// Установка текущей комнаты
	if len(contacts) > 0 {
		a.currentRoomHash = contacts[0].Hash
		a.storage.EnsureRoom(contacts[0].Nickname, contacts[0].Hash)
	}

	// --- Сетевые компоненты ---
	a.sender = network.NewSender()
	a.listener = network.NewListener()

	// Регистрация кастомного протокола
	runtime.EventsOn(ctx, "scheme-requested", func(data ...interface{}) {
		if len(data) > 0 {
			if rawURL, ok := data[0].(string); ok {
				a.handleSchemeURL(rawURL)
			}
		}
	})

	// --- Запуск фонового опроса ---
	go a.startMessageListener()

	a.logger.Info("App started",
		"nickname", a.identity.Nickname,
		"hash", a.identity.Hash[:16]+"...",
		"contacts", len(contacts),
	)
}

func (a *App) shutdown(_ context.Context) {
	if err := a.storage.Close(); err != nil {
		a.logger.Error("Failed to close storage", "error", err)
	}
	a.logger.Info("App stopped")
}

// ============================================================
// КАСТОМНЫЙ ПРОТОКОЛ lastchance://
// ============================================================

// handleSchemeURL обрабатывает URL кастомного протокола lastchance://
func (a *App) handleSchemeURL(rawURL string) {
	a.logger.Info("Scheme URL received", "url", rawURL)

	if !strings.HasPrefix(rawURL, "lastchance://contact?") {
		a.logger.Warn("Unknown scheme URL")
		return
	}

	// Извлекаем параметры
	paramsStr := strings.TrimPrefix(rawURL, "lastchance://contact?")
	params, err := url.ParseQuery(paramsStr)
	if err != nil {
		runtime.EventsEmit(a.ctx, "contact-import-error", map[string]interface{}{
			"error": "Invalid contact link format",
		})
		return
	}

	contactData := map[string]interface{}{
		"hash":     params.Get("hash"),
		"x25519":   params.Get("x25519"),
		"nickname": params.Get("nickname"),
	}

	if ed25519 := params.Get("ed25519"); ed25519 != "" {
		contactData["ed25519"] = ed25519
	}

	runtime.EventsEmit(a.ctx, "contact-import", contactData)
}

// GetMyContactLink возвращает ссылку lastchance:// для текущего профиля
func (a *App) GetMyContactLink() string {
	params := url.Values{}
	params.Set("hash", a.identity.Hash)
	params.Set("x25519", a.identity.GetPublicKeyHex())
	params.Set("ed25519", a.ed25519PubKeyHex)
	params.Set("nickname", a.identity.Nickname)
	return "lastchance://contact?" + params.Encode()
}

// GetMyContactJSON возвращает JSON с контактными данными
func (a *App) GetMyContactJSON() string {
	data := map[string]interface{}{
		"hash":     a.identity.Hash,
		"x25519":   a.identity.GetPublicKeyHex(),
		"ed25519":  a.ed25519PubKeyHex,
		"nickname": a.identity.Nickname,
	}
	jsonBytes, _ := json.MarshalIndent(data, "", "  ")
	return string(jsonBytes)
}

// ============================================================
// ФОНОВЫЙ ОПРОС СООБЩЕНИЙ
// ============================================================

func (a *App) startMessageListener() {
	stopChan := make(chan struct{})
	pullInterval := 5 * time.Second

	a.listener.StartListeningWithAuth(
		a.identity.Hash,
		a.ed25519PubKeyHex,
		a.ed25519PrivKey,
		func(packet []byte) {
			a.handleIncomingPacket(packet)
		},
		pullInterval,
		stopChan,
	)
}

func (a *App) handleIncomingPacket(packet []byte) {
	var message []byte
	var fromHash string
	var err error

	// Пробуем расшифровать всеми известными ключами
	for hash, secret := range a.sharedSecrets {
		message, _, err = crypto.DecryptMessage(packet, secret)
		if err == nil {
			fromHash = hash
			break
		}
	}

	// Если не получилось — пробуем как self-сообщение
	if err != nil {
		secret, _ := crypto.GenerateSharedKey(a.identity.PrivateKey, a.identity.PublicKey)
		message, _, err = crypto.DecryptMessage(packet, secret)
		if err != nil {
			a.logger.Warn("Failed to decrypt incoming packet", "error", err)
			return
		}
		fromHash = a.identity.Hash
	}

	// Имя отправителя
	contact, _ := a.storage.GetContact(fromHash)
	fromName := fromHash[:8]
	if contact != nil {
		fromName = contact.Nickname
	} else if fromHash == a.identity.Hash {
		fromName = a.identity.Nickname + " (self)"
	}

	// Сохраняем в БД
	_ = a.storage.SaveMessageWithRoom(fromHash, "in", fromHash, string(message))

	// Отправляем событие в UI
	runtime.EventsEmit(a.ctx, "new-message", map[string]interface{}{
		"roomHash":  fromHash,
		"direction": "in",
		"text":      string(message),
		"timestamp": time.Now(),
		"sender":    fromName,
	})

	// Системное уведомление (если не активная комната или окно не в фокусе)
	if fromHash != a.identity.Hash {
		if fromHash != a.currentRoomHash || !a.windowActive {
			title := fmt.Sprintf("📨 %s", fromName)
			body := string(message)
			if len(body) > 100 {
				body = body[:100] + "..."
			}
			if strings.HasPrefix(body, "📎 Файл:") {
				parts := strings.SplitN(body, "\n", 2)
				body = parts[0]
			}
			a.notifyUser(title, body)
		}
	}
}

// ============================================================
// СИСТЕМНЫЕ УВЕДОМЛЕНИЯ
// ============================================================

func (a *App) notifyUser(title, message string) {
	go func() {
		if err := beeep.Notify(title, message, ""); err != nil {
			a.logger.Warn("Failed to show notification", "error", err)
		}
	}()
}

// SetWindowActive вызывается из фронтенда при focus/blur окна
func (a *App) SetWindowActive(active bool) {
	a.windowActive = active
}

// ============================================================
// API ДЛЯ UI — ПРОФИЛЬ И КОНТАКТЫ
// ============================================================

// ... (previous imports unchanged)

// UpdateContactNickname changes the alias of a contact identified by hash.
// UpdateContactNickname changes the local alias/nickname of a contact.
// This only affects the local display name, not the cryptographic identity.
func (a *App) UpdateContactNickname(contactHash, newNickname string) error {
	if contactHash == a.identity.Hash {
		return fmt.Errorf("cannot rename your own identity this way; use UpdateNickname()")
	}
	if newNickname == "" {
		return fmt.Errorf("nickname cannot be empty")
	}
	return a.storage.UpdateContactNickname(contactHash, newNickname)
}

func (a *App) GetProfile() map[string]interface{} {
	return map[string]interface{}{
		"nickname":         a.identity.Nickname,
		"hash":             a.identity.Hash,
		"publicKey":        a.identity.GetPublicKeyHex(),
		"ed25519PublicKey": a.ed25519PubKeyHex,
		"seedPhrase":       a.identity.SeedPhrase,
	}
}

func (a *App) GetContacts() []storage.Contact {
	contacts, err := a.storage.GetAllContacts()
	if err != nil || contacts == nil {
		return []storage.Contact{}
	}
	return contacts
}

func (a *App) AddContact(hash, pubKeyHex, nickname string) error {
	pubKey, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return fmt.Errorf("invalid public key hex: %w", err)
	}
	if len(pubKey) != 32 {
		return fmt.Errorf("public key must be 32 bytes, got %d", len(pubKey))
	}

	sharedSecret, err := crypto.GenerateSharedKey(a.identity.PrivateKey, pubKey)
	if err != nil {
		return fmt.Errorf("failed to generate shared secret: %w", err)
	}

	if err := a.storage.AddContact(hash, pubKey, nickname); err != nil {
		return err
	}

	a.sharedSecrets[hash] = sharedSecret
	a.storage.EnsureRoom(nickname, hash)

	if a.currentRoomHash == "" {
		a.currentRoomHash = hash
	}

	return nil
}

func (a *App) UpdateNickname(newNickname string) error {
	if err := a.idManager.UpdateNickname(newNickname); err != nil {
		return err
	}
	a.identity.Nickname = newNickname
	return nil
}

// ============================================================
// API ДЛЯ UI — СООБЩЕНИЯ
// ============================================================

func (a *App) SendMessage(roomHash, text string) error {
	sharedSecret, err := a.getSharedSecret(roomHash)
	if err != nil {
		return err
	}

	a.sessionMutex.Lock()
	a.sessionID++
	sessionID := a.sessionID
	a.sessionMutex.Unlock()

	packet, err := crypto.EncryptMessage([]byte(text), sharedSecret, sessionID)
	if err != nil {
		return fmt.Errorf("encrypt message: %w", err)
	}

	if err := a.sender.SendMessage(roomHash, packet); err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	_ = a.storage.SaveMessageWithRoom(roomHash, "out", roomHash, text)
	return nil
}

func (a *App) GetMessages(roomHash string, limit int) []storage.Message {
	msgs, err := a.storage.GetRoomMessages(roomHash, limit)
	if err != nil {
		a.logger.Error("Failed to fetch messages", "room", roomHash, "error", err)
		return []storage.Message{}
	}

	if len(msgs) == 0 {
		return []storage.Message{}
	}

	reversed := make([]storage.Message, len(msgs))
	for i, m := range msgs {
		reversed[len(msgs)-1-i] = m
	}
	return reversed
}

func (a *App) SwitchRoom(roomHash string) {
	a.currentRoomHash = roomHash
}

func (a *App) GetCurrentRoom() string {
	return a.currentRoomHash
}

// ============================================================
// API ДЛЯ UI — ФАЙЛЫ
// ============================================================

func (a *App) SendFileNative(roomHash string) error {
	filePath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Выберите файл для отправки",
		Filters: []runtime.FileFilter{
			{DisplayName: "Все файлы", Pattern: "*.*"},
			{DisplayName: "Изображения", Pattern: "*.jpg;*.jpeg;*.png;*.gif;*.webp;*.bmp"},
			{DisplayName: "Видео", Pattern: "*.mp4;*.webm;*.mov;*.avi"},
			{DisplayName: "Документы", Pattern: "*.pdf;*.doc;*.docx;*.txt;*.md"},
		},
	})
	if err != nil || filePath == "" {
		return fmt.Errorf("файл не выбран")
	}

	fileName := filepath.Base(filePath)

	runtime.EventsEmit(a.ctx, "upload-started", map[string]interface{}{
		"fileName": fileName,
	})

	file, err := a.safeFS.SafeOpenFile(filePath)
	if err != nil {
		return fmt.Errorf("не удалось открыть файл: %w", err)
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			a.logger.Error("Failed to close file", "error", cerr)
		}
	}()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("не удалось получить информацию о файле: %w", err)
	}
	fileSize := stat.Size()

	sharedSecret, err := a.getSharedSecret(roomHash)
	if err != nil {
		return err
	}

	fileData, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("не удалось прочитать файл: %w", err)
	}

	runtime.EventsEmit(a.ctx, "upload-progress", map[string]interface{}{
		"progress": 30,
		"status":   "Шифрование...",
	})

	encryptedData, err := crypto.EncryptFile(fileData, sharedSecret)
	if err != nil {
		return fmt.Errorf("ошибка шифрования: %w", err)
	}

	runtime.EventsEmit(a.ctx, "upload-progress", map[string]interface{}{
		"progress": 60,
		"status":   "Отправка на сервер...",
	})

	fileHash, err := a.sender.UploadFile(encryptedData, fileName)
	if err != nil {
		runtime.EventsEmit(a.ctx, "upload-error", map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("ошибка загрузки: %w", err)
	}

	_ = a.storage.SaveFileRecord(fileHash, roomHash, fileName, fileSize)

	runtime.EventsEmit(a.ctx, "upload-progress", map[string]interface{}{
		"progress": 100,
		"status":   "Завершено",
	})

	messageText := fmt.Sprintf("📎 Файл: %s\nРазмер: %s\nХеш: %s",
		fileName,
		safeHumanize(fileSize), // новая функция
		fileHash,
	)
	if err := a.SendMessage(roomHash, messageText); err != nil {
		a.logger.Error("Не удалось отправить уведомление о файле", "error", err)
	}

	runtime.EventsEmit(a.ctx, "upload-completed", map[string]interface{}{
		"fileHash": fileHash,
		"fileName": fileName,
		"fileSize": fileSize,
	})

	return nil
}

func (a *App) DownloadAndSaveFile(fileHash, suggestedFileName string) error {
	runtime.EventsEmit(a.ctx, "download-started", map[string]interface{}{
		"fileHash": fileHash,
	})

	encryptedData, err := a.listener.DownloadFile(fileHash)
	if err != nil {
		runtime.EventsEmit(a.ctx, "download-error", map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("ошибка скачивания: %w", err)
	}

	a.logger.Info("Файл скачан",
		"hash", fileHash,
		"size", len(encryptedData),
	)

	runtime.EventsEmit(a.ctx, "download-progress", map[string]interface{}{
		"progress": 40,
		"status":   "Расшифровка...",
	})

	sharedSecret, ok := a.sharedSecrets[a.currentRoomHash]
	if !ok {
		return fmt.Errorf("нет ключа для расшифровки")
	}

	fileData, err := crypto.DecryptFile(encryptedData, sharedSecret)
	if err != nil {
		a.logger.Error("Ошибка расшифровки файла",
			"hash", fileHash,
			"error", err,
		)
		runtime.EventsEmit(a.ctx, "download-error", map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("ошибка расшифровки: %w", err)
	}

	runtime.EventsEmit(a.ctx, "download-progress", map[string]interface{}{
		"progress": 80,
		"status":   "Сохранение...",
	})

	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Сохранить файл как",
		DefaultFilename: suggestedFileName,
	})
	if err != nil || savePath == "" {
		return nil
	}

	if err := a.safeFS.SafeWriteFile(savePath, fileData, 0600); err != nil {
		runtime.EventsEmit(a.ctx, "download-error", map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("ошибка сохранения: %w", err)
	}

	runtime.EventsEmit(a.ctx, "download-completed", map[string]interface{}{
		"filePath": savePath,
	})

	return nil
}

func safeHumanize(fileSize int64) string {
	if fileSize < 0 {
		return "0 B (ошибка)"
	}
	if fileSize > math.MaxInt64/2 { // защита от огромных чисел
		return fmt.Sprintf("%d B", fileSize)
	}
	return humanize.Bytes(uint64(fileSize))
}

// ============================================================
// ВСПОМОГАТЕЛЬНЫЕ МЕТОДЫ
// ============================================================

func (a *App) getSharedSecret(roomHash string) ([]byte, error) {
	if secret, ok := a.sharedSecrets[roomHash]; ok {
		return secret, nil
	}

	if roomHash == a.identity.Hash {
		secret, err := crypto.GenerateSharedKey(a.identity.PrivateKey, a.identity.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("failed to generate self shared secret: %w", err)
		}
		a.sharedSecrets[roomHash] = secret
		return secret, nil
	}

	contact, err := a.storage.GetContact(roomHash)
	if err != nil {
		return nil, fmt.Errorf("contact not found: %s", roomHash[:16])
	}

	secret, err := crypto.GenerateSharedKey(a.identity.PrivateKey, contact.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to generate shared secret: %w", err)
	}

	a.sharedSecrets[roomHash] = secret
	return secret, nil
}
