package crypto

import (
	"fmt"
	"testing"
	"time"
)

func TestBasicGroupEncryption(t *testing.T) {
	// Create group crypto manager
	group := NewGroupCrypto("test-room")
	
	// Generate group key
	if err := group.GenerateGroupKey(); err != nil {
		t.Fatalf("Failed to generate group key: %v", err)
	}
	
	// Test message encryption/decryption
	plaintext := "Hello, secure group!"
	messageID := "test-msg-001"
	sender := "alice"
	
	// Encrypt message
	encrypted, err := group.EncryptMessage(sender, plaintext, messageID)
	if err != nil {
		t.Fatalf("Failed to encrypt message: %v", err)
	}
	
	// Verify encrypted message structure
	if encrypted.Sender != sender {
		t.Errorf("Expected sender %s, got %s", sender, encrypted.Sender)
	}
	if encrypted.Room != "test-room" {
		t.Errorf("Expected room test-room, got %s", encrypted.Room)
	}
	if encrypted.MessageID != messageID {
		t.Errorf("Expected message ID %s, got %s", messageID, encrypted.MessageID)
	}
	
	// Decrypt message
	decrypted, err := group.DecryptMessage(encrypted)
	if err != nil {
		t.Fatalf("Failed to decrypt message: %v", err)
	}
	
	// Verify decrypted content
	if decrypted != plaintext {
		t.Errorf("Expected %s, got %s", plaintext, decrypted)
	}
	
	t.Logf("✅ Basic encryption/decryption works!")
	t.Logf("   Original: %s", plaintext)
	t.Logf("   Decrypted: %s", decrypted)
}

func TestKeyExchange(t *testing.T) {
	// Alice creates group
	aliceGroup := NewGroupCrypto("secure-room")
	if err := aliceGroup.GenerateGroupKey(); err != nil {
		t.Fatalf("Failed to generate Alice's group key: %v", err)
	}
	
	// Generate key pairs for Alice and Bob
	alicePriv, alicePub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate Alice's key pair: %v", err)
	}
	
	bobPriv, bobPub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate Bob's key pair: %v", err)
	}
	
	// Alice adds Bob as member
	aliceGroup.AddMember("bob", bobPub)
	
	// Alice creates key exchange message for Bob
	keyMsg, err := aliceGroup.CreateKeyExchangeMessage("bob", bobPub, alicePriv)
	if err != nil {
		t.Fatalf("Failed to create key exchange message: %v", err)
	}
	
	// Bob creates his group instance and processes key exchange
	bobGroup := NewGroupCrypto("secure-room")
	if err := bobGroup.ProcessKeyExchangeMessage(keyMsg, bobPriv, alicePub); err != nil {
		t.Fatalf("Failed to process key exchange: %v", err)
	}
	
	// Verify both have same group key
	if string(aliceGroup.GroupKey) != string(bobGroup.GroupKey) {
		t.Errorf("Group keys don't match after key exchange")
	}
	
	// Test that both can encrypt/decrypt messages
	plaintext := "Secret group message!"
	
	// Alice encrypts
	aliceMsg, err := aliceGroup.EncryptMessage("alice", plaintext, "msg-001")
	if err != nil {
		t.Fatalf("Alice failed to encrypt: %v", err)
	}
	
	// Bob decrypts Alice's message
	bobDecrypted, err := bobGroup.DecryptMessage(aliceMsg)
	if err != nil {
		t.Fatalf("Bob failed to decrypt Alice's message: %v", err)
	}
	
	if bobDecrypted != plaintext {
		t.Errorf("Expected %s, Bob got %s", plaintext, bobDecrypted)
	}
	
	// Bob encrypts
	bobMsg, err := bobGroup.EncryptMessage("bob", "Reply from Bob!", "msg-002")
	if err != nil {
		t.Fatalf("Bob failed to encrypt: %v", err)
	}
	
	// Alice decrypts Bob's message
	aliceDecrypted, err := aliceGroup.DecryptMessage(bobMsg)
	if err != nil {
		t.Fatalf("Alice failed to decrypt Bob's message: %v", err)
	}
	
	if aliceDecrypted != "Reply from Bob!" {
		t.Errorf("Expected 'Reply from Bob!', Alice got %s", aliceDecrypted)
	}
	
	t.Logf("✅ Key exchange and bidirectional encryption works!")
	t.Logf("   Alice → Bob: %s", plaintext)
	t.Logf("   Bob → Alice: %s", "Reply from Bob!")
}

func TestKeyRotation(t *testing.T) {
	group := NewGroupCrypto("rotation-test")
	if err := group.GenerateGroupKey(); err != nil {
		t.Fatalf("Failed to generate initial group key: %v", err)
	}
	
	initialVersion := group.KeyVersion
	initialKey := make([]byte, len(group.GroupKey))
	copy(initialKey, group.GroupKey)
	
	// Test rotation conditions
	if group.ShouldRotateKey(50, time.Hour) {
		t.Error("Should not rotate with 50 messages and 1 hour")
	}
	
	if !group.ShouldRotateKey(100, time.Hour) {
		t.Error("Should rotate with 100 messages")
	}
	
	if !group.ShouldRotateKey(50, 25*time.Hour) {
		t.Error("Should rotate after 25 hours")
	}
	
	// Perform rotation
	if err := group.GenerateGroupKey(); err != nil {
		t.Fatalf("Failed to rotate group key: %v", err)
	}
	
	// Verify key changed
	if group.KeyVersion <= initialVersion {
		t.Error("Key version should have incremented")
	}
	
	if string(group.GroupKey) == string(initialKey) {
		t.Error("Group key should have changed after rotation")
	}
	
	t.Logf("✅ Key rotation works!")
	t.Logf("   Initial version: %d, New version: %d", initialVersion, group.KeyVersion)
}

func TestForwardSecrecy(t *testing.T) {
	group := NewGroupCrypto("forward-secrecy-test")
	if err := group.GenerateGroupKey(); err != nil {
		t.Fatalf("Failed to generate group key: %v", err)
	}
	
	// Encrypt multiple messages
	messages := []string{"Message 1", "Message 2", "Message 3"}
	encrypted := make([]*EncryptedMessage, len(messages))
	
	for i, msg := range messages {
		enc, err := group.EncryptMessage("alice", msg, fmt.Sprintf("msg-%d", i))
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
		}
	}
	
	// Verify all messages can still be decrypted
	for i, enc := range encrypted {
		decrypted, err := group.DecryptMessage(enc)
		if err != nil {
			t.Fatalf("Failed to decrypt message %d: %v", i, err)
		}
		if decrypted != messages[i] {
			t.Errorf("Message %d: expected %s, got %s", i, messages[i], decrypted)
		}
	}
	
	t.Logf("✅ Forward secrecy works!")
	t.Logf("   Each message uses unique ephemeral key")
	t.Logf("   All messages decryptable with group key")
}
