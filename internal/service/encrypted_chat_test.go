package service

import (
	"context"
	"fmt"
	"net"
	"testing"

	"rat/internal/crypto"
	"rat/internal/interfaces"
	"rat/internal/logger"
	"rat/internal/models"
)

// MockConfig implements interfaces.Configuration for testing
type MockConfig struct {
	username    string
	room        string
	port        int
	logLevel    string
	serviceName string
	serviceType string
}

func (m *MockConfig) GetUsername() string    { return m.username }
func (m *MockConfig) GetRoom() string        { return m.room }
func (m *MockConfig) GetPort() int           { return m.port }
func (m *MockConfig) GetLogLevel() string    { return m.logLevel }
func (m *MockConfig) GetServiceName() string { return m.serviceName }
func (m *MockConfig) GetServiceType() string { return "_test._tcp" }
func (m *MockConfig) GetQuiet() bool         { return false }
func (m *MockConfig) GetLogFile() string     { return "" }

// MockDiscovery implements interfaces.DiscoveryService for testing
type MockDiscovery struct {
	peers []*models.Peer
}

func (m *MockDiscovery) Start(ctx context.Context) error                         { return nil }
func (m *MockDiscovery) Stop() error                                             { return nil }
func (m *MockDiscovery) SetListenPort(port int)                                 {}
func (m *MockDiscovery) GetPeers() []*models.Peer                               { return m.peers }
func (m *MockDiscovery) SetPeerHandler(handler interfaces.PeerEventHandler)     {}
func (m *MockDiscovery) GetLocalPeer() *models.Peer                             { return nil }

// MockTransport implements interfaces.MessageTransport for testing
type MockTransport struct {
	messages       []*models.Message
	messageHandler interfaces.MessageEventHandler
	peers          []*models.Peer
}

func (m *MockTransport) Start(ctx context.Context) error                                { return nil }
func (m *MockTransport) Stop() error                                                   { return nil }
func (m *MockTransport) SendMessage(peerID string, message *models.Message) error     { return nil }
func (m *MockTransport) BroadcastMessage(message *models.Message) error               { m.messages = append(m.messages, message); return nil }
func (m *MockTransport) SetMessageHandler(handler interfaces.MessageEventHandler)     { m.messageHandler = handler }
func (m *MockTransport) ConnectToPeer(peer *models.Peer) error                        { return nil }
func (m *MockTransport) DisconnectFromPeer(peerID string) error                       { return nil }
func (m *MockTransport) GetConnectedPeers() []*models.Peer                            { return m.peers }
func (m *MockTransport) GetListenAddress() net.Addr                                   { return nil }

func TestCryptoInitialization(t *testing.T) {
	// Create chat service with encryption enabled
	config := &MockConfig{username: "alice", room: "secure-room", logLevel: "INFO"}
	testLogger := logger.New(logger.LevelInfo)
	discovery := &MockDiscovery{}
	transport := &MockTransport{}
	
	service := NewLocalChatService(config, testLogger, discovery, transport).(*LocalChatService)
	
	// Start service (should initialize crypto)
	ctx := context.Background()
	if err := service.Start(ctx); err != nil {
		t.Fatalf("Failed to start service: %v", err)
	}
	defer service.Stop()
	
	// Verify crypto was initialized
	if service.userPrivateKey == nil {
		t.Error("User private key not initialized")
	}
	
	if service.userPublicKey == nil {
		t.Error("User public key not initialized")
	}
	
	if !service.encryptionEnabled {
		t.Error("Encryption should be enabled by default")
	}
	
	// Test public key retrieval
	publicKey := service.GetPublicKey()
	if publicKey == nil {
		t.Error("GetPublicKey returned nil")
	}
	
	if len(publicKey) != 32 {
		t.Errorf("Expected 32-byte public key, got %d bytes", len(publicKey))
	}
	
	t.Logf("✅ Crypto initialization works!")
	t.Logf("   Public key length: %d bytes", len(publicKey))
	t.Logf("   Encryption enabled: %v", service.encryptionEnabled)
}

