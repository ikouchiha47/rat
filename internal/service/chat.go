package service

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"rat/internal/crypto"
	"rat/internal/interfaces"
	"rat/internal/logger"
	"rat/internal/models"
)

// KeyOperationType defines the type of key operation
type KeyOperationType int

const (
	KeyOpRequest KeyOperationType = iota  // Request group key from peers
	KeyOpResponse                         // Respond to key request
	KeyOpRotate                          // Rotate group key
	KeyOpMemberJoin                      // Handle new member joining
	KeyOpMemberLeave                     // Handle member leaving (future)
)

// KeyOperation represents a key exchange operation
type KeyOperation struct {
	Type     KeyOperationType
	RoomID   string
	Username string
	Message  *models.Message // For request/response operations
	Data     interface{}     // Additional data for specific operations
}

// LocalChatService implements the ChatService interface
type LocalChatService struct {
	config         interfaces.Configuration
	logger         *logger.Logger
	discovery      interfaces.DiscoveryService
	transport      interfaces.MessageTransport
	eventHandler   interfaces.ChatEventHandler
	currentRoom    string
	username       string
	messages       []*models.Message
	peers          *models.PeerList
	userRegistry   *UserRegistry
	registered     bool
	ctx            context.Context
	cancel         context.CancelFunc
	isRunning      bool
	mutex          sync.RWMutex
	heartbeatTimer *time.Timer
	heartbeatDone  chan struct{}
	
	// Channels for message handling
	incomingMessages chan *models.Message
	outgoingMessages chan *models.Message
	uiUpdates        chan struct{}
	
	// Message deduplication with ordered sequence and pluggable writer
	messageCache     *MessageCache         // Ordered cache with max limit
	messageCacheSize int                   // Max cache size
	
	// Cryptographic components
	userPrivateKey   []byte                           // User's X25519 private key
	userPublicKey    []byte                           // User's X25519 public key
	groupCrypto      map[string]*crypto.GroupCrypto   // Room ID -> Group crypto manager
	peerPublicKeys   map[string][]byte                // Username -> Public key mapping
	cryptoMutex      sync.RWMutex                     // Mutex for crypto operations
	encryptionEnabled bool                            // Whether encryption is enabled
	
	// Key exchange channel for dedicated key operations
	keyOperations    chan KeyOperation                // Channel for key exchange operations
}

// NewLocalChatService creates a new local chat service
func NewLocalChatService(
	config interfaces.Configuration,
	logger *logger.Logger,
	discovery interfaces.DiscoveryService,
	transport interfaces.MessageTransport,
) interfaces.ChatService {
	return &LocalChatService{
		config:         config,
		logger:         logger.WithComponent("chat-service"),
		discovery:      discovery,
		transport:      transport,
		currentRoom:    "", // Start without a room
		username:       config.GetUsername(),
		messages:       make([]*models.Message, 0),
		peers:          models.NewPeerList(),
		userRegistry:   NewUserRegistry(),
		heartbeatDone:  make(chan struct{}),
		
		// Initialize channels with appropriate buffer sizes
		incomingMessages: make(chan *models.Message, 100), // Buffer for incoming messages
		outgoingMessages: make(chan *models.Message, 100), // Buffer for outgoing messages
		uiUpdates:       make(chan struct{}, 10),          // Buffer for UI update signals
		
		// Initialize message cache with default writer (oblivion)
		messageCache:     NewMessageCache(1000, nil), // Max 1000 messages, default to /dev/null
		messageCacheSize: 1000,
		
		// Initialize crypto components
		groupCrypto:       make(map[string]*crypto.GroupCrypto),
		peerPublicKeys:    make(map[string][]byte),
		encryptionEnabled: true, // Enable encryption by default
		keyOperations:     make(chan KeyOperation, 100), // Buffer for key operations
	}
}

// Start initializes and starts the chat service
func (s *LocalChatService) Start(ctx context.Context) error {
	s.logger.Info("Starting chat service")
	
	s.ctx, s.cancel = context.WithCancel(ctx)
	
	// Initialize user cryptographic keys
	if err := s.initializeCrypto(); err != nil {
		return fmt.Errorf("failed to initialize crypto: %w", err)
	}
	
	// Set up event handlers
	s.setupEventHandlers()
	
	// Start transport service first to get the actual port
	if err := s.transport.Start(s.ctx); err != nil {
		return fmt.Errorf("failed to start transport: %w", err)
	}
	
    // Get actual port from transport and pass it to discovery before starting it
    if addr := s.transport.GetListenAddress(); addr != nil {
        if tcpAddr, ok := addr.(*net.TCPAddr); ok {
            s.discovery.SetListenPort(tcpAddr.Port)
            // Also update local peer if already present
            if localPeer := s.discovery.GetLocalPeer(); localPeer != nil {
                localPeer.Port = tcpAddr.Port
            }
            s.logger.Info("Updated discovery with actual port", "port", tcpAddr.Port)
        }
    }
	
	// Start discovery service with actual port
	if err := s.discovery.Start(s.ctx); err != nil {
		return fmt.Errorf("failed to start discovery: %w", err)
	}

    // Auto-register if username is provided in config (e.g., from CLI flags)
    if s.username != "" && !s.registered {
        s.logger.Info("Auto-registering with username from config", "username", s.username)
        if err := s.RegisterUser(s.username); err != nil {
            s.logger.Warn("Auto registration failed", "error", err)
            s.logger.Info("Chat service ready - use /register <username> to register manually")
        } else {
            s.logger.Info("Auto-registered successfully", "username", s.username)
            
            // Auto-connect to room if specified in config
            if room := s.config.GetRoom(); room != "" {
                s.logger.Info("Auto-connecting to room from config", "room", room)
                if err := s.ConnectRoom(room); err != nil {
                    s.logger.Warn("Auto connect to room failed", "room", room, "error", err)
                    s.logger.Info("Use /connect <room> to join a room manually")
                } else {
                    s.logger.Info("Auto-connected to room successfully", "room", room)
                }
            } else {
                s.logger.Info("Chat service ready - use /connect <room> to join a room")
            }
        }
    } else {
        s.logger.Info("Chat service ready - use /register <username> and /connect <room> to get started")
    }
	
	// Start message processing goroutines
	go s.processIncomingMessages()
	go s.processOutgoingMessages()
	go s.processKeyOperations() // Dedicated key exchange processor
	go s.processUIUpdates()
	
	// Start heartbeat
	s.startHeartbeat()
	
	// Send join message
	s.sendJoinMessage()
	
	s.isRunning = true
	s.logger.Info("Chat service started successfully")
	return nil
}

