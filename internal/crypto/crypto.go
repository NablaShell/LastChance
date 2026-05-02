package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"

	"crypto/ecdh"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

// =============================================================================
// CONSTANTS
// =============================================================================

const (
	// Nonce size for XChaCha20-Poly1305 (standard)
	NonceSize = 24

	// Authentication tag size for Poly1305
	TagSize = 16

	// Size of frame header inside encrypted block
	// 2 bytes magic + 8 bytes sessionID + 4 bytes payloadLength
	FrameHeaderSize = 14

	// Magic bytes for frame validation after decryption
	MagicBytesValue = 0x4E53 // "NS" => Naive Secure (just a fun name, not a real standard)

	// Server-allowed packet sizes (GRID)
	// Any other size → HTTP 400 from server
	SizeTiny   = 256
	SizeSmall  = 1024
	SizeMedium = 4096
	SizeMax    = 65536
)

// AllowedTotalSizes -total packet sizes that the server accepts.
// Packet = nonce (24 bytes) + encrypted_block
var AllowedTotalSizes = []int{SizeTiny, SizeSmall, SizeMedium, SizeMax}

// =============================================================================
// AUXILIARY FUNCTIONS
// =============================================================================

// FindMinAllowedTotalSize finds the minimum size from the grid,
// which holds minDataLen bytes.
// Returns an error if the data is larger than the maximum size.
func FindMinAllowedTotalSize(minDataLen int) (int, error) {
	for _, size := range AllowedTotalSizes {
		if size >= minDataLen {
			return size, nil
		}
	}
	return 0, fmt.Errorf("data too large: %d bytes exceeds max allowed %d", minDataLen, SizeMax)
}

// =============================================================================
// ECDH -KEY EXCHANGE (X25519 + HKDF)
// =============================================================================

// GenerateSharedKey performs ECDH on Curve25519 and outputs
// shared 32-byte key via HKDF-SHA256.
// Used for both messages and files.
// crypto/crypto.go -GenerateSharedKey function
func GenerateSharedKey(myPrivateKey, theirPublicKey []byte) ([]byte, error) {
	// Validation
	if len(myPrivateKey) != 32 {
		return nil, fmt.Errorf("private key must be 32 bytes, got %d", len(myPrivateKey))
	}
	if len(theirPublicKey) != 32 {
		return nil, fmt.Errorf("public key must be 32 bytes, got %d", len(theirPublicKey))
	}

	// Create ECDH keys via crypto/ecdh (Go 1.20+)
	privKey, err := ecdh.X25519().NewPrivateKey(myPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create private key: %w", err)
	}

	pubKey, err := ecdh.X25519().NewPublicKey(theirPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create public key: %w", err)
	}

	// We perform ECDH (protected from low-order points)
	sharedSecret, err := privKey.ECDH(pubKey)
	if err != nil {
		return nil, fmt.Errorf("ECDH failed: %w", err)
	}

	// Outputting the final key via HKDF
	hash := sha256.New
	salt := make([]byte, 32)
	info := []byte("ECDH_v1")

	hkdfReader := hkdf.New(hash, sharedSecret, salt, info)
	finalKey := make([]byte, 32)
	if _, err := hkdfReader.Read(finalKey); err != nil {
		return nil, fmt.Errorf("HKDF derivation failed: %w", err)
	}

	return finalKey, nil
}

// =============================================================================
// ENCRYPTION /DECRYPTION OF MESSAGES
// =============================================================================