func TestGroupCryptoInitialization(t *testing.T) {
	// Create and start service
	config := &MockConfig{username: "alice", room: ""}
	service := createTestService(t, config)
	defer service.Stop()
	
	// Register user
	if err := service.RegisterUser("alice"); err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}
	
	// Connect to room (should initialize group crypto)
	if err := service.ConnectRoom("secure-room"); err != nil {
		t.Fatalf("Failed to connect to room: %v", err)
	}
	
	// Verify group crypto was initialized
	groupCrypto := service.getGroupCrypto("secure-room")
	if groupCrypto == nil {
		t.Fatal("Group crypto not initialized for room")
	}
	
	if groupCrypto.RoomID != "secure-room" {
		t.Errorf("Expected room ID 'secure-room', got '%s'", groupCrypto.RoomID)
	}
	
	if len(groupCrypto.Members) != 1 {
		t.Errorf("Expected 1 member, got %d", len(groupCrypto.Members))
	}
	
	if _, exists := groupCrypto.Members["alice"]; !exists {
		t.Error("Alice not found in group members")
	}
	
	t.Logf("✅ Group crypto initialization works!")
	t.Logf("   Room ID: %s", groupCrypto.RoomID)
	t.Logf("   Key version: %d", groupCrypto.KeyVersion)
	t.Logf("   Members: %d", len(groupCrypto.Members))
}

func TestEncryptedMessageFlow(t *testing.T) {
	// Create two chat services (Alice and Bob)
	aliceService := createTestService(t, &MockConfig{username: "alice"})
	bobService := createTestService(t, &MockConfig{username: "bob"})
	defer aliceService.Stop()
	defer bobService.Stop()
	
	// Register both users
	if err := aliceService.RegisterUser("alice"); err != nil {
		t.Fatalf("Failed to register Alice: %v", err)
	}
	if err := bobService.RegisterUser("bob"); err != nil {
		t.Fatalf("Failed to register Bob: %v", err)
	}
	
	// Both connect to same room
	if err := aliceService.ConnectRoom("secure-room"); err != nil {
		t.Fatalf("Alice failed to connect to room: %v", err)
	}
	if err := bobService.ConnectRoom("secure-room"); err != nil {
		t.Fatalf("Bob failed to connect to room: %v", err)
	}
	
	// Get group crypto instances
	aliceGroupCrypto := aliceService.getGroupCrypto("secure-room")
	bobGroupCrypto := bobService.getGroupCrypto("secure-room")
	
	if aliceGroupCrypto == nil || bobGroupCrypto == nil {
		t.Fatal("Group crypto not initialized for both users")
	}
	
	// Simulate key exchange (in real scenario, this would happen via network)
	// For now, we'll manually sync the group keys
	bobGroupCrypto.GroupKey = make([]byte, len(aliceGroupCrypto.GroupKey))
	copy(bobGroupCrypto.GroupKey, aliceGroupCrypto.GroupKey)
	bobGroupCrypto.KeyVersion = aliceGroupCrypto.KeyVersion
	bobGroupCrypto.AddMember("alice", aliceService.GetPublicKey())
	aliceGroupCrypto.AddMember("bob", bobService.GetPublicKey())
	
	// Test encrypted message creation
	plaintext := "Hello Bob, this is a secret message!"
	
	// Alice encrypts message
	encryptedMsg, err := aliceGroupCrypto.EncryptMessage("alice", plaintext, "msg-001")
	if err != nil {
		t.Fatalf("Alice failed to encrypt message: %v", err)
	}
	
	// Verify encrypted message structure
	if encryptedMsg.Sender != "alice" {
		t.Errorf("Expected sender 'alice', got '%s'", encryptedMsg.Sender)
	}
	if encryptedMsg.Room != "secure-room" {
		t.Errorf("Expected room 'secure-room', got '%s'", encryptedMsg.Room)
	}
	if len(encryptedMsg.EncryptedContent) == 0 {
		t.Error("Encrypted content is empty")
	}
	if len(encryptedMsg.WrappedKey) == 0 {
		t.Error("Wrapped key is empty")
	}
	
	// Bob decrypts message
	decrypted, err := bobGroupCrypto.DecryptMessage(encryptedMsg)
	if err != nil {
		t.Fatalf("Bob failed to decrypt message: %v", err)
	}
	
	if decrypted != plaintext {
		t.Errorf("Expected '%s', got '%s'", plaintext, decrypted)
	}
	
	t.Logf("✅ Encrypted message flow works!")
	t.Logf("   Original: %s", plaintext)
	t.Logf("   Decrypted: %s", decrypted)
	t.Logf("   Encrypted size: %d bytes", len(encryptedMsg.EncryptedContent))
}