// Stop gracefully shuts down the chat service
func (s *LocalChatService) Stop() error {
	s.logger.Info("Stopping chat service")
	
	// Send leave message
	s.sendLeaveMessage()
	
	// Stop heartbeat
	if s.heartbeatTimer != nil {
		s.heartbeatTimer.Stop()
	}
	close(s.heartbeatDone)
	
	// Stop services
	if s.cancel != nil {
		s.cancel()
	}
	
	if s.transport != nil {
		s.transport.Stop()
	}
	
	if s.discovery != nil {
		s.discovery.Stop()
	}
	
	s.isRunning = false
	s.logger.Info("Chat service stopped")
	return nil
}

// SendMessage sends a chat message
func (s *LocalChatService) SendMessage(content, recipient string) error {
	if !s.isRunning {
		return fmt.Errorf("chat service is not running")
	}
	
	if !s.registered {
		return fmt.Errorf("you must register first with /register <username>")
	}
	
	if s.currentRoom == "" {
		return fmt.Errorf("you must join a room first with /connect <room>")
	}
	
	// Handle @username mentions
	if strings.HasPrefix(content, "@") {
		parts := strings.SplitN(content, " ", 2)
		if len(parts) >= 2 {
			recipient = strings.TrimPrefix(parts[0], "@")
			content = parts[1]
		}
	}
	
	// Create the message to send (potentially encrypted)
	var messageToSend *models.Message
	var localMessage *models.Message
	
	// Check if encryption is enabled and we have group crypto for this room
	groupCrypto := s.getGroupCrypto(s.currentRoom)
	if s.encryptionEnabled && groupCrypto != nil {
		s.logger.Info("CRYPTO: Encrypting message", "room", s.currentRoom, "key_version", groupCrypto.KeyVersion)
		
		// Create original message for local storage (unencrypted)
		localMessage = models.NewChatMessage(s.username, recipient, content, s.currentRoom)
		
		// Encrypt the message content
		encryptedMsg, err := groupCrypto.EncryptMessage(s.username, content, localMessage.MessageID)
		if err != nil {
			s.logger.Error("CRYPTO: Failed to encrypt message", "error", err, "message_id", localMessage.MessageID)
			// Fall back to unencrypted message
			messageToSend = localMessage
		} else {
			// Create encrypted message for transmission
			messageToSend = &models.Message{
				MessageID: localMessage.MessageID,
				From:      s.username,
				To:        recipient,
				Content:   fmt.Sprintf("🔒[ENCRYPTED:%s:%s:%s:%d]", 
					crypto.ToBase64(encryptedMsg.EncryptedContent),
					crypto.ToBase64(encryptedMsg.WrappedKey),
					crypto.ToBase64(encryptedMsg.Nonce),
					encryptedMsg.KeyVersion),
				Room:      s.currentRoom,
				Type:      models.MessageTypeChat,
				Timestamp: localMessage.Timestamp,
			}
			
			s.logger.Info("CRYPTO: Message encrypted successfully", 
				"message_id", localMessage.MessageID,
				"key_version", encryptedMsg.KeyVersion,
				"encrypted_size", len(encryptedMsg.EncryptedContent))
		}
	} else {
		// No encryption - use plain message
		localMessage = models.NewChatMessage(s.username, recipient, content, s.currentRoom)
		messageToSend = localMessage
		
		if s.encryptionEnabled {
			s.logger.Warn("CRYPTO: Encryption enabled but no group crypto for room", "room", s.currentRoom)
		}
	}
	
	s.logger.Info("=== SENDING MESSAGE DEBUG ===", 
		"message_id", localMessage.MessageID,
		"from", s.username, 
		"to", recipient, 
		"content", content, 
		"room", s.currentRoom,
		"type", localMessage.Type,
		"encrypted", messageToSend.Content != content)
	
	// Add ORIGINAL (unencrypted) message to local list immediately for sender
	// Since sender won't receive their own message via TCP broadcast
	s.mutex.Lock()
	s.messages = append(s.messages, localMessage)
	s.mutex.Unlock()
	
	// Log encryption status for user awareness
	if messageToSend.Content != content {
		s.logger.Info("SEND: Message encrypted and queued", "message_id", localMessage.MessageID, "encrypted", true)
	} else {
		s.logger.Warn("SEND: Message sent UNENCRYPTED", "message_id", localMessage.MessageID, "encrypted", false)
	}
	
	s.logger.Info("SEND: Added message to local list for sender", "message_id", localMessage.MessageID)
	
	// Queue ENCRYPTED message for sending (non-blocking)
	select {
	case s.outgoingMessages <- messageToSend:
		s.logger.Info("SEND: Message queued successfully", "message_id", messageToSend.MessageID, "encrypted", messageToSend.Content != content)
	default:
		// Channel full, this is an error condition
		return fmt.Errorf("message queue is full, try again later")
	}
	
	// Signal UI update for immediate display
	select {
	case s.uiUpdates <- struct{}{}:
		s.logger.Info("SEND: UI update signaled", "message_id", localMessage.MessageID)
	default:
		// Channel full, UI will update on next cycle
	}
	
	return nil
}

