package service

import (
	"context"
	"net"
	"testing"
	"time"

	"rat/internal/interfaces"
	"rat/internal/logger"
	"rat/internal/models"
)

// MockConfig for testing
type MockConfigForDebug struct {
	username string
	room     string
}

func (m *MockConfigForDebug) GetUsername() string    { return m.username }
func (m *MockConfigForDebug) GetRoom() string        { return m.room }
func (m *MockConfigForDebug) GetPort() int           { return 8080 }
func (m *MockConfigForDebug) GetLogLevel() string    { return "INFO" }
func (m *MockConfigForDebug) GetServiceName() string { return "test-chat" }
func (m *MockConfigForDebug) GetServiceType() string { return "_test._tcp" }
func (m *MockConfigForDebug) GetQuiet() bool         { return false }
func (m *MockConfigForDebug) GetLogFile() string     { return "" }

// MockDiscovery for testing
type MockDiscoveryForDebug struct{}

func (m *MockDiscoveryForDebug) Start(ctx context.Context) error                         { return nil }
func (m *MockDiscoveryForDebug) Stop() error                                             { return nil }
func (m *MockDiscoveryForDebug) SetListenPort(port int)                                 {}
func (m *MockDiscoveryForDebug) GetPeers() []*models.Peer                               { return nil }
func (m *MockDiscoveryForDebug) SetPeerHandler(handler interfaces.PeerEventHandler)     {}
func (m *MockDiscoveryForDebug) GetLocalPeer() *models.Peer {
	return &models.Peer{
		ID:       "test-peer",
		Username: "test-user",
		Address:  net.ParseIP("127.0.0.1"),
		Port:     8080,
		Room:     "secure-room",
	}
}

// SharedMockTransport allows multiple services to communicate
type SharedMockTransport struct {
	services map[string]*LocalChatService
	messages []*models.Message
}

func NewSharedMockTransport() *SharedMockTransport {
	return &SharedMockTransport{
		services: make(map[string]*LocalChatService),
		messages: make([]*models.Message, 0),
	}
}

func (s *SharedMockTransport) RegisterService(username string, service *LocalChatService) {
	s.services[username] = service
}

func (s *SharedMockTransport) Start(ctx context.Context) error { return nil }
func (s *SharedMockTransport) Stop() error                     { return nil }
func (s *SharedMockTransport) SendMessage(peerID string, message *models.Message) error {
	return nil
}

func (s *SharedMockTransport) BroadcastMessage(message *models.Message) error {
	s.messages = append(s.messages, message)
	
	// Deliver to all services in the same room
	for username, service := range s.services {
		if username != message.From && service.currentRoom == message.Room {
			// Simulate async delivery
			go func(svc *LocalChatService, msg *models.Message) {
				time.Sleep(1 * time.Millisecond)
				select {
				case svc.incomingMessages <- msg:
				default:
				}
			}(service, message)
		}
	}
	
	return nil
}

func (s *SharedMockTransport) SetMessageHandler(handler interfaces.MessageEventHandler) {}
func (s *SharedMockTransport) ConnectToPeer(peer *models.Peer) error                     { return nil }
func (s *SharedMockTransport) DisconnectFromPeer(peerID string) error                    { return nil }
func (s *SharedMockTransport) GetConnectedPeers() []*models.Peer                        { return nil }
func (s *SharedMockTransport) GetListenAddress() net.Addr                               { return nil }

