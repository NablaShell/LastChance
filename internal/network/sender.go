package network

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// ============================================================
// URL СЕРВЕРОВ
// ============================================================

var ServerBaseURL string // Для сообщений (/push, /pull)
var FileServerURL string // Для файлов (/upload, /download)

func init() {
	// URL сервера сообщений
	if url := os.Getenv("SERVER_URL"); url != "" {
		ServerBaseURL = url
	} else {
		ServerBaseURL = "https://metrics-collector-7152.duckdns.org/api-v1"
	}

	// URL файлового сервера
	if url := os.Getenv("FILE_SERVER_URL"); url != "" {
		FileServerURL = url
	} else {
		FileServerURL = "https://metrics-collector-7152.duckdns.org/file-api"
	}
}

// ============================================================
// СТРУКТУРА SENDER
// ============================================================

type Sender struct {
	client *http.Client
}

func NewSender() *Sender {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS13,
			MaxVersion: tls.VersionTLS13,
		},
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	return &Sender{
		client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// ============================================================
// ОТПРАВКА СООБЩЕНИЙ (PUSH)
// ============================================================

// SendMessage отправляет зашифрованный пакет на сервер.
//
// Пакет должен быть одного из разрешённых размеров: 256, 1024, 4096, 65536.
// Это гарантируется crypto.EncryptMessage().
//
// При получении 429 (Rate Limit) делает до 3 повторных попыток
// с экспоненциальной задержкой: 1с → 2с → 4с.
func (s *Sender) SendMessage(targetHash string, packet []byte) error {
	url := fmt.Sprintf("%s/push/%s", ServerBaseURL, targetHash)

	maxRetries := 3
	baseDelay := 1 * time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Создаём новый reader для каждой попытки
		req, err := http.NewRequest("POST", url, bytes.NewReader(packet))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		// Заголовки
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set(MaskHeaderName, MaskHeaderValue)
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:149.0) Gecko/20100101 Firefox/149.0")

		// Выполняем запрос
		resp, err := s.client.Do(req)
		if err != nil {
			// Сетевая ошибка — пробуем ещё раз
			if attempt == maxRetries {
				return fmt.Errorf("request failed after %d retries: %w", maxRetries+1, err)
			}
			time.Sleep(baseDelay * time.Duration(1<<attempt))
			continue
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Printf("Error closing response body: %v", err)
			}
		}()

		// Успех
		if resp.StatusCode == http.StatusOK {
			return nil
		}

		// Rate limit — ждём и пробуем снова
		if resp.StatusCode == http.StatusTooManyRequests {
			if attempt == maxRetries {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("rate limited after %d retries: %s", maxRetries+1, string(body))
			}
			retryDelay := baseDelay * time.Duration(1<<attempt)
			fmt.Printf("[SENDER] Rate limited, retrying in %v (attempt %d/%d)\n", retryDelay, attempt+1, maxRetries+1)
			time.Sleep(retryDelay)
			continue
		}

		// Другая ошибка сервера — не повторяем
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	return fmt.Errorf("unexpected loop exit")
}

// ============================================================
// ОТПРАВКА ФАЙЛОВ (UPLOAD)
// ============================================================

// UploadFile отправляет зашифрованный файл на файловый сервер.
//
// Параметры:
//   - payload: зашифрованные данные файла (результат crypto.EncryptFile)
//   - fileName: оригинальное имя файла (для заголовка X-File-Name)
//
// Возвращает SHA256 хеш файла, присвоенный сервером.
// Этот хеш потом отправляется в чат для скачивания получателем.
//
// Лимиты:
//   - Клиентская проверка: максимум 100MB
//   - Серверная проверка: может быть меньше, зависит от конфигурации
func (s *Sender) UploadFile(payload []byte, fileName string) (string, error) {
	// Клиентская проверка лимита
	if len(payload) > 100*1024*1024 {
		return "", fmt.Errorf("file too large: %d bytes (max 100MB)", len(payload))
	}

	url := fmt.Sprintf("%s/upload", FileServerURL)

	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create upload request: %w", err)
	}

	// Заголовки
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-File-Name", fileName)
	req.Header.Set(MaskHeaderName, MaskHeaderValue)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:149.0) Gecko/20100101 Firefox/149.0")
	// Увеличенный таймаут для загрузки больших файлов
	uploadClient := &http.Client{
		Timeout: 120 * time.Second,
	}

	resp, err := uploadClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload request failed: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			log.Printf("[SENDER] Error closing response body: %v", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload failed: %s - %s", resp.Status, string(body))
	}

	// Сервер возвращает SHA256 хеш в теле ответа
	hashBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	fileHash := string(bytes.TrimSpace(hashBytes))

	return fileHash, nil
}