// RegisterUser registers the current user with a username
func (s *LocalChatService) RegisterUser(username string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Check if already registered
	if s.registered {
		return fmt.Errorf("already registered as %s", s.username)
	}

	// Register with user registry
	localPeer := s.discovery.GetLocalPeer()
	if localPeer == nil {
		return fmt.Errorf("failed to get local peer information")
	}

	// Update username
	oldUsername := s.username
	s.username = username
	localPeer.Username = username

	// Register user
	err := s.userRegistry.RegisterUser(username, localPeer)
	if err != nil {
		// Revert username change
		s.username = oldUsername
		localPeer.Username = oldUsername
		return err
	}

	s.registered = true
	s.logger.Info("User registered", "username", username)

	// Send registration message
	regMsg := models.NewRegisterMessage(username, "")
	s.messages = append(s.messages, regMsg)

	if s.eventHandler != nil {
		s.eventHandler.OnMessageReceived(regMsg)
	}

	return nil
}

// ConnectRoom connects to a chat room
func (s *LocalChatService) ConnectRoom(roomName string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.registered {
		return fmt.Errorf("you must register first with /register <username>")
	}

	oldRoom := s.currentRoom
	s.currentRoom = roomName
	
	s.logger.Info("Connected to room", "room", roomName, "old_room", oldRoom)

	// Initialize group crypto for this room
	if err := s.initializeGroupCrypto(roomName); err != nil {
		s.logger.Warn("Failed to initialize group crypto", "room", roomName, "error", err)
		// Continue without encryption if crypto fails
	}
	
	// Request group key from existing members if encryption is enabled
	if s.encryptionEnabled && s.userPublicKey != nil {
		// Queue key request operation
		select {
		case s.keyOperations <- KeyOperation{
			Type:   KeyOpRequest,
			RoomID: roomName,
		}:
		default:
			s.logger.Warn("KEY_OP: Key operations channel full, requesting key directly")
			s.requestGroupKey(roomName)
		}
	}
	
	// Leave old room if in one
	if oldRoom != "" {
		s.userRegistry.LeaveRoom(s.username, oldRoom)
		leaveMsg := models.NewLeaveMessage(s.username, oldRoom)
		s.messages = append(s.messages, leaveMsg)
		s.transport.BroadcastMessage(leaveMsg)
	}

	// Join new room
	err := s.userRegistry.JoinRoom(s.username, roomName)
	if err != nil {
		return err
	}

	s.currentRoom = roomName
	s.logger.Info("Connected to room", "room", roomName, "old_room", oldRoom)

	// Connect to existing peers in this room
	discoveredPeers := s.discovery.GetPeers()
	for _, peer := range discoveredPeers {
		if peer.Room == roomName {
			s.logger.Info("Connecting to existing peer in room", "peer", peer.ID, "room", roomName)
			s.transport.ConnectToPeer(peer)
		}
	}

	// Send join message
	joinMsg := models.NewJoinMessage(s.username, roomName)
	s.messages = append(s.messages, joinMsg)
	s.transport.BroadcastMessage(joinMsg)

	// Send presence update
	users := s.userRegistry.GetUsersInRoom(roomName)
	presenceMsg := models.NewPresenceMessage(s.username, roomName, users)
	s.transport.BroadcastMessage(presenceMsg)

	if s.eventHandler != nil {
		s.eventHandler.OnRoomChanged(roomName)
	}

	return nil
}

// GetUsersInCurrentRoom returns all users in the current room
func (s *LocalChatService) GetUsersInCurrentRoom() []string {
	if s.currentRoom == "" {
		return []string{}
	}
	return s.userRegistry.GetUsersInRoom(s.currentRoom)
}

// GetAllRooms returns all available rooms
func (s *LocalChatService) GetAllRooms() []string {
	return s.userRegistry.GetAllRooms()
}

// IsUsernameAvailable checks if a username is available
func (s *LocalChatService) IsUsernameAvailable(username string) bool {
	return !s.userRegistry.IsUsernameTaken(username)
}

// LeaveRoom leaves the current room
func (s *LocalChatService) LeaveRoom() error {
	if s.currentRoom == "" {
		return nil
	}
	
	leaveMsg := models.NewLeaveMessage(s.username, s.currentRoom)
	s.transport.BroadcastMessage(leaveMsg)
	
	oldRoom := s.currentRoom
	s.currentRoom = ""
	
	s.logger.Info("Left room", "room", oldRoom)
	
	if s.eventHandler != nil {
		s.eventHandler.OnRoomChanged("")
	}
	
	return nil
}

// GetMessages returns the message history
func (s *LocalChatService) GetMessages() []*models.Message {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	// Return a copy of the messages
	messages := make([]*models.Message, len(s.messages))
	copy(messages, s.messages)
	return messages
}

// GetPeers returns the current peer list (connected peers in the same room)
func (s *LocalChatService) GetPeers() []*models.Peer {
	// Get all discovered peers
	discoveredPeers := s.discovery.GetPeers()
	// Get connected peers from transport
	connectedPeers := s.transport.GetConnectedPeers()
	
	// Create a map for quick lookup of connected peer IDs
	connectedMap := make(map[string]*models.Peer)
	for _, peer := range connectedPeers {
		connectedMap[peer.ID] = peer
	}
	
	// Filter for peers that are both discovered and connected, and in the same room
	var activePeers []*models.Peer
	for _, peer := range discoveredPeers {
		if connectedPeer, isConnected := connectedMap[peer.ID]; isConnected {
			// Only include peers in the same room (or if we're not in a room yet)
			if s.currentRoom == "" || peer.Room == s.currentRoom {
				// Use the connected peer info which has the most up-to-date status
				activePeers = append(activePeers, connectedPeer)
			}
		}
	}
	
	s.logger.Debug("Getting peers", "discovered", len(discoveredPeers), "connected", len(connectedPeers), "active", len(activePeers), "current_room", s.currentRoom)
	return activePeers
}

// GetCurrentRoom returns the current room name
func (s *LocalChatService) GetCurrentRoom() string {
	return s.currentRoom
}

// GetUsername returns the current username
func (s *LocalChatService) GetUsername() string {
	return s.username
}

// SetEventHandler sets the event handler for UI updates
func (s *LocalChatService) SetEventHandler(handler interfaces.ChatEventHandler) {
	s.eventHandler = handler
}

