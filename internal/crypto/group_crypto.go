package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

// GroupCrypto manages encryption for a chat room
type GroupCrypto struct {
	RoomID      string
	GroupKey    []byte
	Members     map[string]*Member
	KeyVersion  int
	CreatedAt   time.Time
	LastRotation time.Time
}

// Member represents a group member with their cryptographic identity
type Member struct {
	Username    string
	PublicKey   []byte    // X25519 public key
	JoinedAt    time.Time
}

// EncryptedMessage represents an encrypted group message
type EncryptedMessage struct {
	Sender         string    `json:"sender"`
	Room           string    `json:"room"`
	EncryptedContent []byte  `json:"encrypted_content"`
	WrappedKey     []byte    `json:"wrapped_key"`
	KeyVersion     int       `json:"key_version"`
	MessageID      string    `json:"message_id"`
	Timestamp      time.Time `json:"timestamp"`
	Nonce          []byte    `json:"nonce"`
}

// KeyExchangeMessage for distributing group keys
type KeyExchangeMessage struct {
	Type           string `json:"type"` // "group_key_distribution"
	RoomID         string `json:"room_id"`
	EncryptedGroupKey []byte `json:"encrypted_group_key"`
	KeyVersion     int    `json:"key_version"`
	Sender         string `json:"sender"`
	Recipient      string `json:"recipient"`
}

// NewGroupCrypto creates a new group crypto manager
func NewGroupCrypto(roomID string) *GroupCrypto {
	return &GroupCrypto{
		RoomID:       roomID,
		Members:      make(map[string]*Member),
		KeyVersion:   1,
		CreatedAt:    time.Now(),
		LastRotation: time.Now(),
	}
}

// GenerateKeyPair generates an X25519 key pair for a user
func GenerateKeyPair() (privateKey, publicKey []byte, err error) {
	privateKey = make([]byte, 32)
	if _, err := rand.Read(privateKey); err != nil {
		return nil, nil, fmt.Errorf("failed to generate private key: %w", err)
	}
	
	publicKey, err = curve25519.X25519(privateKey, curve25519.Basepoint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate public key: %w", err)
	}
	
	return privateKey, publicKey, nil
}

// GenerateGroupKey creates a new random group key
func (g *GroupCrypto) GenerateGroupKey() error {
	groupKey := make([]byte, 32)
	if _, err := rand.Read(groupKey); err != nil {
		return fmt.Errorf("failed to generate group key: %w", err)
	}
	
	g.GroupKey = groupKey
	g.KeyVersion++
	g.LastRotation = time.Now()
	
	return nil
}

// AddMember adds a new member to the group
func (g *GroupCrypto) AddMember(username string, publicKey []byte) {
	g.Members[username] = &Member{
		Username:  username,
		PublicKey: publicKey,
		JoinedAt:  time.Now(),
	}
}

// EncryptMessage encrypts a message for the group using ephemeral key
func (g *GroupCrypto) EncryptMessage(sender, plaintext, messageID string) (*EncryptedMessage, error) {
	if g.GroupKey == nil {
		return nil, fmt.Errorf("no group key available")
	}
	
	// Generate ephemeral key for this message
	ephemeralKey := make([]byte, 32)
	if _, err := rand.Read(ephemeralKey); err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}
	
	// Create ChaCha20-Poly1305 cipher with ephemeral key
	aead, err := chacha20poly1305.New(ephemeralKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	
	// Generate nonce
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	
	// Encrypt the message content
	encryptedContent := aead.Seal(nil, nonce, []byte(plaintext), nil)
	
	// Encrypt ephemeral key with group key
	groupAead, err := chacha20poly1305.New(g.GroupKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create group cipher: %w", err)
	}
	
	keyNonce := make([]byte, groupAead.NonceSize())
	if _, err := rand.Read(keyNonce); err != nil {
		return nil, fmt.Errorf("failed to generate key nonce: %w", err)
	}
	
	wrappedKey := groupAead.Seal(keyNonce, keyNonce, ephemeralKey, nil)
	
	// Securely delete ephemeral key from memory
	for i := range ephemeralKey {
		ephemeralKey[i] = 0
	}
	
	return &EncryptedMessage{
		Sender:           sender,
		Room:             g.RoomID,
		EncryptedContent: encryptedContent,
		WrappedKey:       wrappedKey,
		KeyVersion:       g.KeyVersion,
		MessageID:        messageID,
		Timestamp:        time.Now(),
		Nonce:            nonce,
	}, nil
}

