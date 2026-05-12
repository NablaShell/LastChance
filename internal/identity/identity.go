package identity

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
	"github.com/NablaShell/LastChance/internal/storage"
	"golang.org/x/crypto/hkdf"

	"github.com/tyler-smith/go-bip39"
)

const IdentityFileName = "identity.json"

// Identity stores ALL user keys.
type Identity struct {
	Nickname   string `json:"nickname"`
	SeedPhrase string `json:"seed_phrase"`

	// X25519 keys for ECDH (message and file encryption)
	PublicKey  []byte `json:"public_key"`
	PrivateKey []byte `json:"private_key"` // #nosec G101 is an encryption key, stored in a protected file

	// Ed25519 keys for server authentication
	Ed25519PublicKey  []byte `json:"ed25519_public_key"`
	Ed25519PrivateKey []byte `json:"ed25519_private_key"` // #nosec G101 is a signing key, stored in a protected file

	Hash      string `json:"hash"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// The IdentityManager manages the loading, saving, and updating of identities.
type IdentityManager struct {
	storagePath string
	safeFS      *storage.SafeFSOps
	cachedID    *Identity // loaded identity cache
}

// NewIdentityManager creates a manager without immediately initializing safeFS.
func NewIdentityManager(storagePath string) *IdentityManager {
	return &IdentityManager{
		storagePath: storagePath,
	}
}

// initSafeFS initializes safeFS on first access.
func (im *IdentityManager) initSafeFS() error {
	if im.safeFS != nil {
		return nil
	}
	var err error
	im.safeFS, err = storage.NewSafeFSOps(im.storagePath)
	return err
}

// deriveKeysFromSeed generates X25519 and Ed25519 keys from the master seed.
func deriveKeysFromSeed(seed []byte) (
	x25519Pub, x25519Priv []byte,
	ed25519Pub, ed25519Priv []byte,
	hash string,
	err error,
) {
	// X25519 via crypto/ecdh
	infoX := []byte("X25519_E2EE_Key_v1")
	xPrivRaw := make([]byte, 32)
	hkdfX := hkdf.New(sha256.New, seed, nil, infoX)
	if _, err := hkdfX.Read(xPrivRaw); err != nil {
		return nil, nil, nil, nil, "", fmt.Errorf("hkdf x25519: %w", err)
	}

	// Using modern crypto/ecdh instead of deprecated curve25519
	privKey, err := ecdh.X25519().NewPrivateKey(xPrivRaw)
	if err != nil {
		return nil, nil, nil, nil, "", fmt.Errorf("x25519 private key: %w", err)
	}

	x25519Priv = privKey.Bytes()
	x25519Pub = privKey.PublicKey().Bytes()

	// Ed25519
	infoE := []byte("Ed25519_Auth_Key_v1")
	edSeed := make([]byte, ed25519.SeedSize)
	hkdfE := hkdf.New(sha256.New, seed, nil, infoE)
	if _, err := hkdfE.Read(edSeed); err != nil {
		return nil, nil, nil, nil, "", fmt.Errorf("hkdf ed25519: %w", err)
	}

	edPriv := ed25519.NewKeyFromSeed(edSeed)
	edPub := edPriv.Public().(ed25519.PublicKey)

	// Hash = SHA256(Ed25519PublicKey)
	hashSum := sha256.Sum256(edPub)
	hash = hex.EncodeToString(hashSum[:])

	return x25519Pub, x25519Priv, edPub, edPriv, hash, nil
}

// GenerateNewIdentity creates a new identity with a mnemonic phrase (24 words).
func (im *IdentityManager) GenerateNewIdentity(nickname string) (*Identity, error) {
	entropy := make([]byte, 32)
	if _, err := rand.Read(entropy); err != nil {
		return nil, fmt.Errorf("failed to generate entropy: %w", err)
	}

	seedPhrase, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return nil, fmt.Errorf("failed to generate mnemonic: %w", err)
	}

	seed := bip39.NewSeed(seedPhrase, "")

	xPub, xPriv, edPub, edPriv, hash, err := deriveKeysFromSeed(seed)
	if err != nil {
		return nil, fmt.Errorf("failed to derive keys: %w", err)
	}

	if nickname == "" {
		nickname = "user_" + hash[:8]
	}

	now := time.Now().Unix()

	identity := &Identity{
		Nickname:          nickname,
		SeedPhrase:        seedPhrase,
		PublicKey:         xPub,
		PrivateKey:        xPriv,
		Ed25519PublicKey:  edPub,
		Ed25519PrivateKey: edPriv,
		Hash:              hash,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	im.cachedID = identity
	return identity, nil
}

// LoadOrCreate loads an existing identity or creates a new one.
func (im *IdentityManager) LoadOrCreate(nickname string) (*Identity, error) {
	if err := im.initSafeFS(); err != nil {
		return nil, fmt.Errorf("failed to init safe FS: %w", err)
	}

	// Checking existence via safeFS
	_, err := im.safeFS.SafeStat(IdentityFileName)
	if err == nil {
		identity, err := im.Load()
		if err != nil {
			return nil, fmt.Errorf("failed to load identity: %w", err)
		}
		return identity, nil
	}

	// There is no file -create a new one
	identity, err := im.GenerateNewIdentity(nickname)
	if err != nil {
		return nil, err
	}

	if err := im.Save(identity); err != nil {
		return nil, fmt.Errorf("failed to save new identity: %w", err)
	}

	return identity, nil
}

// Save retains the identity with rights 0600.
func (im *IdentityManager) Save(identity *Identity) error {
	if err := im.initSafeFS(); err != nil {
		return err
	}

	// #nosec G117 -the private key must be in JSON for recovery, the file is protected 0600
	data, err := json.MarshalIndent(identity, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal identity: %w", err)
	}

	return im.safeFS.SafeWriteFile(IdentityFileName, data, 0600)
}

// Load loads an identity from a file.
func (im *IdentityManager) Load() (*Identity, error) {
	if err := im.initSafeFS(); err != nil {
		return nil, err
	}

	data, err := im.safeFS.SafeReadFile(IdentityFileName)
	if err != nil {
		return nil, fmt.Errorf("failed to read identity: %w", err)
	}

	var identity Identity
	if err := json.Unmarshal(data, &identity); err != nil {
		return nil, fmt.Errorf("failed to unmarshal identity: %w", err)
	}

	im.cachedID = &identity
	return &identity, nil
}

// UpdateNickname updates the nickname and resaves the file.
func (im *IdentityManager) UpdateNickname(nickname string) error {
	if im.cachedID == nil {
		// We try to download if it is not in the cache
		if _, err := im.LoadOrCreate(nickname); err != nil {
			return fmt.Errorf("no identity loaded and cannot load: %w", err)
		}
	}
	im.cachedID.Nickname = nickname
	im.cachedID.UpdatedAt = time.Now().Unix()
	return im.Save(im.cachedID)
}

// GetEncryptionKey returns the 32-byte encryption key.
func (i *Identity) GetEncryptionKey() [32]byte {
	var key [32]byte
	hash := sha256.Sum256(i.PrivateKey)
	copy(key[:], hash[:])
	return key
}

// GetPublicKeyHex returns X25519 public key in hex.
func (i *Identity) GetPublicKeyHex() string {
	return hex.EncodeToString(i.PublicKey)
}

// GetEd25519PublicKeyHex returns the Ed25519 public key in hex.
func (i *Identity) GetEd25519PublicKeyHex() string {
	return hex.EncodeToString(i.Ed25519PublicKey)
}

// SignAuth creates an Ed25519 signature to authorize the Pull request.
func (i *Identity) SignAuth(timestamp int64) (string, error) {
	if len(i.Ed25519PrivateKey) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("ed25519 private key invalid size: %d", len(i.Ed25519PrivateKey))
	}

	message := fmt.Sprintf("%d%s", timestamp, i.Hash)
	priv := ed25519.PrivateKey(i.Ed25519PrivateKey)
	signature := ed25519.Sign(priv, []byte(message))

	return hex.EncodeToString(signature), nil
}