// SetMessageWriter sets a custom message writer for evicted messages
func (s *LocalChatService) SetMessageWriter(writer MessageWriter) {
	if s.messageCache != nil {
		s.messageCache.Close() // Close existing cache and writer
	}
	s.messageCache = NewMessageCache(s.messageCacheSize, writer)
}

// initializeCrypto generates user key pair and sets up crypto components
func (s *LocalChatService) initializeCrypto() error {
	if !s.encryptionEnabled {
		s.logger.Info("Encryption disabled, skipping crypto initialization")
		return nil
	}
	
	// Generate user key pair
	privateKey, publicKey, err := crypto.GenerateKeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate user key pair: %w", err)
	}
	
	s.userPrivateKey = privateKey
	s.userPublicKey = publicKey
	
	s.logger.Info("Crypto initialized", 
		"public_key_b64", crypto.ToBase64(publicKey),
		"encryption_enabled", s.encryptionEnabled)
	
	return nil
}

// EnableEncryption enables or disables encryption for the chat service
func (s *LocalChatService) EnableEncryption(enabled bool) {
	s.cryptoMutex.Lock()
	defer s.cryptoMutex.Unlock()
	
	s.encryptionEnabled = enabled
	s.logger.Info("Encryption setting changed", "enabled", enabled)
}

// GetPublicKey returns the user's public key for key exchange
func (s *LocalChatService) GetPublicKey() []byte {
	s.cryptoMutex.RLock()
	defer s.cryptoMutex.RUnlock()
	
	if s.userPublicKey == nil {
		return nil
	}
	
	// Return a copy to prevent modification
	publicKey := make([]byte, len(s.userPublicKey))
	copy(publicKey, s.userPublicKey)
	return publicKey
}

// initializeGroupCrypto initializes group crypto for a room
func (s *LocalChatService) initializeGroupCrypto(roomID string) error {
	if !s.encryptionEnabled {
		s.logger.Debug("Encryption disabled, skipping group crypto initialization", "room", roomID)
		return nil
	}
	
	s.cryptoMutex.Lock()
	defer s.cryptoMutex.Unlock()
	
	// Check if group crypto already exists for this room
	if _, exists := s.groupCrypto[roomID]; exists {
		s.logger.Debug("Group crypto already initialized for room", "room", roomID)
		return nil
	}
	
	// Create new group crypto manager (but don't generate key yet)
	groupCrypto := crypto.NewGroupCrypto(roomID)
	
	// Add ourselves as a member
	groupCrypto.AddMember(s.username, s.userPublicKey)
	
	// Store group crypto (without a group key initially)
	s.groupCrypto[roomID] = groupCrypto
	
	s.logger.Info("Group crypto initialized (no key yet)", 
		"room", roomID, 
		"members", len(groupCrypto.Members))
	
	// We'll generate a key only if no one responds to our key request
	// The key request/response mechanism will handle key distribution
	
	return nil
}

// getGroupCrypto safely retrieves group crypto for a room
func (s *LocalChatService) getGroupCrypto(roomID string) *crypto.GroupCrypto {
	s.cryptoMutex.RLock()
	defer s.cryptoMutex.RUnlock()
	
	return s.groupCrypto[roomID]
}

// requestGroupKey sends a key request to existing room members
func (s *LocalChatService) requestGroupKey(roomID string) {
    if !s.encryptionEnabled || s.userPublicKey == nil {
        return
    }
	
	// Send key request with our public key
	publicKeyB64 := crypto.ToBase64(s.userPublicKey)
	keyRequest := models.NewKeyRequestMessage(s.username, roomID, publicKeyB64)
	
	s.logger.Info("CRYPTO: Requesting group key for room", 
		"room", roomID, 
		"public_key_b64", publicKeyB64[:16]+"...")
	
	// Broadcast key request to room members
	    if err := s.transport.BroadcastMessage(keyRequest); err != nil {
        s.logger.Error("CRYPTO: Failed to send key request", "error", err, "room", roomID)
        return
    }
    
    // Wait a bit for responses, re-broadcast once, then generate our own key if no one responds
    go func() {
        // Informatively log that we are waiting before fallback
        s.logger.Info("CRYPTO: Waiting for key responses before fallback", "room", roomID, "timeout", "2s")
        // Re-broadcast once after ~700ms in case peers connected slightly later
        time.Sleep(700 * time.Millisecond)
        if gc := s.getGroupCrypto(roomID); gc != nil && gc.GroupKey == nil {
            s.logger.Info("CRYPTO: Re-sending key request to peers", "room", roomID)
            _ = s.transport.BroadcastMessage(keyRequest)
        }
        // Wait remaining time before fallback
        time.Sleep(1300 * time.Millisecond)
        
        groupCrypto := s.getGroupCrypto(roomID)
        if groupCrypto != nil && groupCrypto.GroupKey == nil {
            s.logger.Info("CRYPTO: No key response received, generating our own group key", "room", roomID)
            
            if err := groupCrypto.GenerateGroupKey(); err != nil {
                s.logger.Error("CRYPTO: Failed to generate fallback group key", "error", err, "room", roomID)
            } else {
                s.logger.Info("CRYPTO: Generated fallback group key", 
                    "room", roomID, 
                    "key_version", groupCrypto.KeyVersion)
            }
        }
    }()
}

