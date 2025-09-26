package service

import (
	"fmt"
	"testing"

	"rat/internal/crypto"
	"rat/internal/logger"
	"rat/internal/models"
)

// TestAutomaticKeyDistribution tests the complete key distribution flow
func TestAutomaticKeyDistribution(t *testing.T) {
	// Create Alice (existing room member)
	alice := &LocalChatService{
		username:          "alice",
		currentRoom:       "secure-room",
		registered:        true,
		encryptionEnabled: true,
		groupCrypto:       make(map[string]*crypto.GroupCrypto),
		peerPublicKeys:    make(map[string][]byte),
		logger:            logger.New(logger.LevelInfo),
		messages:          make([]*models.Message, 0),
	}
	
	// Create Bob (new member joining)
	bob := &LocalChatService{
		username:          "bob",
		currentRoom:       "secure-room",
		registered:        true,
		encryptionEnabled: true,
		groupCrypto:       make(map[string]*crypto.GroupCrypto),
		peerPublicKeys:    make(map[string][]byte),
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
	
	// Alice initializes group crypto (she's already in the room)
	if err := alice.initializeGroupCrypto("secure-room"); err != nil {
		t.Fatalf("Failed to initialize Alice's group crypto: %v", err)
	}
	
	// Bob initializes group crypto (but doesn't have the group key yet)
	if err := bob.initializeGroupCrypto("secure-room"); err != nil {
		t.Fatalf("Failed to initialize Bob's group crypto: %v", err)
	}
	
	t.Logf("✅ Setup complete - Alice has group key, Bob needs it")
	
	// Step 1: Bob sends key request (simulating joining the room)
	bobPublicKeyB64 := crypto.ToBase64(bob.GetPublicKey())
	keyRequest := models.NewKeyRequestMessage("bob", "secure-room", bobPublicKeyB64)
	
	t.Logf("📤 Bob sends key request: %s", bobPublicKeyB64[:16]+"...")
	
	// Step 2: Alice receives and processes the key request
	// We need to manually handle this since we don't have a real transport
	// Simulate the key request processing without the transport call
	
	// Decode Bob's public key from the request
	bobPublicKey, err := crypto.FromBase64(keyRequest.Content)
	if err != nil {
		t.Fatalf("Failed to decode Bob's public key: %v", err)
	}
	
	// Store Bob's public key
	alice.storePeerPublicKey("bob", bobPublicKey)
	
	// Get Alice's group crypto
	aliceGroupCrypto := alice.getGroupCrypto("secure-room")
	if aliceGroupCrypto == nil {
		t.Fatal("Alice should have group crypto")
	}
	
	// Add Bob to Alice's group
	aliceGroupCrypto.AddMember("bob", bobPublicKey)
	
	// Verify Alice stored Bob's public key
	storedBobKey := alice.getPeerPublicKey("bob")
	if storedBobKey == nil {
		t.Fatal("Alice should have stored Bob's public key")
	}
	if string(storedBobKey) != string(bob.GetPublicKey()) {
		t.Error("Stored public key doesn't match Bob's actual public key")
	}
	
	// Verify Alice added Bob to her group crypto (already retrieved above)
	if _, exists := aliceGroupCrypto.Members["bob"]; !exists {
		t.Error("Alice should have added Bob to group members")
	}
	
	t.Logf("✅ Alice processed key request and added Bob as member")
	
	// Step 3: Simulate Alice's key response (normally sent via transport)
	// We need to manually create the key response since we don't have real transport
	alicePublicKey := alice.GetPublicKey()
	
	// Store Alice's public key in Bob's peer keys (simulating previous exchange)
	bob.storePeerPublicKey("alice", alicePublicKey)
	
	// Create the key exchange message that Alice would send
	keyExchangeMsg, err := aliceGroupCrypto.CreateKeyExchangeMessage("bob", bob.GetPublicKey(), alice.userPrivateKey)
	if err != nil {
		t.Fatalf("Failed to create key exchange message: %v", err)
	}
	
	encryptedGroupKeyB64 := crypto.ToBase64(keyExchangeMsg.EncryptedGroupKey)
	keyResponseContent := fmt.Sprintf("%s:%d", encryptedGroupKeyB64, keyExchangeMsg.KeyVersion)
	keyResponse := models.NewKeyResponseMessage("alice", "bob", "secure-room", keyResponseContent)
	
	t.Logf("📤 Alice sends encrypted group key to Bob")
	
	// Step 4: Bob receives and processes the key response
	bob.handleKeyResponse(keyResponse)
	
	// Verify Bob received the group key
	bobGroupCrypto := bob.getGroupCrypto("secure-room")
	if bobGroupCrypto == nil {
		t.Fatal("Bob should have group crypto")
	}
	
	// Verify Bob has the same group key as Alice
	if string(bobGroupCrypto.GroupKey) != string(aliceGroupCrypto.GroupKey) {
		t.Error("Bob and Alice should have the same group key")
	}
	
	if bobGroupCrypto.KeyVersion != aliceGroupCrypto.KeyVersion {
		t.Errorf("Key versions should match: Alice=%d, Bob=%d", aliceGroupCrypto.KeyVersion, bobGroupCrypto.KeyVersion)
	}
	
	t.Logf("✅ Bob successfully received and processed group key")
	
	// Step 5: Test encrypted communication between Alice and Bob
	plaintext := "Welcome to the secure room, Bob! 🔐"
	
	// Alice encrypts message
	encryptedMsg, err := aliceGroupCrypto.EncryptMessage("alice", plaintext, "welcome-msg-001")
	if err != nil {
		t.Fatalf("Alice failed to encrypt message: %v", err)
	}
	
	// Create network message
	networkMessage := &models.Message{
		MessageID: "welcome-msg-001",
		From:      "alice",
		To:        "broadcast",
		Content:   fmt.Sprintf("🔒[ENCRYPTED:%s:%s:%s:%d]", 
			crypto.ToBase64(encryptedMsg.EncryptedContent), 
			crypto.ToBase64(encryptedMsg.WrappedKey), 
			crypto.ToBase64(encryptedMsg.Nonce), 
			encryptedMsg.KeyVersion),
		Room:      "secure-room",
		Type:      models.MessageTypeChat,
		Timestamp: encryptedMsg.Timestamp,
	}
	
	// Bob decrypts message
	decryptedMessage := bob.decryptIncomingMessage(networkMessage)
	
	if decryptedMessage.Content != plaintext {
		t.Errorf("Expected '%s', Bob decrypted '%s'", plaintext, decryptedMessage.Content)
	}
	
	t.Logf("📥 Bob successfully decrypted: %s", decryptedMessage.Content)
	
	// Step 6: Test Bob sending encrypted message back
	replyText := "Thanks Alice! I can send encrypted messages too! 🚀"
	
	bobEncryptedMsg, err := bobGroupCrypto.EncryptMessage("bob", replyText, "reply-msg-001")
	if err != nil {
		t.Fatalf("Bob failed to encrypt reply: %v", err)
	}
	
	bobNetworkMessage := &models.Message{
		MessageID: "reply-msg-001",
		From:      "bob",
		To:        "broadcast",
		Content:   fmt.Sprintf("🔒[ENCRYPTED:%s:%s:%s:%d]", 
			crypto.ToBase64(bobEncryptedMsg.EncryptedContent), 
			crypto.ToBase64(bobEncryptedMsg.WrappedKey), 
			crypto.ToBase64(bobEncryptedMsg.Nonce), 
			bobEncryptedMsg.KeyVersion),
		Room:      "secure-room",
		Type:      models.MessageTypeChat,
		Timestamp: bobEncryptedMsg.Timestamp,
	}
	
	aliceDecryptedReply := alice.decryptIncomingMessage(bobNetworkMessage)
	
	if aliceDecryptedReply.Content != replyText {
		t.Errorf("Expected '%s', Alice decrypted '%s'", replyText, aliceDecryptedReply.Content)
	}
	
	t.Logf("📥 Alice successfully decrypted Bob's reply: %s", aliceDecryptedReply.Content)
	
	t.Logf("🎉 Automatic key distribution test passed!")
	t.Logf("   ✓ Bob requested group key")
	t.Logf("   ✓ Alice shared encrypted group key")
	t.Logf("   ✓ Bob received and processed group key")
	t.Logf("   ✓ Bidirectional encrypted communication working")
	t.Logf("   ✓ Both users have same group key (version %d)", bobGroupCrypto.KeyVersion)
}

// TestKeyDistributionWithMultipleMembers tests key distribution with multiple existing members
func TestKeyDistributionWithMultipleMembers(t *testing.T) {
	// Create Alice and Charlie (existing members)
	alice := createTestCryptoService(t, "alice", "secure-room")
	charlie := createTestCryptoService(t, "charlie", "secure-room")
	
	// Initialize group crypto for existing members
	if err := alice.initializeGroupCrypto("secure-room"); err != nil {
		t.Fatalf("Failed to initialize Alice's group crypto: %v", err)
	}
	if err := charlie.initializeGroupCrypto("secure-room"); err != nil {
		t.Fatalf("Failed to initialize Charlie's group crypto: %v", err)
	}
	
	// Sync Alice and Charlie's group keys (they're both existing members)
	aliceGroupCrypto := alice.getGroupCrypto("secure-room")
	charlieGroupCrypto := charlie.getGroupCrypto("secure-room")
	
	// Charlie gets Alice's group key
	charlieGroupCrypto.GroupKey = make([]byte, len(aliceGroupCrypto.GroupKey))
	copy(charlieGroupCrypto.GroupKey, aliceGroupCrypto.GroupKey)
	charlieGroupCrypto.KeyVersion = aliceGroupCrypto.KeyVersion
	
	// Add each other as members
	aliceGroupCrypto.AddMember("charlie", charlie.GetPublicKey())
	charlieGroupCrypto.AddMember("alice", alice.GetPublicKey())
	
	// Store each other's public keys
	alice.storePeerPublicKey("charlie", charlie.GetPublicKey())
	charlie.storePeerPublicKey("alice", alice.GetPublicKey())
	
	// Create Bob (new member)
	bob := createTestCryptoService(t, "bob", "secure-room")
	if err := bob.initializeGroupCrypto("secure-room"); err != nil {
		t.Fatalf("Failed to initialize Bob's group crypto: %v", err)
	}
	
	t.Logf("✅ Setup: Alice and Charlie are existing members, Bob wants to join")
	
	// Bob sends key request
	bobPublicKeyB64 := crypto.ToBase64(bob.GetPublicKey())
	keyRequest := models.NewKeyRequestMessage("bob", "secure-room", bobPublicKeyB64)
	
	// Both Alice and Charlie should respond (in real scenario, first responder wins)
	// Let's have Alice respond
	alice.handleKeyRequest(keyRequest)
	
	// Verify Alice added Bob
	if _, exists := aliceGroupCrypto.Members["bob"]; !exists {
		t.Error("Alice should have added Bob to group members")
	}
	
	// Simulate Alice's response reaching Bob
	bob.storePeerPublicKey("alice", alice.GetPublicKey())
	
	keyExchangeMsg, err := aliceGroupCrypto.CreateKeyExchangeMessage("bob", bob.GetPublicKey(), alice.userPrivateKey)
	if err != nil {
		t.Fatalf("Failed to create key exchange message: %v", err)
	}
	
	encryptedGroupKeyB64 := crypto.ToBase64(keyExchangeMsg.EncryptedGroupKey)
	keyResponse := models.NewKeyResponseMessage("alice", "bob", "secure-room", encryptedGroupKeyB64)
	
	bob.handleKeyResponse(keyResponse)
	
	// Verify Bob has the group key
	bobGroupCrypto := bob.getGroupCrypto("secure-room")
	if string(bobGroupCrypto.GroupKey) != string(aliceGroupCrypto.GroupKey) {
		t.Error("Bob should have the same group key as Alice")
	}
	
	// Test 3-way encrypted communication
	// Alice sends message
	aliceMsg := "Hello everyone! 👋"
	encryptedAliceMsg, _ := aliceGroupCrypto.EncryptMessage("alice", aliceMsg, "alice-msg-001")
	aliceNetworkMsg := createEncryptedNetworkMessage("alice", encryptedAliceMsg)
	
	// Bob and Charlie should both be able to decrypt
	bobDecrypted := bob.decryptIncomingMessage(aliceNetworkMsg)
	charlieDecrypted := charlie.decryptIncomingMessage(aliceNetworkMsg)
	
	if bobDecrypted.Content != aliceMsg {
		t.Errorf("Bob failed to decrypt Alice's message: got '%s'", bobDecrypted.Content)
	}
	if charlieDecrypted.Content != aliceMsg {
		t.Errorf("Charlie failed to decrypt Alice's message: got '%s'", charlieDecrypted.Content)
	}
	
	t.Logf("🎉 Multi-member key distribution test passed!")
	t.Logf("   ✓ Bob joined group with existing members Alice and Charlie")
	t.Logf("   ✓ All members can decrypt each other's messages")
}

// Helper function to create a test crypto service
func createTestCryptoService(t *testing.T, username, room string) *LocalChatService {
	service := &LocalChatService{
		username:          username,
		currentRoom:       room,
		registered:        true,
		encryptionEnabled: true,
		groupCrypto:       make(map[string]*crypto.GroupCrypto),
		peerPublicKeys:    make(map[string][]byte),
		logger:            logger.New(logger.LevelInfo),
		messages:          make([]*models.Message, 0),
	}
	
	if err := service.initializeCrypto(); err != nil {
		t.Fatalf("Failed to initialize crypto for %s: %v", username, err)
	}
	
	return service
}

// Helper function to create encrypted network message
func createEncryptedNetworkMessage(sender string, encryptedMsg *crypto.EncryptedMessage) *models.Message {
	return &models.Message{
		MessageID: encryptedMsg.MessageID,
		From:      sender,
		To:        "broadcast",
		Content:   fmt.Sprintf("🔒[ENCRYPTED:%s:%s:%s:%d]", 
			crypto.ToBase64(encryptedMsg.EncryptedContent), 
			crypto.ToBase64(encryptedMsg.WrappedKey), 
			crypto.ToBase64(encryptedMsg.Nonce), 
			encryptedMsg.KeyVersion),
		Room:      encryptedMsg.Room,
		Type:      models.MessageTypeChat,
		Timestamp: encryptedMsg.Timestamp,
	}
}
