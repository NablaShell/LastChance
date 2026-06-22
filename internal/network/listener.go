// internal/network/listener.go
package network

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"
)

// Listener handles pulling messages and downloading files.
type Listener struct {
	client *http.Client
	config *Config // использует общую структуру Config из config.go
}

func NewListener(cfg *Config) *Listener {
	return &Listener{
		config: cfg,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// PullMessagesWithAuth pulls messages from the server with an Ed25519 signature.
func (l *Listener) PullMessagesWithAuth(
	identityHash, pubKeyHex string,
	privKey ed25519.PrivateKey,
) ([][]byte, error) {
	now := time.Now().Unix()
	message := fmt.Sprintf("%d%s", now, identityHash)
	signature := ed25519.Sign(privKey, []byte(message))
	sigHex := hex.EncodeToString(signature)

	url := fmt.Sprintf("%s/pull/%s", l.config.ServerURL, identityHash)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create pull request: %w", err)
	}

	req.Header.Set("X-Identity-Key", pubKeyHex)
	req.Header.Set("X-Timestamp", strconv.FormatInt(now, 10))
	req.Header.Set("X-Signature", sigHex)

	req.Header.Set(l.config.MaskHeaderName, l.config.MaskHeaderValue)
	req.Header.Set("Accept", "multipart/form-data")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Arch Linux; rv:130.0) Gecko/20100101 Firefox/130.0")

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pull request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("rate limited (429)")
	}

	if resp.StatusCode == http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unauthorized (401): %s", string(body))
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	contentType := resp.Header.Get("Content-Type")
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		// fallback: read as plain binary
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}
		if len(data) == 0 {
			return nil, nil
		}
		return [][]byte{data}, nil
	}

	boundary, ok := params["boundary"]
	if !ok {
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}
		if len(data) == 0 {
			return nil, nil
		}
		return [][]byte{data}, nil
	}

	reader := multipart.NewReader(resp.Body, boundary)
	var messages [][]byte

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("multipart next part: %w", err)
		}

		data, err := io.ReadAll(part)
		closeErr := part.Close()
		if closeErr != nil {
			fmt.Printf("[LISTENER] Failed to close part: %v\n", closeErr)
		}
		if err != nil {
			return nil, fmt.Errorf("read part: %w", err)
		}

		if len(data) > 0 {
			messages = append(messages, data)
		}
	}

	return messages, nil
}

// StartListeningWithAuth starts a background queue poll with Ed25519 authorization.
func (l *Listener) StartListeningWithAuth(
	identityHash, pubKeyHex string,
	privKey ed25519.PrivateKey,
	onMessage func([]byte),
	baseInterval time.Duration,
	stopChan <-chan struct{},
) {
	currentInterval := baseInterval
	ticker := time.NewTicker(currentInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			messages, err := l.PullMessagesWithAuth(identityHash, pubKeyHex, privKey)
			if err != nil {
				fmt.Printf("[LISTENER] Pull error: %v\n", err)
				if err.Error() == "rate limited (429)" {
					currentInterval = minDuration(currentInterval*2, 60*time.Second)
					ticker.Reset(currentInterval)
					fmt.Printf("[LISTENER] Rate limited, increasing interval to %v\n", currentInterval)
				}
				continue
			}

			if currentInterval != baseInterval {
				currentInterval = baseInterval
				ticker.Reset(currentInterval)
			}

			for _, msg := range messages {
				onMessage(msg)
			}

		case <-stopChan:
			return
		}
	}
}

// DownloadFile downloads encrypted bytes of a file using its hash.
func (l *Listener) DownloadFile(fileHash string) ([]byte, error) {
	url := fmt.Sprintf("%s/download/%s", l.config.FileServerURL, fileHash)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create download request: %w", err)
	}

	req.Header.Set(l.config.MaskHeaderName, l.config.MaskHeaderValue)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Arch Linux; rv:130.0) Gecko/20100101 Firefox/130.0")

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download failed: %s - %s", resp.Status, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read file data: %w", err)
	}

	return data, nil
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