// handleKeyRequest processes a key request from a new room member
func (s *LocalChatService) handleKeyRequest(message *models.Message) {
	if !s.encryptionEnabled {
		s.logger.Debug("CRYPTO: Ignoring key request - encryption disabled")
		return
	}
	
	// Don't respond to our own key requests
	if message.From == s.username {
		return
	}
	
	s.logger.Info("CRYPTO: Received key request", 
		"from", message.From, 
		"room", message.Room)
	
	// Get group crypto for this room
	groupCrypto := s.getGroupCrypto(message.Room)
	if groupCrypto == nil {
		s.logger.Warn("CRYPTO: No group crypto for room, cannot respond to key request", "room", message.Room)
		return
	}
	
	// Only respond if we actually have a group key
	if groupCrypto.GroupKey == nil {
		s.logger.Info("CRYPTO: We don't have a group key yet, cannot respond to key request", 
			"room", message.Room, "from", message.From)
		return
	}
	
	// Decode requester's public key
	requesterPublicKey, err := crypto.FromBase64(message.Content)
	if err != nil {
		s.logger.Error("CRYPTO: Failed to decode requester public key", "error", err, "from", message.From)
		return
	}
	
	// Store the requester's public key for future use
	s.storePeerPublicKey(message.From, requesterPublicKey)
	
	// Create key exchange message for the requester
	keyExchangeMsg, err := groupCrypto.CreateKeyExchangeMessage(message.From, requesterPublicKey, s.userPrivateKey)
	if err != nil {
		s.logger.Error("CRYPTO: Failed to create key exchange message", "error", err, "from", message.From)
		return
	}
	
	// Send encrypted group key to requester
	// Include key version and our public key in the message content: "encrypted_key:version:sender_public_key"
	encryptedGroupKeyB64 := crypto.ToBase64(keyExchangeMsg.EncryptedGroupKey)
	senderPublicKeyB64 := crypto.ToBase64(s.userPublicKey)
	keyResponseContent := fmt.Sprintf("%s:%d:%s", encryptedGroupKeyB64, keyExchangeMsg.KeyVersion, senderPublicKeyB64)
	keyResponse := models.NewKeyResponseMessage(s.username, message.From, message.Room, keyResponseContent)
	
	s.logger.Info("CRYPTO: Sending group key to new member", 
		"to", message.From, 
		"room", message.Room,
		"key_version", keyExchangeMsg.KeyVersion)
	
	if err := s.transport.BroadcastMessage(keyResponse); err != nil {
		s.logger.Error("CRYPTO: Failed to send key response", "error", err, "to", message.From)
		return
	}
	
	// Add new member to our group crypto
	groupCrypto.AddMember(message.From, requesterPublicKey)
	
	s.logger.Info("CRYPTO: Added new member to group", 
		"member", message.From, 
		"room", message.Room,
		"total_members", len(groupCrypto.Members))
}

// handleKeyResponse processes a key response containing the group key
func (s *LocalChatService) handleKeyResponse(message *models.Message) {
	if !s.encryptionEnabled {
		s.logger.Debug("CRYPTO: Ignoring key response - encryption disabled")
		return
	}
	
	// Only process key responses meant for us
	if message.To != s.username {
		return
	}
	
	s.logger.Info("CRYPTO: Received key response", 
		"from", message.From, 
		"room", message.Room)
	
	// Get group crypto for this room
	groupCrypto := s.getGroupCrypto(message.Room)
	if groupCrypto == nil {
		s.logger.Error("CRYPTO: No group crypto for room", "room", message.Room)
		return
	}
	
	// Parse message content: "encrypted_key:version:sender_public_key"
	parts := strings.Split(message.Content, ":")
	if len(parts) != 3 {
		s.logger.Error("CRYPTO: Invalid key response format", "content", message.Content, "from", message.From)
		return
	}
	
	// Decode encrypted group key
	encryptedGroupKey, err := crypto.FromBase64(parts[0])
	if err != nil {
		s.logger.Error("CRYPTO: Failed to decode encrypted group key", "error", err, "from", message.From)
		return
	}
	
	// Parse key version
	var keyVersion int
	if _, err := fmt.Sscanf(parts[1], "%d", &keyVersion); err != nil {
		s.logger.Error("CRYPTO: Failed to parse key version", "error", err, "version_str", parts[1])
		return
	}
	
	// Decode sender's public key
	senderPublicKey, err := crypto.FromBase64(parts[2])
	if err != nil {
		s.logger.Error("CRYPTO: Failed to decode sender public key", "error", err, "from", message.From)
		return
	}
	
	// Store sender's public key
	s.storePeerPublicKey(message.From, senderPublicKey)
	
	// Create key exchange message structure
	keyExchangeMsg := &crypto.KeyExchangeMessage{
		Type:              "group_key_distribution",
		RoomID:            message.Room,
		EncryptedGroupKey: encryptedGroupKey,
		KeyVersion:        keyVersion, // Use the version from the sender
		Recipient:         s.username,
	}
	
	// Process the key exchange message
	if err := groupCrypto.ProcessKeyExchangeMessage(keyExchangeMsg, s.userPrivateKey, senderPublicKey); err != nil {
		s.logger.Error("CRYPTO: Failed to process key exchange", "error", err, "from", message.From)
		return
	}
	
	// Add sender to our group members
	groupCrypto.AddMember(message.From, senderPublicKey)
	
	s.logger.Info("CRYPTO: Successfully received group key", 
		"from", message.From, 
		"room", message.Room,
		"key_version", groupCrypto.KeyVersion,
		"total_members", len(groupCrypto.Members))
}

// getPeerPublicKey retrieves a peer's public key
func (s *LocalChatService) getPeerPublicKey(username string) []byte {
	s.cryptoMutex.RLock()
	defer s.cryptoMutex.RUnlock()
	
	publicKey, exists := s.peerPublicKeys[username]
	if !exists {
		return nil
	}
	
	// Return a copy to prevent modification
	result := make([]byte, len(publicKey))
	copy(result, publicKey)
	return result
}

// storePeerPublicKey stores a peer's public key
func (s *LocalChatService) storePeerPublicKey(username string, publicKey []byte) {
	s.cryptoMutex.Lock()
	defer s.cryptoMutex.Unlock()
	
	// Store a copy to prevent external modification
	keyData := make([]byte, len(publicKey))
	copy(keyData, publicKey)
	s.peerPublicKeys[username] = keyData
	
	s.logger.Info("CRYPTO: Stored public key for peer", 
		"username", username, 
		"key_b64", crypto.ToBase64(publicKey)[:16]+"...")
}

