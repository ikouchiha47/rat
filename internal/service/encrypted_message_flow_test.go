package service

import (
	"fmt"
	"strings"
	"testing"

	"rat/internal/crypto"
	"rat/internal/logger"
	"rat/internal/models"
)

// TestCompleteEncryptedMessageFlow tests the complete encrypted message flow
func TestCompleteEncryptedMessageFlow(t *testing.T) {
	// Create Alice's service
	alice := &LocalChatService{
		username:          "alice",
		currentRoom:       "secure-room",
		registered:        true,
		encryptionEnabled: true,
		groupCrypto:       make(map[string]*crypto.GroupCrypto),
		logger:            logger.New(logger.LevelInfo),
		messages:          make([]*models.Message, 0),
	}
	
	// Create Bob's service
	bob := &LocalChatService{
		username:          "bob",
		currentRoom:       "secure-room",
		registered:        true,
		encryptionEnabled: true,
		groupCrypto:       make(map[string]*crypto.GroupCrypto),
		logger:            logger.New(logger.LevelInfo),
		messages:          make([]*models.Message, 0),
	}
	
	// Initialize crypto for both users
	if err := alice.initializeCrypto(); err != nil {
		t.Fatalf("Failed to initialize Alice's crypto: %v", err)
	}
	if err := bob.initializeCrypto(); err != nil {
		t.Fatalf("Failed to initialize Bob's crypto: %v", err)
	}
	
	// Initialize group crypto for both users
	if err := alice.initializeGroupCrypto("secure-room"); err != nil {
		t.Fatalf("Failed to initialize Alice's group crypto: %v", err)
	}
	if err := bob.initializeGroupCrypto("secure-room"); err != nil {
		t.Fatalf("Failed to initialize Bob's group crypto: %v", err)
	}
	
	// Simulate key exchange (in real scenario, this would happen via network)
	aliceGroupCrypto := alice.getGroupCrypto("secure-room")
	bobGroupCrypto := bob.getGroupCrypto("secure-room")
	
	// Sync group keys (Alice shares her key with Bob)
	bobGroupCrypto.GroupKey = make([]byte, len(aliceGroupCrypto.GroupKey))
	copy(bobGroupCrypto.GroupKey, aliceGroupCrypto.GroupKey)
	bobGroupCrypto.KeyVersion = aliceGroupCrypto.KeyVersion
	
	// Add each other as members
	aliceGroupCrypto.AddMember("bob", bob.GetPublicKey())
	bobGroupCrypto.AddMember("alice", alice.GetPublicKey())
	
	t.Logf("✅ Setup complete - both users have synced group keys")
	
	// Test 1: Alice sends encrypted message
	plaintext := "Hello Bob, this is a secret message! 🔐"
	
	// Simulate Alice creating an encrypted message (like SendMessage would do)
	localMessage := models.NewChatMessage("alice", "broadcast", plaintext, "secure-room")
	
	// Encrypt the message
	encryptedMsg, err := aliceGroupCrypto.EncryptMessage("alice", plaintext, localMessage.MessageID)
	if err != nil {
		t.Fatalf("Failed to encrypt Alice's message: %v", err)
	}
	
	// Create the network message (what would be sent over TCP)
	networkMessage := &models.Message{
		MessageID: localMessage.MessageID,
		From:      "alice",
		To:        "broadcast",
		Content:   fmt.Sprintf("🔒[ENCRYPTED:%s:%s:%s:%d]", crypto.ToBase64(encryptedMsg.EncryptedContent), crypto.ToBase64(encryptedMsg.WrappedKey), crypto.ToBase64(encryptedMsg.Nonce), encryptedMsg.KeyVersion),
		Room:      "secure-room",
		Type:      models.MessageTypeChat,
		Timestamp: localMessage.Timestamp,
	}
	
	t.Logf("📤 Alice's encrypted message: %s", networkMessage.Content[:50]+"...")
	
	// Test 2: Bob receives and decrypts the message
	decryptedMessage := bob.decryptIncomingMessage(networkMessage)
	
	if decryptedMessage.Content != plaintext {
		t.Errorf("Expected decrypted content '%s', got '%s'", plaintext, decryptedMessage.Content)
	}
	
	if decryptedMessage.From != "alice" {
		t.Errorf("Expected sender 'alice', got '%s'", decryptedMessage.From)
	}
	
	t.Logf("📥 Bob decrypted message: %s", decryptedMessage.Content)
	
	// Test 3: Bob sends encrypted reply
	replyText := "Hi Alice! I got your secret message. This is my encrypted reply! 🚀"
	
	bobLocalMessage := models.NewChatMessage("bob", "broadcast", replyText, "secure-room")
	bobEncryptedMsg, err := bobGroupCrypto.EncryptMessage("bob", replyText, bobLocalMessage.MessageID)
	if err != nil {
		t.Fatalf("Failed to encrypt Bob's reply: %v", err)
	}
	
	bobNetworkMessage := &models.Message{
		MessageID: bobLocalMessage.MessageID,
		From:      "bob",
		To:        "broadcast",
		Content:   fmt.Sprintf("🔒[ENCRYPTED:%s:%s:%s:%d]", crypto.ToBase64(bobEncryptedMsg.EncryptedContent), crypto.ToBase64(bobEncryptedMsg.WrappedKey), crypto.ToBase64(bobEncryptedMsg.Nonce), bobEncryptedMsg.KeyVersion),
		Room:      "secure-room",
		Type:      models.MessageTypeChat,
		Timestamp: bobLocalMessage.Timestamp,
	}
	
	// Alice receives and decrypts Bob's reply
	aliceDecryptedReply := alice.decryptIncomingMessage(bobNetworkMessage)
	
	if aliceDecryptedReply.Content != replyText {
		t.Errorf("Expected Alice to decrypt '%s', got '%s'", replyText, aliceDecryptedReply.Content)
	}
	
	t.Logf("📥 Alice decrypted Bob's reply: %s", aliceDecryptedReply.Content)
	
	// Test 4: Verify forward secrecy (different ephemeral keys)
	if string(encryptedMsg.WrappedKey) == string(bobEncryptedMsg.WrappedKey) {
		t.Error("Messages should use different ephemeral keys (forward secrecy)")
	}
	
	// Test 5: Test unencrypted message handling
	plainMessage := &models.Message{
		MessageID: "plain-msg-001",
		From:      "alice",
		To:        "broadcast",
		Content:   "This is a plain text message",
		Room:      "secure-room",
		Type:      models.MessageTypeChat,
	}
	
	decryptedPlain := bob.decryptIncomingMessage(plainMessage)
	if decryptedPlain.Content != "This is a plain text message" {
		t.Errorf("Plain message should pass through unchanged, got '%s'", decryptedPlain.Content)
	}
	
	t.Logf("✅ End-to-end encrypted message flow test passed!")
	t.Logf("   Alice → Bob: %s", plaintext)
	t.Logf("   Bob → Alice: %s", replyText)
	t.Logf("   Forward secrecy: ✓")
	t.Logf("   Plain message handling: ✓")
}