// DecryptMessage decrypts a group message
func (g *GroupCrypto) DecryptMessage(msg *EncryptedMessage) (string, error) {
	if g.GroupKey == nil {
		return "", fmt.Errorf("no group key available")
	}
	
	if msg.KeyVersion != g.KeyVersion {
		return "", fmt.Errorf("key version mismatch: have %d, message has %d", g.KeyVersion, msg.KeyVersion)
	}
	
	// Decrypt ephemeral key using group key
	groupAead, err := chacha20poly1305.New(g.GroupKey)
	if err != nil {
		return "", fmt.Errorf("failed to create group cipher: %w", err)
	}
	
	// Extract nonce from wrapped key (first 12 bytes)
	if len(msg.WrappedKey) < groupAead.NonceSize() {
		return "", fmt.Errorf("wrapped key too short")
	}
	
	keyNonce := msg.WrappedKey[:groupAead.NonceSize()]
	encryptedEphemeralKey := msg.WrappedKey[groupAead.NonceSize():]
	
	ephemeralKey, err := groupAead.Open(nil, keyNonce, encryptedEphemeralKey, nil)
	if err != nil {
		return "", fmt.Errorf("failed to unwrap ephemeral key: %w", err)
	}
	
	// Decrypt message content using ephemeral key
	aead, err := chacha20poly1305.New(ephemeralKey)
	if err != nil {
		// Securely delete ephemeral key
		for i := range ephemeralKey {
			ephemeralKey[i] = 0
		}
		return "", fmt.Errorf("failed to create message cipher: %w", err)
	}
	
	plaintext, err := aead.Open(nil, msg.Nonce, msg.EncryptedContent, nil)
	
	// Securely delete ephemeral key from memory
	for i := range ephemeralKey {
		ephemeralKey[i] = 0
	}
	
	if err != nil {
		return "", fmt.Errorf("failed to decrypt message: %w", err)
	}
	
	return string(plaintext), nil
}

// CreateKeyExchangeMessage creates a message to distribute group key to a member
func (g *GroupCrypto) CreateKeyExchangeMessage(recipient string, recipientPublicKey, senderPrivateKey []byte) (*KeyExchangeMessage, error) {
	if g.GroupKey == nil {
		return nil, fmt.Errorf("no group key to distribute")
	}
	
	// Perform ECDH to get shared secret
	sharedSecret, err := curve25519.X25519(senderPrivateKey, recipientPublicKey)
	if err != nil {
		return nil, fmt.Errorf("ECDH failed: %w", err)
	}
	
	// Derive encryption key from shared secret using HKDF
	hkdf := hkdf.New(sha256.New, sharedSecret, nil, []byte("group-key-distribution"))
	derivedKey := make([]byte, 32)
	if _, err := hkdf.Read(derivedKey); err != nil {
		return nil, fmt.Errorf("key derivation failed: %w", err)
	}
	
	// Encrypt group key with derived key
	aead, err := chacha20poly1305.New(derivedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	
	encryptedGroupKey := aead.Seal(nonce, nonce, g.GroupKey, nil)
	
	// Securely delete derived key
	for i := range derivedKey {
		derivedKey[i] = 0
	}
	
	return &KeyExchangeMessage{
		Type:              "group_key_distribution",
		RoomID:            g.RoomID,
		EncryptedGroupKey: encryptedGroupKey,
		KeyVersion:        g.KeyVersion,
		Recipient:         recipient,
	}, nil
}

// ProcessKeyExchangeMessage processes a received group key distribution message
func (g *GroupCrypto) ProcessKeyExchangeMessage(msg *KeyExchangeMessage, recipientPrivateKey, senderPublicKey []byte) error {
	// Perform ECDH to get shared secret
	sharedSecret, err := curve25519.X25519(recipientPrivateKey, senderPublicKey)
	if err != nil {
		return fmt.Errorf("ECDH failed: %w", err)
	}
	
	// Derive decryption key from shared secret
	hkdf := hkdf.New(sha256.New, sharedSecret, nil, []byte("group-key-distribution"))
	derivedKey := make([]byte, 32)
	if _, err := hkdf.Read(derivedKey); err != nil {
		return fmt.Errorf("key derivation failed: %w", err)
	}
	
	// Decrypt group key
	aead, err := chacha20poly1305.New(derivedKey)
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}
	
	// Extract nonce (first 12 bytes)
	if len(msg.EncryptedGroupKey) < aead.NonceSize() {
		return fmt.Errorf("encrypted group key too short")
	}
	
	nonce := msg.EncryptedGroupKey[:aead.NonceSize()]
	encryptedKey := msg.EncryptedGroupKey[aead.NonceSize():]
	
	groupKey, err := aead.Open(nil, nonce, encryptedKey, nil)
	if err != nil {
		return fmt.Errorf("failed to decrypt group key: %w", err)
	}
	
	// Update group key
	g.GroupKey = groupKey
	g.KeyVersion = msg.KeyVersion
	g.LastRotation = time.Now()
	
	// Securely delete derived key
	for i := range derivedKey {
		derivedKey[i] = 0
	}
	
	return nil
}

// ShouldRotateKey determines if the group key should be rotated
func (g *GroupCrypto) ShouldRotateKey(messageCount int, timeSinceLastRotation time.Duration) bool {
	// Rotate every 100 messages or every 24 hours, whichever comes first
	return messageCount >= 100 || timeSinceLastRotation >= 24*time.Hour
}

// String returns a string representation for debugging (without sensitive data)
func (g *GroupCrypto) String() string {
	return fmt.Sprintf("GroupCrypto{RoomID: %s, KeyVersion: %d, Members: %d, LastRotation: %s}",
		g.RoomID, g.KeyVersion, len(g.Members), g.LastRotation.Format(time.RFC3339))
}

// ToBase64 converts bytes to base64 for JSON serialization
func ToBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// FromBase64 converts base64 string back to bytes
func FromBase64(data string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(data)
}