// decryptIncomingMessage decrypts an incoming message if it's encrypted
func (s *LocalChatService) decryptIncomingMessage(message *models.Message) *models.Message {
	// Check if message is encrypted (starts with 🔒[ENCRYPTED:)
	if !strings.HasPrefix(message.Content, "🔒[ENCRYPTED:") {
		// Not encrypted, return as-is
		return message
	}
	
	s.logger.Info("CRYPTO: Received encrypted message", "from", message.From, "message_id", message.MessageID)
	
	// Parse encrypted message format: 🔒[ENCRYPTED:base64_content:base64_key:base64_nonce:version]
	content := message.Content
	if !strings.HasSuffix(content, "]") {
		s.logger.Error("CRYPTO: Invalid encrypted message format", "content", content)
		return message
	}
	
	// Remove prefix and suffix
	encryptedPart := strings.TrimPrefix(content, "🔒[ENCRYPTED:")
	encryptedPart = strings.TrimSuffix(encryptedPart, "]")
	
	// Split into components
	parts := strings.Split(encryptedPart, ":")
	if len(parts) != 4 {
		s.logger.Error("CRYPTO: Invalid encrypted message parts", "parts", len(parts), "content", content)
		return message
	}
	
	// Decode components
	encryptedContent, err := crypto.FromBase64(parts[0])
	if err != nil {
		s.logger.Error("CRYPTO: Failed to decode encrypted content", "error", err)
		return message
	}
	
	wrappedKey, err := crypto.FromBase64(parts[1])
	if err != nil {
		s.logger.Error("CRYPTO: Failed to decode wrapped key", "error", err)
		return message
	}
	
	nonce, err := crypto.FromBase64(parts[2])
	if err != nil {
		s.logger.Error("CRYPTO: Failed to decode nonce", "error", err)
		return message
	}
	
	keyVersion := 0
	if _, err := fmt.Sscanf(parts[3], "%d", &keyVersion); err != nil {
		s.logger.Error("CRYPTO: Failed to parse key version", "error", err)
		return message
	}
	
	// Get group crypto for this room
	groupCrypto := s.getGroupCrypto(message.Room)
	if groupCrypto == nil {
		s.logger.Error("CRYPTO: No group crypto for room", "room", message.Room)
		return message
	}
	
	// Create encrypted message object for decryption
	encryptedMsg := &crypto.EncryptedMessage{
		Sender:           message.From,
		Room:             message.Room,
		EncryptedContent: encryptedContent,
		WrappedKey:       wrappedKey,
		KeyVersion:       keyVersion,
		MessageID:        message.MessageID,
		Timestamp:        message.Timestamp,
		Nonce:            nonce,
	}
	
	// Decrypt the message
	plaintext, err := groupCrypto.DecryptMessage(encryptedMsg)
	if err != nil {
		s.logger.Error("CRYPTO: Failed to decrypt message", "error", err, "from", message.From, "key_version", keyVersion)
		// Return original message with error indicator
		errorMessage := *message
		errorMessage.Content = fmt.Sprintf("🔒❌ [DECRYPTION FAILED: %s]", err.Error())
		return &errorMessage
	}
	
	s.logger.Info("CRYPTO: Message decrypted successfully", 
		"from", message.From, 
		"message_id", message.MessageID,
		"key_version", keyVersion,
		"plaintext_length", len(plaintext))
	
	// Return decrypted message
	decryptedMessage := *message
	decryptedMessage.Content = plaintext
	return &decryptedMessage
}

// setupEventHandlers sets up the event handlers for discovery and transport
func (s *LocalChatService) setupEventHandlers() {
	// Discovery event handlers
	s.discovery.SetPeerHandler(&discoveryEventHandler{service: s})
	
	// Transport event handlers
	s.transport.SetMessageHandler(&transportEventHandler{service: s})
}

// sendJoinMessage sends a join message to the current room
func (s *LocalChatService) sendJoinMessage() {
	if s.currentRoom == "" {
		return
	}
	
	joinMsg := models.NewJoinMessage(s.username, s.currentRoom)
	s.mutex.Lock()
	s.messages = append(s.messages, joinMsg)
	s.mutex.Unlock()
	
	s.transport.BroadcastMessage(joinMsg)
}

// sendLeaveMessage sends a leave message to the current room
func (s *LocalChatService) sendLeaveMessage() {
	if s.currentRoom == "" {
		return
	}
	
	leaveMsg := models.NewLeaveMessage(s.username, s.currentRoom)
	s.transport.BroadcastMessage(leaveMsg)
}

// processIncomingMessages handles incoming messages from the channel
func (s *LocalChatService) processIncomingMessages() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case message := <-s.incomingMessages:
			s.handleIncomingMessage(message)
		}
	}
}

// processOutgoingMessages handles outgoing messages with batching
func (s *LocalChatService) processOutgoingMessages() {
	ticker := time.NewTicker(10 * time.Millisecond) // Small delay for batching
	defer ticker.Stop()
	
	var batch []*models.Message
	
	for {
		select {
		case <-s.ctx.Done():
			// Send any remaining messages before shutdown
			if len(batch) > 0 {
				s.sendMessageBatch(batch)
			}
			return
			
		case message := <-s.outgoingMessages:
			batch = append(batch, message)
			
		case <-ticker.C:
			if len(batch) > 0 {
				s.sendMessageBatch(batch)
				batch = batch[:0] // Reset slice but keep capacity
			}
		}
	}
}

// processUIUpdates handles UI update notifications with debouncing
func (s *LocalChatService) processUIUpdates() {
	ticker := time.NewTicker(50 * time.Millisecond) // Debounce UI updates
	defer ticker.Stop()
	
	needsUpdate := false
	
	for {
		select {
		case <-s.ctx.Done():
			return
			
		case <-s.uiUpdates:
			needsUpdate = true
			
		case <-ticker.C:
			if needsUpdate && s.eventHandler != nil {
				// Trigger UI refresh
				s.eventHandler.OnMessageReceived(nil) // nil indicates general refresh
				needsUpdate = false
			}
		}
	}
}

