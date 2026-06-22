// internal/network/sender.go
package network

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// Sender handles pushing messages and uploading files to the server.
type Sender struct {
	client *http.Client
	config *Config
}

// NewSender creates a new Sender with the given configuration.
func NewSender(cfg *Config) *Sender {
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
		config: cfg,
		client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// SendMessage sends an encrypted packet to the server.
func (s *Sender) SendMessage(targetHash string, packet []byte) error {
	url := fmt.Sprintf("%s/push/%s", s.config.ServerURL, targetHash)

	maxRetries := 3
	baseDelay := 1 * time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest("POST", url, bytes.NewReader(packet))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set(s.config.MaskHeaderName, s.config.MaskHeaderValue)
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:149.0) Gecko/20100101 Firefox/149.0")

		resp, err := s.client.Do(req)
		if err != nil {
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

		if resp.StatusCode == http.StatusOK {
			return nil
		}

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

		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	return fmt.Errorf("unexpected loop exit")
}

// UploadFile uploads the encrypted file to the file server.
func (s *Sender) UploadFile(payload []byte, fileName string) (string, error) {
	if len(payload) > 100*1024*1024 {
		return "", fmt.Errorf("file too large: %d bytes (max 100MB)", len(payload))
	}

	url := fmt.Sprintf("%s/upload", s.config.FileServerURL)

	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create upload request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-File-Name", fileName)
	req.Header.Set(s.config.MaskHeaderName, s.config.MaskHeaderValue)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:149.0) Gecko/20100101 Firefox/149.0")

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

	hashBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	fileHash := string(bytes.TrimSpace(hashBytes))
	return fileHash, nil
}