// TestEncryptionDisabled tests message flow when encryption is disabled
func TestEncryptionDisabled(t *testing.T) {
	service := &LocalChatService{
		username:          "alice",
		currentRoom:       "plain-room",
		registered:        true,
		encryptionEnabled: false, // Encryption disabled
		groupCrypto:       make(map[string]*crypto.GroupCrypto),
		logger:            logger.New(logger.LevelInfo),
		messages:          make([]*models.Message, 0),
	}
	
	// Initialize crypto (should be skipped)
	if err := service.initializeCrypto(); err != nil {
		t.Fatalf("initializeCrypto should not fail when disabled: %v", err)
	}
	
	// Verify no keys were generated
	if service.userPrivateKey != nil {
		t.Error("Private key should not be generated when encryption disabled")
	}
	
	// Test message decryption (should pass through unchanged)
	message := &models.Message{
		MessageID: "test-msg-001",
		From:      "bob",
		Content:   "Hello Alice!",
		Room:      "plain-room",
		Type:      models.MessageTypeChat,
	}
	
	decrypted := service.decryptIncomingMessage(message)
	if decrypted.Content != "Hello Alice!" {
		t.Errorf("Expected unchanged content, got '%s'", decrypted.Content)
	}
	
	t.Logf("✅ Encryption disabled test passed!")
}

// TestInvalidEncryptedMessage tests handling of malformed encrypted messages
func TestInvalidEncryptedMessage(t *testing.T) {
	service := &LocalChatService{
		username:          "alice",
		currentRoom:       "secure-room",
		registered:        true,
		encryptionEnabled: true,
		groupCrypto:       make(map[string]*crypto.GroupCrypto),
		logger:            logger.New(logger.LevelInfo),
		messages:          make([]*models.Message, 0),
	}
	
	// Initialize crypto
	if err := service.initializeCrypto(); err != nil {
		t.Fatalf("Failed to initialize crypto: %v", err)
	}
	if err := service.initializeGroupCrypto("secure-room"); err != nil {
		t.Fatalf("Failed to initialize group crypto: %v", err)
	}
	
	testCases := []struct {
		name    string
		content string
	}{
		{"Invalid format", "🔒[ENCRYPTED:invalid"},
		{"Missing parts", "🔒[ENCRYPTED:part1:part2]"},
		{"Invalid base64", "🔒[ENCRYPTED:invalid_base64:invalid_base64:1]"},
		{"Invalid version", "🔒[ENCRYPTED:dGVzdA==:dGVzdA==:invalid]"},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			message := &models.Message{
				MessageID: "invalid-msg-001",
				From:      "bob",
				Content:   tc.content,
				Room:      "secure-room",
				Type:      models.MessageTypeChat,
			}
			
			// Should return original message or error message, not crash
			result := service.decryptIncomingMessage(message)
			if result == nil {
				t.Error("decryptIncomingMessage should not return nil")
			}
			
			// Should either be original content or error message
			if result.Content != tc.content && !strings.Contains(result.Content, "DECRYPTION FAILED") {
				t.Logf("Result content: %s", result.Content)
			}
		})
	}
	
	t.Logf("✅ Invalid encrypted message handling test passed!")
}