// processKeyOperations handles key exchange operations in a dedicated goroutine
func (s *LocalChatService) processKeyOperations() {
	for {
		select {
		case keyOp := <-s.keyOperations:
			s.handleKeyOperation(keyOp)
		case <-s.ctx.Done():
			return
		}
	}
}

// handleKeyOperation processes different types of key operations
func (s *LocalChatService) handleKeyOperation(keyOp KeyOperation) {
	switch keyOp.Type {
	case KeyOpRequest:
		s.logger.Info("KEY_OP: Processing key request", "room", keyOp.RoomID, "from", keyOp.Username)
		s.requestGroupKey(keyOp.RoomID)
		
	case KeyOpResponse:
		s.logger.Info("KEY_OP: Processing key response", "room", keyOp.RoomID, "from", keyOp.Username)
		if keyOp.Message != nil {
			s.handleKeyRequest(keyOp.Message)
		}
		
	case KeyOpMemberJoin:
		s.logger.Info("KEY_OP: Processing member join", "room", keyOp.RoomID, "member", keyOp.Username)
		// When someone joins our room, resend key request if we don't have a group key yet
		if keyOp.RoomID == s.currentRoom && s.encryptionEnabled {
			groupCrypto := s.getGroupCrypto(keyOp.RoomID)
			if groupCrypto != nil && groupCrypto.GroupKey == nil {
				s.logger.Info("KEY_OP: New user joined, requesting key", 
					"new_user", keyOp.Username, "room", keyOp.RoomID)
				
				// Small delay to ensure the join message is processed
				time.Sleep(100 * time.Millisecond)
				s.requestGroupKey(keyOp.RoomID)
			}
		}
		
	case KeyOpRotate:
		s.logger.Info("KEY_OP: Processing key rotation", "room", keyOp.RoomID)
		// TODO: Implement key rotation
		s.rotateGroupKey(keyOp.RoomID)
		
	case KeyOpMemberLeave:
		s.logger.Info("KEY_OP: Processing member leave", "room", keyOp.RoomID, "member", keyOp.Username)
		// TODO: Implement key rotation on member leave for forward secrecy
		
	default:
		s.logger.Warn("KEY_OP: Unknown key operation type", "type", keyOp.Type)
	}
}

// rotateGroupKey rotates the group key for a room (future implementation)
func (s *LocalChatService) rotateGroupKey(roomID string) {
	// TODO: Implement key rotation
	s.logger.Info("KEY_OP: Key rotation not yet implemented", "room", roomID)
}

// handleIncomingMessage processes a single incoming message
func (s *LocalChatService) handleIncomingMessage(message *models.Message) {
	s.logger.Info("=== INCOMING MESSAGE DEBUG ===", 
		"message_id", message.MessageID,
		"from", message.From, 
		"to", message.To,
		"content", message.Content,
		"room", message.Room,
		"type", message.Type,
		"our_username", s.username,
		"our_room", s.currentRoom)
	
	// First, do all the filtering BEFORE checking for duplicates
	// This ensures we only cache messages we actually care about
	
	// Skip messages from different rooms
	if message.Room != s.currentRoom && message.Room != "" {
		s.logger.Info("FILTER: Skipping message from different room", "message_room", message.Room, "current_room", s.currentRoom)
		return
	}
	
	// Skip private messages not for us (but allow our own private messages)
	if message.To != "" && message.To != "broadcast" && message.To != s.username {
		s.logger.Info("FILTER: Skipping private message not for us", "to", message.To, "username", s.username)
		return
	}
	
	// Skip heartbeats from self to avoid spam
	if message.Type == models.MessageTypeHeartbeat && message.From == s.username {
		s.logger.Debug("FILTER: Skipping heartbeat from self")
		return
	}
	
	s.logger.Info("FILTER: Message passed all filters, checking for duplicates")
	
	// NOW check for duplicates (only for messages we want to process)
	if s.messageCache.Contains(message.MessageID) {
		s.logger.Info("DUPLICATE: Skipping duplicate message", "message_id", message.MessageID, "from", message.From, "content", message.Content)
		return
	}
	
	s.logger.Info("CACHE: Adding message to cache", "message_id", message.MessageID)
	
	// Add to cache (this marks it as seen)
	if err := s.messageCache.Add(message); err != nil {
		s.logger.Error("CACHE: Failed to add message to cache", "error", err, "message_id", message.MessageID)
		return
	}
	
	s.logger.Info("Processing incoming message", "from", message.From, "type", message.Type, "content", message.Content, "message_id", message.MessageID)
	
	// Decrypt message if it's encrypted
	decryptedMessage := s.decryptIncomingMessage(message)
	
	// Handle special message types FIRST (before adding to history)
	switch message.Type {
	case models.MessageTypeJoin:
		s.logger.Info("User joined", "user", message.From, "room", message.Room)
		if message.From != s.username { // Don't register ourselves
			if peer, exists := s.peers.Get(message.From); exists {
				s.userRegistry.RegisterUser(message.From, peer)
				s.userRegistry.JoinRoom(message.From, message.Room)
			}
			
			// When someone joins our room, queue a member join key operation
			if message.Room == s.currentRoom && s.encryptionEnabled {
				select {
				case s.keyOperations <- KeyOperation{
					Type:     KeyOpMemberJoin,
					RoomID:   message.Room,
					Username: message.From,
				}:
				default:
					s.logger.Warn("KEY_OP: Key operations channel full, processing member join directly")
					// Fallback to direct processing
					groupCrypto := s.getGroupCrypto(message.Room)
					if groupCrypto != nil && groupCrypto.GroupKey == nil {
						s.logger.Info("CRYPTO: New user joined, resending key request", 
							"new_user", message.From, "room", message.Room)
						go func() {
							time.Sleep(100 * time.Millisecond)
							s.requestGroupKey(message.Room)
						}()
					}
				}
			}
		}
	case models.MessageTypeLeave:
		s.logger.Info("User left", "user", message.From, "room", message.Room)
		if message.From != s.username { // Don't unregister ourselves
			s.userRegistry.LeaveRoom(message.From, message.Room)
		}
	case models.MessageTypeKeyRequest:
		// Queue key request for processing
		select {
		case s.keyOperations <- KeyOperation{
			Type:     KeyOpResponse,
			RoomID:   message.Room,
			Username: message.From,
			Message:  message,
		}:
		default:
			s.logger.Warn("KEY_OP: Key operations channel full, processing key request directly")
			s.handleKeyRequest(message)
		}
		return // Don't add key requests to message history
	case models.MessageTypeKeyResponse:
		s.handleKeyResponse(message)
		return // Don't add key responses to message history
	case models.MessageTypeHeartbeat:
		// Update user registry with heartbeat info
		if peer, exists := s.peers.Get(message.From); exists {
			s.userRegistry.RegisterUser(message.From, peer)
			s.userRegistry.JoinRoom(message.From, message.Room)
			s.logger.Debug("HEARTBEAT: Updated user presence", "user", message.From, "room", message.Room)
		}
		return // Don't add heartbeats to message history
	}
	
	// Add decrypted message to message list (only for displayable messages)
	s.mutex.Lock()
	s.messages = append(s.messages, decryptedMessage)
	s.mutex.Unlock()
	
	// Signal UI update
	select {
	case s.uiUpdates <- struct{}{}:
	default:
		// Channel full, skip this update signal
	}
}

