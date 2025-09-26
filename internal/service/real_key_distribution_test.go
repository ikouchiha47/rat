package service

import (
	"context"
	"net"
	"testing"
	"time"

	"rat/internal/crypto"
	"rat/internal/interfaces"
	"rat/internal/logger"
	"rat/internal/models"
)

// MockTransportForKeyDistribution simulates real transport for key distribution testing
type MockTransportForKeyDistribution struct {
	messages []struct {
		message *models.Message
		sender  *LocalChatService
	}
	services map[string]*LocalChatService
}

func NewMockTransportForKeyDistribution() *MockTransportForKeyDistribution {
	return &MockTransportForKeyDistribution{
		messages: make([]struct {
			message *models.Message
			sender  *LocalChatService
		}, 0),
		services: make(map[string]*LocalChatService),
	}
}

func (m *MockTransportForKeyDistribution) RegisterService(username string, service *LocalChatService) {
	m.services[username] = service
}

func (m *MockTransportForKeyDistribution) BroadcastMessage(message *models.Message) error {
	// Store the message and simulate delivery to all other services
	sender := m.services[message.From]
	
	m.messages = append(m.messages, struct {
		message *models.Message
		sender  *LocalChatService
	}{message: message, sender: sender})
	
	// Deliver to all other services in the same room
	for username, service := range m.services {
		if username != message.From && service.currentRoom == message.Room {
			// Simulate message delivery
			go func(svc *LocalChatService, msg *models.Message) {
				// Add small delay to simulate network
				time.Sleep(1 * time.Millisecond)
				
				// Deliver via the incoming message channel
				select {
				case svc.incomingMessages <- msg:
				default:
					// Channel full, drop message
				}
			}(service, message)
		}
	}
	
	return nil
}

// Implement other transport methods (not used in this test)
func (m *MockTransportForKeyDistribution) Start(ctx context.Context) error { return nil }
func (m *MockTransportForKeyDistribution) Stop() error                 { return nil }
func (m *MockTransportForKeyDistribution) SendMessage(peerID string, message *models.Message) error {
	return nil
}
func (m *MockTransportForKeyDistribution) SetMessageHandler(handler interfaces.MessageEventHandler) {}
func (m *MockTransportForKeyDistribution) ConnectToPeer(peer *models.Peer) error  { return nil }
func (m *MockTransportForKeyDistribution) DisconnectFromPeer(peerID string) error { return nil }
func (m *MockTransportForKeyDistribution) GetConnectedPeers() []*models.Peer      { return nil }
func (m *MockTransportForKeyDistribution) GetListenAddress() net.Addr            { return nil }

