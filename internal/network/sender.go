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
// SERVER URL
// ============================================================

var ServerBaseURL string // For messages (/push, /pull)
var FileServerURL string // For files (/upload, /download)

func init() {
	requiredEnvs := map[string]*string{
		"SERVER_URL":      &ServerBaseURL,
		"FILE_SERVER_URL": &FileServerURL,
	}

	for env, variable := range requiredEnvs {
		val := os.Getenv(env)
		if val == "" {
			log.Fatalf("[FATAL] Missing required environment variable: %s. Please check your .env file.", env)
		}
		*variable = val
	}
}

// ============================================================
// SENDER STRUCTURE
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
// SENDING MESSAGES (PUSH)
// ============================================================
// SendMessage sends an encrypted packet to the server.
//
// The packet must be one of the allowed sizes: 256, 1024, 4096, 65536.
// This is guaranteed by crypto.EncryptMessage().
//
// When receiving 429 (Rate Limit), retries up to 3 times
// with exponential delay: 1s → 2s → 4s.
func (s *Sender) SendMessage(targetHash string, packet []byte) error {
	url := fmt.Sprintf("%s/push/%s", ServerBaseURL, targetHash)

	maxRetries := 3
	baseDelay := 1 * time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Create a new reader for each attempt
		req, err := http.NewRequest("POST", url, bytes.NewReader(packet))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		// Headings
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set(MaskHeaderName, MaskHeaderValue)
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:149.0) Gecko/20100101 Firefox/149.0")

		// Execute the request
		resp, err := s.client.Do(req)
		if err != nil {
			// Network error - try again
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

		// Success
		if resp.StatusCode == http.StatusOK {
			return nil
		}

		// Rate limit — wait and try again
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

		//Another server error - we won’t repeat it
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	return fmt.Errorf("unexpected loop exit")
}

// ============================================================
// SENDING FILES (UPLOAD)
// ============================================================

// UploadFile uploads the encrypted file to the file server.
//
// Parameters:
// - payload: encrypted file data (result of crypto.EncryptFile)
// - fileName: original file name (for X-File-Name header)
//
// Returns the SHA256 hash of the file assigned by the server.
// This hash is then sent to the chat for download by the recipient.
//
// Limits:
// - Client check: maximum 100MB
// - Server check: may be less, depends on the configuration
func (s *Sender) UploadFile(payload []byte, fileName string) (string, error) {
	// Client-side limit check
	if len(payload) > 100*1024*1024 {
		return "", fmt.Errorf("file too large: %d bytes (max 100MB)", len(payload))
	}

	url := fmt.Sprintf("%s/upload", FileServerURL)

	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create upload request: %w", err)
	}

	// Headings
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-File-Name", fileName)
	req.Header.Set(MaskHeaderName, MaskHeaderValue)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:149.0) Gecko/20100101 Firefox/149.0")
	// Increased timeout for uploading large files
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

	// Server returns SHA256 hash in the response body
	hashBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	fileHash := string(bytes.TrimSpace(hashBytes))

	return fileHash, nil
}