// sendMessageBatch sends a batch of messages
func (s *LocalChatService) sendMessageBatch(batch []*models.Message) {
	for _, message := range batch {
		if err := s.transport.BroadcastMessage(message); err != nil {
			s.logger.Error("Failed to send message", "error", err, "from", message.From, "content", message.Content)
		} else {
			s.logger.Debug("Message sent", "from", message.From, "type", message.Type, "content", message.Content)
		}
	}
}


// startHeartbeat starts the heartbeat mechanism
func (s *LocalChatService) startHeartbeat() {
	s.heartbeatDone = make(chan struct{})
	s.heartbeatTimer = time.NewTimer(30 * time.Second)
	
	go func() {
		defer s.heartbeatTimer.Stop()
		
		for {
			select {
			case <-s.heartbeatDone:
				s.logger.Debug("HEARTBEAT: Stopping heartbeat timer")
				return
				
			case <-s.heartbeatTimer.C:
				// Send heartbeat if we're in a room and registered
				if s.currentRoom != "" && s.registered {
					s.sendHeartbeat()
					s.logger.Debug("HEARTBEAT: Sent heartbeat", "room", s.currentRoom, "username", s.username)
				}
				// Reset timer for next heartbeat
				s.heartbeatTimer.Reset(30 * time.Second)
				
			case <-s.ctx.Done():
				return
			}
		}
	}()
	
	s.logger.Info("HEARTBEAT: Started heartbeat timer", "interval", "30s")
}

// sendHeartbeat sends a heartbeat message
func (s *LocalChatService) sendHeartbeat() {
	if s.currentRoom == "" || !s.registered {
		return
	}
	
	heartbeatMsg := models.NewHeartbeatMessage(s.username, s.currentRoom)
	s.transport.BroadcastMessage(heartbeatMsg)
}

// discoveryEventHandler handles discovery events
type discoveryEventHandler struct {
	service *LocalChatService
}

func (h *discoveryEventHandler) OnPeerDiscovered(peer *models.Peer) {
	h.service.logger.Info("Peer discovered", "peer", peer.ID, "username", peer.Username, "room", peer.Room)
	
	// Add peer to our peer list
	h.service.peers.Add(peer)
	
	// Auto-connect to peers in the same room (only if we're in a room)
	if h.service.currentRoom != "" && peer.Room == h.service.currentRoom {
		h.service.logger.Info("Auto-connecting to peer in same room", "peer", peer.ID, "room", peer.Room)
		h.service.transport.ConnectToPeer(peer)
	} else {
		h.service.logger.Info("Peer discovered but not auto-connecting", "peer_room", peer.Room, "our_room", h.service.currentRoom)
	}
}

func (h *discoveryEventHandler) OnPeerLost(peerID string) {
	h.service.logger.Info("Peer lost", "peer", peerID)
	h.service.transport.DisconnectFromPeer(peerID)
}

func (h *discoveryEventHandler) OnPeerConnected(peer *models.Peer) {
	h.service.logger.Info("Peer connected", "peer", peer.ID, "username", peer.Username)
	
	// Send current room info to new peer
	if h.service.currentRoom != "" {
		joinMsg := models.NewJoinMessage(h.service.username, h.service.currentRoom)
		h.service.transport.SendMessage(peer.ID, joinMsg)
	}
	
	if h.service.eventHandler != nil {
		h.service.eventHandler.OnPeerJoined(peer)
	}
}

func (h *discoveryEventHandler) OnPeerDisconnected(peerID string) {
	h.service.logger.Info("Peer disconnected", "peer", peerID)
	
	if h.service.eventHandler != nil {
		h.service.eventHandler.OnPeerLeft(peerID)
	}
}

// transportEventHandler handles transport events
type transportEventHandler struct {
	service *LocalChatService
}

func (h *transportEventHandler) OnMessageReceived(message *models.Message) {
	h.service.logger.Info("=== TRANSPORT RECEIVED ===", 
		"message_id", message.MessageID,
		"from", message.From, 
		"type", message.Type, 
		"content", message.Content, 
		"room", message.Room,
		"to", message.To)
	
	// Queue message for processing (non-blocking)
	select {
	case h.service.incomingMessages <- message:
		h.service.logger.Info("TRANSPORT: Message queued for processing", "message_id", message.MessageID)
	default:
		// Channel full, drop message (this shouldn't happen with proper buffer sizing)
		h.service.logger.Warn("TRANSPORT: Incoming message queue full, dropping message", "from", message.From, "type", message.Type)
	}
}

func (h *transportEventHandler) OnMessageSent(message *models.Message) {
	// Message sent successfully, no action needed
}

func (h *transportEventHandler) OnConnectionError(peerID string, err error) {
	h.service.logger.Error("Connection error", "peer", peerID, "error", err)
}