// EncryptMessage encrypts plaintext with a shared key and returns a packet,
// whose size is guaranteed to be included in the grid (256, 1024, 4096, 65536).
//
// Package format:
//
//	[nonce: 24 bytes] [ciphertext: variable length]
//
// Decrypted frame format (before encryption):
//
//	[magic: 2 bytes] [sessionID: 8 bytes] [payloadLength: 4 bytes] [payload: ...] [random_padding: ...]
func EncryptMessage(plaintext []byte, sharedSecret []byte, sessionID uint64) ([]byte, error) {
	// Validation
	if len(sharedSecret) != 32 {
		return nil, fmt.Errorf("shared secret must be 32 bytes, got %d", len(sharedSecret))
	}

	var encKey [32]byte
	copy(encKey[:], sharedSecret)

	// Create an AEAD cipher
	aead, err := chacha20poly1305.NewX(encKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create XChaCha20-Poly1305 cipher: %w", err)
	}

	// Forming a basic frame
	// Structure: [magic:2][sessionID:8][payloadLength:4][payload:N]
	baseFrame := make([]byte, FrameHeaderSize+len(plaintext))

	// Magic bytes for validation during decryption
	binary.LittleEndian.PutUint16(baseFrame[0:2], MagicBytesValue)

	// Session ID for protection against replay (the client manages it himself)
	binary.LittleEndian.PutUint64(baseFrame[2:10], sessionID)

	// Payload length
	// In the EncryptMessage function, before binary.LittleEndian.PutUint32:
	// After line 138 (baseFrame = make(...))
	// and BEFORE line 157 (binary.LittleEndian.PutUint32)

	// Checking for uint32 overflow
	if len(plaintext) > math.MaxUint32 {
		return nil, fmt.Errorf("plaintext too large: %d bytes exceeds max uint32", len(plaintext))
	}
	binary.LittleEndian.PutUint32(baseFrame[10:14], uint32(len(plaintext)))

	// We copy the plaintext itself
	copy(baseFrame[14:], plaintext)

	// Calculate the final packet size
	// Minimum size: nonce + frame + tag
	minTotalSize := NonceSize + len(baseFrame) + TagSize

	// Select the closest size from the grid
	totalSize, err := FindMinAllowedTotalSize(minTotalSize)
	if err != nil {
		return nil, fmt.Errorf("message too large: %w", err)
	}

	// Encrypted block size (including tag)
	encryptedBlockSize := totalSize - NonceSize

	// Frame size with padding (before encryption)
	frameWithPaddingSize := encryptedBlockSize - TagSize

	// Adding random padding
	frameWithPadding := make([]byte, frameWithPaddingSize)

	// Copy the base frame to the beginning
	copy(frameWithPadding, baseFrame)

	// Fill the remaining space with cryptographically strong random padding
	if len(baseFrame) < frameWithPaddingSize {
		paddingArea := frameWithPadding[len(baseFrame):]
		if _, err := rand.Read(paddingArea); err != nil {
			return nil, fmt.Errorf("failed to generate random padding: %w", err)
		}
	}

	// Generate a random nonce
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt
	// aead.Seal adds a 16-byte Poly1305 tag automatically
	ciphertext := aead.Seal(nil, nonce, frameWithPadding, nil)

	// Sanitary size check
	if len(ciphertext) != encryptedBlockSize {
		return nil, fmt.Errorf(
			"internal ciphertext size mismatch: got %d, expected %d",
			len(ciphertext), encryptedBlockSize,
		)
	}

	// Putting together the final package
	packet := make([]byte, totalSize)
	copy(packet[:NonceSize], nonce)
	copy(packet[NonceSize:], ciphertext)

	return packet, nil
}

// DecryptMessage decrypts the packet received from EncryptMessage.
// Accepts any size bag from the mesh.
// Returns the decrypted plaintext and sessionID.
func DecryptMessage(packet []byte, sharedSecret []byte) ([]byte, uint64, error) {
	// Checking package size
	validSize := false
	for _, allowedSize := range AllowedTotalSizes {
		if len(packet) == allowedSize {
			validSize = true
			break
		}
	}
	if !validSize {
		return nil, 0, fmt.Errorf(
			"invalid packet size %d: must be one of %v",
			len(packet), AllowedTotalSizes,
		)
	}

	// Validation key
	if len(sharedSecret) != 32 {
		return nil, 0, fmt.Errorf("shared secret must be 32 bytes, got %d", len(sharedSecret))
	}

	var encKey [32]byte
	copy(encKey[:], sharedSecret)

	// Create an AEAD cipher
	aead, err := chacha20poly1305.NewX(encKey[:])
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Minimum length check
	if len(packet) < NonceSize+TagSize {
		return nil, 0, fmt.Errorf("packet too short: %d bytes, need at least %d", len(packet), NonceSize+TagSize)
	}

	// Extract the nonce and ciphertext
	nonce := packet[:NonceSize]
	encryptedBlock := packet[NonceSize:]

	// Deciphering
	frameWithPadding, err := aead.Open(nil, nonce, encryptedBlock, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("decryption failed (wrong key or corrupted data): %w", err)
	}

	// Checking the frame title
	if len(frameWithPadding) < FrameHeaderSize {
		return nil, 0, fmt.Errorf("decrypted frame too short: %d bytes", len(frameWithPadding))
	}

	// Checking magic bytes
	magic := binary.LittleEndian.Uint16(frameWithPadding[0:2])
	if magic != MagicBytesValue {
		return nil, 0, fmt.Errorf(
			"invalid magic bytes: expected 0x%04X, got 0x%04X",
			MagicBytesValue, magic,
		)
	}

	// I extract the sessionID
	sessionID := binary.LittleEndian.Uint64(frameWithPadding[2:10])

	// Retrieving the declared data length
	dataLen := binary.LittleEndian.Uint32(frameWithPadding[10:14])

	// We check that the declared length does not exceed the boundaries of the frame
	if int(dataLen) > len(frameWithPadding)-FrameHeaderSize {
		return nil, 0, fmt.Errorf(
			"declared data length %d exceeds frame bounds %d",
			dataLen, len(frameWithPadding)-FrameHeaderSize,
		)
	}

	// Extracting the original message (ignoring padding)
	message := make([]byte, dataLen)
	copy(message, frameWithPadding[FrameHeaderSize:FrameHeaderSize+dataLen])

	return message, sessionID, nil
}

