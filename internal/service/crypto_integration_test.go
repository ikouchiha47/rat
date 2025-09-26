package service

import (
	"fmt"
	"testing"

	"rat/internal/crypto"
	"rat/internal/logger"
)

// testLogger is a simple logger for unit tests
type testLogger struct {
	*logger.Logger
}

func newTestLogger() *testLogger {
	return &testLogger{
		Logger: logger.New(logger.LevelInfo),
	}
}

// TestCryptoIntegrationUnit tests crypto functionality in isolation
func TestCryptoIntegrationUnit(t *testing.T) {
	// Test basic crypto initialization
	service := &LocalChatService{
		encryptionEnabled: true,
		groupCrypto:       make(map[string]*crypto.GroupCrypto),
		logger:            newTestLogger().Logger,
	}
	
	// Test user key generation
	if err := service.initializeCrypto(); err != nil {
		t.Fatalf("Failed to initialize crypto: %v", err)
	}
	
	// Verify keys were generated
	if service.userPrivateKey == nil {
		t.Error("User private key not generated")
	}
	if service.userPublicKey == nil {
		t.Error("User public key not generated")
	}
	if len(service.userPrivateKey) != 32 {
		t.Errorf("Expected 32-byte private key, got %d", len(service.userPrivateKey))
	}
	if len(service.userPublicKey) != 32 {
		t.Errorf("Expected 32-byte public key, got %d", len(service.userPublicKey))
	}
	
	// Test public key retrieval
	publicKey := service.GetPublicKey()
	if publicKey == nil {
		t.Error("GetPublicKey returned nil")
	}
	if string(publicKey) != string(service.userPublicKey) {
		t.Error("GetPublicKey returned different key")
	}
	
	t.Logf("✅ Crypto integration unit test passed!")
	t.Logf("   Private key length: %d bytes", len(service.userPrivateKey))
	t.Logf("   Public key length: %d bytes", len(service.userPublicKey))
}

// TestGroupCryptoUnit tests group crypto functionality in isolation
func TestGroupCryptoUnit(t *testing.T) {
	service := &LocalChatService{
		username:          "alice",
		encryptionEnabled: true,
		groupCrypto:       make(map[string]*crypto.GroupCrypto),
		logger:            newTestLogger().Logger,
	}
	
	// Initialize user crypto
	if err := service.initializeCrypto(); err != nil {
		t.Fatalf("Failed to initialize crypto: %v", err)
	}
	
	// Test group crypto initialization
	roomID := "test-room"
	if err := service.initializeGroupCrypto(roomID); err != nil {
		t.Fatalf("Failed to initialize group crypto: %v", err)
	}
	
	// Verify group crypto was created
	groupCrypto := service.getGroupCrypto(roomID)
	if groupCrypto == nil {
		t.Fatal("Group crypto not created")
	}
	
	if groupCrypto.RoomID != roomID {
		t.Errorf("Expected room ID %s, got %s", roomID, groupCrypto.RoomID)
	}
	
	if len(groupCrypto.Members) != 1 {
		t.Errorf("Expected 1 member, got %d", len(groupCrypto.Members))
	}
	
	if _, exists := groupCrypto.Members["alice"]; !exists {
		t.Error("Alice not found in group members")
	}
	
	// Test encryption/decryption
	plaintext := "Hello, encrypted world!"
	messageID := "test-msg-001"
	
	encrypted, err := groupCrypto.EncryptMessage("alice", plaintext, messageID)
	if err != nil {
		t.Fatalf("Failed to encrypt message: %v", err)
	}
	
	decrypted, err := groupCrypto.DecryptMessage(encrypted)
	if err != nil {
		t.Fatalf("Failed to decrypt message: %v", err)
	}
	
	if decrypted != plaintext {
		t.Errorf("Expected %s, got %s", plaintext, decrypted)
	}
	
	t.Logf("✅ Group crypto unit test passed!")
	t.Logf("   Room: %s", roomID)
	t.Logf("   Members: %d", len(groupCrypto.Members))
	t.Logf("   Message encrypted/decrypted successfully")
}

// TestEncryptionToggle tests enabling/disabling encryption
func TestEncryptionToggleUnit(t *testing.T) {
	service := &LocalChatService{
		encryptionEnabled: true,
		groupCrypto:       make(map[string]*crypto.GroupCrypto),
		logger:            newTestLogger().Logger,
	}
	
	// Test initial state
	if !service.encryptionEnabled {
		t.Error("Encryption should be enabled initially")
	}
	
	// Test disabling
	service.EnableEncryption(false)
	if service.encryptionEnabled {
		t.Error("Encryption should be disabled")
	}
	
	// Test that crypto initialization is skipped when disabled
	if err := service.initializeCrypto(); err != nil {
		t.Fatalf("initializeCrypto should not fail when disabled: %v", err)
	}
	
	if service.userPrivateKey != nil {
		t.Error("Private key should not be generated when encryption disabled")
	}
	
	// Test re-enabling
	service.EnableEncryption(true)
	if !service.encryptionEnabled {
		t.Error("Encryption should be enabled")
	}
	
	// Test that crypto works after re-enabling
	if err := service.initializeCrypto(); err != nil {
		t.Fatalf("Failed to initialize crypto after re-enabling: %v", err)
	}
	
	if service.userPrivateKey == nil {
		t.Error("Private key should be generated after re-enabling")
	}
	
	t.Logf("✅ Encryption toggle unit test passed!")
}

