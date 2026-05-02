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

const (
	MaskHeaderName  = "X-Requested-With"
	MaskHeaderValue = "XMLHttpRequest"
)

// =============================================================================
// LISTENER STRUCTURE
// =============================================================================

type Listener struct {
	client *http.Client
}

func NewListener() *Listener {
	return &Listener{
		client: &http.Client{
			Timeout: 60 * time.Second, // 60 seconds for long replies (multipart with files)
		},
	}
}

// =============================================================================
// PULL MESSAGES WITH AUTHORIZATION Ed25519
// =============================================================================

// PullMessagesWithAuth pulls messages from the server with an Ed25519 signature.
//
// Parameters:
//
//	-identityHash: SHA256(Ed25519PublicKey) in hex (aka target_hash in URL)
//	-pubKeyHex: Ed25519 public key in hex
//	-privKey: Ed25519 private key (64 bytes)
//
// The server checks:
//  1. X-Timestamp within ±60 seconds of server time
//  2. SHA256(Ed25519PublicKey) == target_hash from URL
//  3. Ed25519.Verify(publicKey, timestamp+hash, signature)
//
// Server response:
//
//	200 OK + multipart/form-data (parts: package_0, package_1, ...)
//	204 No Content -queue is empty
//	401 Unauthorized -signature error
//	429 Too Many Requests -limit exceeded
func (l *Listener) PullMessagesWithAuth(
	identityHash, pubKeyHex string,
	privKey ed25519.PrivateKey,
) ([][]byte, error) {
	// ---Forming a signature ---
	now := time.Now().Unix()
	message := fmt.Sprintf("%d%s", now, identityHash)
	signature := ed25519.Sign(privKey, []byte(message))
	sigHex := hex.EncodeToString(signature)

	// ---Let's create a request ---
	url := fmt.Sprintf("%s/pull/%s", ServerBaseURL, identityHash)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create pull request: %w", err)
	}

	// Authorization Headers (MANDATORY)
	req.Header.Set("X-Identity-Key", pubKeyHex)
	req.Header.Set("X-Timestamp", strconv.FormatInt(now, 10))
	req.Header.Set("X-Signature", sigHex)

	// Masking Headers
	req.Header.Set(MaskHeaderName, MaskHeaderValue)
	req.Header.Set("Accept", "multipart/form-data")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Arch Linux; rv:130.0) Gecko/20100101 Firefox/130.0")

	// ---Execute the request ---
	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pull request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()
	// ---Processing statuses ---

	// 204 -queue is empty (this is normal)
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	// 429 — rate limit, you need to increase the polling interval
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("rate limited (429)")
	}

	// 401 -authorization error
	if resp.StatusCode == http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unauthorized (401): %s", string(body))
	}

	// Other errors
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	// ---Parse multipart/form-data response ---
	contentType := resp.Header.Get("Content-Type")
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, fmt.Errorf("parse content-type: %w", err)
	}

	boundary, ok := params["boundary"]
	if !ok {
		// We try to read it as a regular binary response (in case of an old server)
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}
		if len(data) == 0 {
			return nil, nil
		}
		return [][]byte{data}, nil
	}

	// Reading multipart parts
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

// =============================================================================
// BACKGROUND POLLING WITH AUTO-ADAPTING INTERVAL
// =============================================================================

// StartListeningWithAuth starts a background queue poll with Ed25519 authorization.
//
// Parameters:
//
//	-identityHash, pubKeyHex, privKey -for authorization
//	-onMessage — callback for each received packet
//	-baseInterval — base polling interval (recommended 5-10 seconds)
//	-stopChan — channel to stop
//
// For error 429, the interval is automatically increased (max. 60 seconds).
// If polling is successful, it returns to the base interval.
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
				// Logging the error
				fmt.Printf("[LISTENER] Pull error: %v\n", err)

				// At 429 we increase the interval
				if err.Error() == "rate limited (429)" {
					currentInterval = minDuration(currentInterval*2, 60*time.Second)
					ticker.Reset(currentInterval)
					fmt.Printf("[LISTENER] Rate limited, increasing interval to %v\n", currentInterval)
				}
				continue
			}

			// Success -reset the interval to the base one
			if currentInterval != baseInterval {
				currentInterval = baseInterval
				ticker.Reset(currentInterval)
			}

			// We deliver messages
			for _, msg := range messages {
				onMessage(msg)
			}

		case <-stopChan:
			return
		}
	}
}

// =============================================================================
// DOWNLOADING FILES
// =============================================================================

// DownloadFile downloads encrypted bytes of a file using its hash.
//
// Uses FileServerURL (separate endpoint from messages).
// Returns the encrypted blob to be decrypted
// via crypto.DecryptFile().
func (l *Listener) DownloadFile(fileHash string) ([]byte, error) {
	url := fmt.Sprintf("%s/download/%s", FileServerURL, fileHash)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create download request: %w", err)
	}

	// Disguise
	req.Header.Set(MaskHeaderName, MaskHeaderValue)
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

	// Reading the file into memory (100MB limit on the server side)
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read file data: %w", err)
	}

	return data, nil
}

// =============================================================================
// AUXILIARY FUNCTIONS
// =============================================================================

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