// =============================================================================
// ENCRYPTION /DECRYPTION OF FILES
// =============================================================================

// EncryptFile encrypts arbitrary data (file) with a shared key.
// Returns the ciphertext in the format: nonce + encrypted_data.
// DOES NOT add padding -used as is.
func EncryptFile(plaintext, sharedSecret []byte) ([]byte, error) {
	if len(sharedSecret) != 32 {
		return nil, fmt.Errorf("shared secret must be 32 bytes, got %d", len(sharedSecret))
	}

	var encKey [32]byte
	copy(encKey[:], sharedSecret)

	aead, err := chacha20poly1305.NewX(encKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Generate a unique nonce for this file
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// We encrypt: the nonce is added to the beginning automatically
	ciphertext := aead.Seal(nonce, nonce, plaintext, nil)

	return ciphertext, nil
}

// DecryptFile decrypts data encrypted by EncryptFile.
// Expects format: nonce + encrypted_data.
func DecryptFile(ciphertext, sharedSecret []byte) ([]byte, error) {
	if len(sharedSecret) != 32 {
		return nil, fmt.Errorf("shared secret must be 32 bytes, got %d", len(sharedSecret))
	}

	var encKey [32]byte
	copy(encKey[:], sharedSecret)

	aead, err := chacha20poly1305.NewX(encKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Check the minimum length (nonce + at least 1 byte + tag)
	if len(ciphertext) < aead.NonceSize()+TagSize {
		return nil, fmt.Errorf("ciphertext too short: %d bytes", len(ciphertext))
	}

	// Extracting the nonce from the beginning
	nonce := ciphertext[:aead.NonceSize()]
	encryptedData := ciphertext[aead.NonceSize():]

	// Deciphering
	plaintext, err := aead.Open(nil, nonce, encryptedData, nil)
	if err != nil {
		return nil, fmt.Errorf("file decryption failed: %w", err)
	}

	return plaintext, nil
}

// =============================================================================
// Ed25519 - AUTHENTICATION (SIGNATURES)
// =============================================================================

// GenerateEd25519KeyFromSeed creates the private key Ed25519 from a 32-byte seed.
// The Ed25519 private key is 64 bytes in size (the first 32 are the seed, the second 32 are the public key).
func GenerateEd25519KeyFromSeed(seed []byte) (ed25519.PrivateKey, error) {
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("seed must be exactly %d bytes, got %d", ed25519.SeedSize, len(seed))
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// SignEd25519 creates an Ed25519 message signature and returns it in hex.
func SignEd25519(priv ed25519.PrivateKey, message []byte) (string, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("private key must be %d bytes, got %d", ed25519.PrivateKeySize, len(priv))
	}
	signature := ed25519.Sign(priv, message)
	return hex.EncodeToString(signature), nil
}

// Verify Ed25519 verifies the Ed25519 signature (for reference, it may not be used on the client).
func VerifyEd25519(pub ed25519.PublicKey, message []byte, signatureHex string) (bool, error) {
	if len(pub) != ed25519.PublicKeySize {
		return false, fmt.Errorf("public key must be %d bytes", ed25519.PublicKeySize)
	}
	signature, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false, fmt.Errorf("invalid signature hex: %w", err)
	}
	return ed25519.Verify(pub, message, signature), nil
}

// ComputeIdentityHash calculates the identity hash according to the server standard:
// SHA256(Ed25519_PublicKey) → hex
// The server checks that the hash in the /pull/{hash} URL matches SHA256(Ed25519_PublicKey).
func ComputeIdentityHash(pubKey ed25519.PublicKey) string {
	hash := sha256.Sum256(pubKey)
	return hex.EncodeToString(hash[:])
}