// TestRealKeyDistribution tests key distribution with actual message transport
func TestRealKeyDistribution(t *testing.T) {
	// Create shared transport
	transport := NewMockTransportForKeyDistribution()
	
	// Create Alice (existing room member)
	alice := &LocalChatService{
		username:          "alice",
		currentRoom:       "",
		registered:        true,
		encryptionEnabled: true,
		groupCrypto:       make(map[string]*crypto.GroupCrypto),
		peerPublicKeys:    make(map[string][]byte),
		logger:            logger.New(logger.LevelInfo),
		messages:          make([]*models.Message, 0),
		transport:         transport,
		incomingMessages:  make(chan *models.Message, 100),
		outgoingMessages:  make(chan *models.Message, 100),
		uiUpdates:         make(chan struct{}, 10),
	}
	
	// Create Bob (new member)
	bob := &LocalChatService{
		username:          "bob",
		currentRoom:       "",
		registered:        true,
		encryptionEnabled: true,
		groupCrypto:       make(map[string]*crypto.GroupCrypto),
		peerPublicKeys:    make(map[string][]byte),
		logger:            logger.New(logger.LevelInfo),
		messages:          make([]*models.Message, 0),
		transport:         transport,
		incomingMessages:  make(chan *models.Message, 100),
		outgoingMessages:  make(chan *models.Message, 100),
		uiUpdates:         make(chan struct{}, 10),
	}
	
	// Register services with transport
	transport.RegisterService("alice", alice)
	transport.RegisterService("bob", bob)
	
	// Initialize crypto for both
	if err := alice.initializeCrypto(); err != nil {
		t.Fatalf("Failed to initialize Alice's crypto: %v", err)
	}
	if err := bob.initializeCrypto(); err != nil {
		t.Fatalf("Failed to initialize Bob's crypto: %v", err)
	}
	
	// Start message processing for both services
	go alice.processIncomingMessages()
	go bob.processIncomingMessages()
	
	t.Logf("✅ Setup complete - both services initialized")
	
	// Step 1: Alice joins room first (becomes the key holder)
	if err := alice.ConnectRoom("secure-room"); err != nil {
		t.Fatalf("Alice failed to connect to room: %v", err)
	}
	
	// Give Alice time to initialize
	time.Sleep(10 * time.Millisecond)
	
	aliceGroupCrypto := alice.getGroupCrypto("secure-room")
	if aliceGroupCrypto == nil {
		t.Fatal("Alice should have group crypto after joining room")
	}
	
	t.Logf("✅ Alice joined room and has group key (version %d)", aliceGroupCrypto.KeyVersion)
	
	// Step 2: Bob joins room (should request key from Alice)
	if err := bob.ConnectRoom("secure-room"); err != nil {
		t.Fatalf("Bob failed to connect to room: %v", err)
	}
	
	// Give time for key exchange to complete
	time.Sleep(50 * time.Millisecond)
	
	bobGroupCrypto := bob.getGroupCrypto("secure-room")
	if bobGroupCrypto == nil {
		t.Fatal("Bob should have group crypto after joining room")
	}
	
	t.Logf("📋 Key distribution status:")
	t.Logf("   Alice key version: %d", aliceGroupCrypto.KeyVersion)
	t.Logf("   Bob key version: %d", bobGroupCrypto.KeyVersion)
	t.Logf("   Alice members: %d", len(aliceGroupCrypto.Members))
	t.Logf("   Bob members: %d", len(bobGroupCrypto.Members))
	
	// Check if Bob received Alice's group key
	if string(bobGroupCrypto.GroupKey) != string(aliceGroupCrypto.GroupKey) {
		t.Errorf("❌ Bob and Alice have different group keys!")
		t.Logf("   Alice key: %x", aliceGroupCrypto.GroupKey[:8])
		t.Logf("   Bob key: %x", bobGroupCrypto.GroupKey[:8])
		
		// Check what messages were exchanged
		t.Logf("📨 Messages exchanged:")
		for i, msg := range transport.messages {
			t.Logf("   %d: %s -> %s: %s (type: %s)", i+1, msg.message.From, msg.message.To, 
				msg.message.Content[:min(50, len(msg.message.Content))], msg.message.Type)
		}
		
		return // Don't continue if key distribution failed
	}
	
	if bobGroupCrypto.KeyVersion != aliceGroupCrypto.KeyVersion {
		t.Errorf("❌ Key versions don't match: Alice=%d, Bob=%d", 
			aliceGroupCrypto.KeyVersion, bobGroupCrypto.KeyVersion)
		return
	}
	
	t.Logf("✅ Key distribution successful - both have same group key!")
	
	// Step 3: Test encrypted communication
	plaintext := "Hello Bob! This should be encrypted! 🔐"
	
	// Alice sends encrypted message
	if err := alice.SendMessage(plaintext, "broadcast"); err != nil {
		t.Fatalf("Alice failed to send message: %v", err)
	}
	
	// Give time for message to be processed
	time.Sleep(20 * time.Millisecond)
	
	// Check if Bob received and decrypted the message
	bobMessages := bob.GetMessages()
	found := false
	for _, msg := range bobMessages {
		if msg.From == "alice" && msg.Content == plaintext {
			found = true
			t.Logf("✅ Bob successfully received and decrypted: %s", msg.Content)
			break
		} else if msg.From == "alice" && msg.Type == models.MessageTypeChat {
			t.Logf("❌ Bob received garbled message: %s", msg.Content)
		}
	}
	
	if !found {
		t.Errorf("❌ Bob did not receive Alice's decrypted message")
		t.Logf("📨 Bob's messages:")
		for i, msg := range bobMessages {
			t.Logf("   %d: %s -> %s: %s", i+1, msg.From, msg.To, msg.Content)
		}
	}
	
	// Step 4: Test Bob sending back
	replyText := "Hi Alice! I got your encrypted message! 🚀"
	
	if err := bob.SendMessage(replyText, "broadcast"); err != nil {
		t.Fatalf("Bob failed to send reply: %v", err)
	}
	
	time.Sleep(20 * time.Millisecond)
	
	// Check if Alice received Bob's reply
	aliceMessages := alice.GetMessages()
	found = false
	for _, msg := range aliceMessages {
		if msg.From == "bob" && msg.Content == replyText {
			found = true
			t.Logf("✅ Alice successfully received and decrypted: %s", msg.Content)
			break
		} else if msg.From == "bob" && msg.Type == models.MessageTypeChat {
			t.Logf("❌ Alice received garbled message: %s", msg.Content)
		}
	}
	
	if !found {
		t.Errorf("❌ Alice did not receive Bob's decrypted message")
	}
	
	t.Logf("🎉 Real key distribution test completed!")
	t.Logf("   ✓ Alice joined room and generated group key")
	t.Logf("   ✓ Bob joined room and requested group key")
	t.Logf("   ✓ Alice shared group key with Bob")
	t.Logf("   ✓ Both users have same group key")
	t.Logf("   ✓ Bidirectional encrypted communication working")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