// TestDebugKeyDistribution - debug the actual key distribution issue
func TestDebugKeyDistribution(t *testing.T) {
	// Create shared transport
	transport := NewSharedMockTransport()
	
	// Create Alice using the real constructor
	aliceConfig := &MockConfigForDebug{username: "alice", room: "secure-room"}
	aliceLogger := logger.New(logger.LevelInfo)
	aliceDiscovery := &MockDiscoveryForDebug{}
	
	alice := NewLocalChatService(aliceConfig, aliceLogger, aliceDiscovery, transport).(*LocalChatService)
	transport.RegisterService("alice", alice)
	
	// Create Bob using the real constructor
	bobConfig := &MockConfigForDebug{username: "bob", room: "secure-room"}
	bobLogger := logger.New(logger.LevelInfo)
	bobDiscovery := &MockDiscoveryForDebug{}
	
	bob := NewLocalChatService(bobConfig, bobLogger, bobDiscovery, transport).(*LocalChatService)
	transport.RegisterService("bob", bob)
	
	// Start both services
	ctx := context.Background()
	if err := alice.Start(ctx); err != nil {
		t.Fatalf("Failed to start Alice: %v", err)
	}
	defer alice.Stop()
	
	if err := bob.Start(ctx); err != nil {
		t.Fatalf("Failed to start Bob: %v", err)
	}
	defer bob.Stop()
	
	t.Logf("✅ Both services started successfully")
	
	// Register both users
	if err := alice.RegisterUser("alice"); err != nil {
		t.Logf("Alice registration failed: %v", err)
	}
	if err := bob.RegisterUser("bob"); err != nil {
		t.Logf("Bob registration failed: %v", err)
	}
	
	// Alice joins room first
	if err := alice.ConnectRoom("secure-room"); err != nil {
		t.Fatalf("Alice failed to connect to room: %v", err)
	}
	
	time.Sleep(10 * time.Millisecond)
	
	// Check Alice's group crypto
	aliceGroupCrypto := alice.getGroupCrypto("secure-room")
	if aliceGroupCrypto == nil {
		t.Fatal("❌ Alice should have group crypto after joining room")
	}
	
	t.Logf("✅ Alice joined room and has group key (version %d)", aliceGroupCrypto.KeyVersion)
	
	// Bob joins room (should trigger key request)
	if err := bob.ConnectRoom("secure-room"); err != nil {
		t.Fatalf("Bob failed to connect to room: %v", err)
	}
	
	// Give time for key exchange
	time.Sleep(100 * time.Millisecond)
	
	// Debug: Check what messages were sent
	t.Logf("📨 Messages exchanged (%d total):", len(transport.messages))
	for i, msg := range transport.messages {
		content := msg.Content
		if len(content) > 50 {
			content = content[:50] + "..."
		}
		t.Logf("   %d: %s -> %s: %s (type: %s, room: %s)", 
			i+1, msg.From, msg.To, content, msg.Type, msg.Room)
	}
	
	// Check Bob's group crypto
	bobGroupCrypto := bob.getGroupCrypto("secure-room")
	if bobGroupCrypto == nil {
		t.Fatal("❌ Bob should have group crypto after joining room")
	}
	
	t.Logf("📋 Key distribution status:")
	t.Logf("   Alice key version: %d", aliceGroupCrypto.KeyVersion)
	t.Logf("   Bob key version: %d", bobGroupCrypto.KeyVersion)
	t.Logf("   Alice members: %d", len(aliceGroupCrypto.Members))
	t.Logf("   Bob members: %d", len(bobGroupCrypto.Members))
	
	// Check if keys match
	if string(bobGroupCrypto.GroupKey) != string(aliceGroupCrypto.GroupKey) {
		t.Errorf("❌ KEYS DON'T MATCH!")
		t.Logf("   Alice key: %x", aliceGroupCrypto.GroupKey[:min(8, len(aliceGroupCrypto.GroupKey))])
		t.Logf("   Bob key: %x", bobGroupCrypto.GroupKey[:min(8, len(bobGroupCrypto.GroupKey))])
		
		// Check peer public keys
		t.Logf("📋 Peer public keys:")
		t.Logf("   Alice has Bob's key: %v", alice.getPeerPublicKey("bob") != nil)
		t.Logf("   Bob has Alice's key: %v", bob.getPeerPublicKey("alice") != nil)
		
		return
	}
	
	if bobGroupCrypto.KeyVersion != aliceGroupCrypto.KeyVersion {
		t.Errorf("❌ Key versions don't match: Alice=%d, Bob=%d", 
			aliceGroupCrypto.KeyVersion, bobGroupCrypto.KeyVersion)
		return
	}
	
	t.Logf("✅ Key distribution successful!")
	
	// Test encrypted message
	plaintext := "Hello Bob! This is encrypted! 🔐"
	
	if err := alice.SendMessage(plaintext, "broadcast"); err != nil {
		t.Fatalf("Alice failed to send message: %v", err)
	}
	
	time.Sleep(50 * time.Millisecond)
	
	// Check if Bob received the decrypted message
	bobMessages := bob.GetMessages()
	found := false
	for _, msg := range bobMessages {
		if msg.From == "alice" && msg.Type == models.MessageTypeChat {
			if msg.Content == plaintext {
				found = true
				t.Logf("✅ Bob successfully decrypted: %s", msg.Content)
			} else {
				t.Logf("❌ Bob received garbled message: %s", msg.Content)
			}
			break
		}
	}
	
	if !found {
		t.Errorf("❌ Bob did not receive Alice's message")
		t.Logf("📨 Bob's messages:")
		for i, msg := range bobMessages {
			t.Logf("   %d: %s -> %s: %s", i+1, msg.From, msg.To, msg.Content)
		}
	}
	
	t.Logf("🎉 Debug test completed!")
}