// TestMultipleRoomsUnit tests crypto for multiple rooms
func TestMultipleRoomsUnit(t *testing.T) {
	service := &LocalChatService{
		username:          "alice",
		encryptionEnabled: true,
		groupCrypto:       make(map[string]*crypto.GroupCrypto),
		logger:            newTestLogger().Logger,
	}
	
	// Initialize user crypto
	if err := service.initializeCrypto(); err != nil {
		t.Fatalf("Failed to initialize crypto: %v", err)
	}
	
	// Create multiple rooms
	rooms := []string{"room1", "room2", "room3"}
	
	for _, room := range rooms {
		if err := service.initializeGroupCrypto(room); err != nil {
			t.Fatalf("Failed to initialize crypto for room %s: %v", room, err)
		}
		
		groupCrypto := service.getGroupCrypto(room)
		if groupCrypto == nil {
			t.Errorf("Group crypto not found for room %s", room)
			continue
		}
		
		if groupCrypto.RoomID != room {
			t.Errorf("Expected room ID %s, got %s", room, groupCrypto.RoomID)
		}
		
		// Test that each room has different keys
		if len(service.groupCrypto) != len(rooms) {
			// We'll check this after all rooms are created
		}
	}
	
	// Verify all rooms have separate crypto instances
	if len(service.groupCrypto) != len(rooms) {
		t.Errorf("Expected %d group crypto instances, got %d", len(rooms), len(service.groupCrypto))
	}
	
	// Verify each room has different group keys
	keys := make(map[string][]byte)
	for roomID, groupCrypto := range service.groupCrypto {
		keys[roomID] = groupCrypto.GroupKey
	}
	
	// Check that all keys are different
	for room1, key1 := range keys {
		for room2, key2 := range keys {
			if room1 != room2 && string(key1) == string(key2) {
				t.Errorf("Rooms %s and %s have identical group keys", room1, room2)
			}
		}
	}
	
	t.Logf("✅ Multiple rooms unit test passed!")
	t.Logf("   Rooms created: %v", rooms)
	t.Logf("   Each room has unique group key")
}

// TestForwardSecrecyUnit tests forward secrecy in isolation
func TestForwardSecrecyUnit(t *testing.T) {
	service := &LocalChatService{
		username:          "alice",
		encryptionEnabled: true,
		groupCrypto:       make(map[string]*crypto.GroupCrypto),
		logger:            newTestLogger().Logger,
	}
	
	// Initialize crypto
	if err := service.initializeCrypto(); err != nil {
		t.Fatalf("Failed to initialize crypto: %v", err)
	}
	
	if err := service.initializeGroupCrypto("secure-room"); err != nil {
		t.Fatalf("Failed to initialize group crypto: %v", err)
	}
	
	groupCrypto := service.getGroupCrypto("secure-room")
	if groupCrypto == nil {
		t.Fatal("Group crypto not found")
	}
	
	// Encrypt multiple messages
	messages := []string{"Message 1", "Message 2", "Message 3", "Message 4", "Message 5"}
	encrypted := make([]*crypto.EncryptedMessage, len(messages))
	
	for i, msg := range messages {
		enc, err := groupCrypto.EncryptMessage("alice", msg, fmt.Sprintf("msg-%d", i))
		if err != nil {
			t.Fatalf("Failed to encrypt message %d: %v", i, err)
		}
		encrypted[i] = enc
	}
	
	// Verify each message uses different ephemeral key (wrapped keys should be different)
	for i := 0; i < len(encrypted)-1; i++ {
		for j := i + 1; j < len(encrypted); j++ {
			if string(encrypted[i].WrappedKey) == string(encrypted[j].WrappedKey) {
				t.Errorf("Messages %d and %d have same wrapped key (no forward secrecy)", i, j)
			}
			if string(encrypted[i].EncryptedContent) == string(encrypted[j].EncryptedContent) {
				t.Errorf("Messages %d and %d have same encrypted content", i, j)
			}
		}
	}
	
	// Verify all messages can still be decrypted
	for i, enc := range encrypted {
		decrypted, err := groupCrypto.DecryptMessage(enc)
		if err != nil {
			t.Fatalf("Failed to decrypt message %d: %v", i, err)
		}
		if decrypted != messages[i] {
			t.Errorf("Message %d: expected %s, got %s", i, messages[i], decrypted)
		}
	}
	
	t.Logf("✅ Forward secrecy unit test passed!")
	t.Logf("   Messages encrypted: %d", len(messages))
	t.Logf("   Each message uses unique ephemeral key")
	t.Logf("   All messages successfully decrypted")
}