func TestEncryptionToggle(t *testing.T) {
	service := createTestService(t, &MockConfig{username: "alice"})
	defer service.Stop()
	
	// Verify encryption is enabled by default
	if !service.encryptionEnabled {
		t.Error("Encryption should be enabled by default")
	}
	
	// Disable encryption
	service.EnableEncryption(false)
	if service.encryptionEnabled {
		t.Error("Encryption should be disabled after calling EnableEncryption(false)")
	}
	
	// Re-enable encryption
	service.EnableEncryption(true)
	if !service.encryptionEnabled {
		t.Error("Encryption should be enabled after calling EnableEncryption(true)")
	}
	
	t.Logf("✅ Encryption toggle works!")
}

func TestMultipleRooms(t *testing.T) {
	service := createTestService(t, &MockConfig{username: "alice"})
	defer service.Stop()
	
	if err := service.RegisterUser("alice"); err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}
	
	// Connect to multiple rooms
	rooms := []string{"room1", "room2", "room3"}
	
	for _, room := range rooms {
		if err := service.ConnectRoom(room); err != nil {
			t.Fatalf("Failed to connect to room %s: %v", room, err)
		}
		
		// Verify group crypto exists for each room
		groupCrypto := service.getGroupCrypto(room)
		if groupCrypto == nil {
			t.Errorf("Group crypto not initialized for room %s", room)
			continue
		}
		
		if groupCrypto.RoomID != room {
			t.Errorf("Expected room ID %s, got %s", room, groupCrypto.RoomID)
		}
	}
	
	// Verify all rooms have separate crypto instances
	service.cryptoMutex.RLock()
	if len(service.groupCrypto) != len(rooms) {
		t.Errorf("Expected %d group crypto instances, got %d", len(rooms), len(service.groupCrypto))
	}
	service.cryptoMutex.RUnlock()
	
	t.Logf("✅ Multiple rooms crypto works!")
	t.Logf("   Rooms created: %v", rooms)
	t.Logf("   Crypto instances: %d", len(service.groupCrypto))
}

func TestForwardSecrecyInService(t *testing.T) {
	service := createTestService(t, &MockConfig{username: "alice"})
	defer service.Stop()
	
	if err := service.RegisterUser("alice"); err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}
	
	if err := service.ConnectRoom("secure-room"); err != nil {
		t.Fatalf("Failed to connect to room: %v", err)
	}
	
	groupCrypto := service.getGroupCrypto("secure-room")
	if groupCrypto == nil {
		t.Fatal("Group crypto not initialized")
	}
	
	// Encrypt multiple messages
	messages := []string{"Message 1", "Message 2", "Message 3"}
	encrypted := make([]*crypto.EncryptedMessage, len(messages))
	
	for i, msg := range messages {
		enc, err := groupCrypto.EncryptMessage("alice", msg, fmt.Sprintf("msg-%d", i))
		if err != nil {
			t.Fatalf("Failed to encrypt message %d: %v", i, err)
		}
		encrypted[i] = enc
	}
	
	// Verify each message uses different ephemeral key
	for i := 0; i < len(encrypted)-1; i++ {
		for j := i + 1; j < len(encrypted); j++ {
			if string(encrypted[i].WrappedKey) == string(encrypted[j].WrappedKey) {
				t.Errorf("Messages %d and %d have same wrapped key (no forward secrecy)", i, j)
			}
		}
	}
	
	// Verify all messages can be decrypted
	for i, enc := range encrypted {
		decrypted, err := groupCrypto.DecryptMessage(enc)
		if err != nil {
			t.Fatalf("Failed to decrypt message %d: %v", i, err)
		}
		if decrypted != messages[i] {
			t.Errorf("Message %d: expected %s, got %s", i, messages[i], decrypted)
		}
	}
	
	t.Logf("✅ Forward secrecy in service works!")
	t.Logf("   Messages encrypted: %d", len(messages))
	t.Logf("   Each uses unique ephemeral key")
}

// Helper function to create a test service
func createTestService(t *testing.T, config *MockConfig) *LocalChatService {
	if config.logLevel == "" {
		config.logLevel = "INFO"
	}
	
	testLogger := logger.New(logger.LogLevel(config.logLevel))
	discovery := &MockDiscovery{}
	transport := &MockTransport{}
	
	service := NewLocalChatService(config, testLogger, discovery, transport).(*LocalChatService)
	
	ctx := context.Background()
	if err := service.Start(ctx); err != nil {
		t.Fatalf("Failed to start service: %v", err)
	}
	
	return service
}
