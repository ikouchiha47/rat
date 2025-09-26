package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"rat/internal/interfaces"
	"rat/internal/logger"
	"rat/internal/models"
)

// TCPMessageTransport implements MessageTransport using TCP
type TCPMessageTransport struct {
	config        interfaces.Configuration
	logger        *logger.Logger
	listener      net.Listener
	connections   map[string]net.Conn
	messageHandler interfaces.MessageEventHandler
	peers         *models.PeerList
	ctx           context.Context
	cancel        context.CancelFunc
	isRunning     bool
	port          int
	mutex         sync.RWMutex
}

// NewTCPMessageTransport creates a new TCP message transport
func NewTCPMessageTransport(config interfaces.Configuration, logger *logger.Logger) interfaces.MessageTransport {
	return &TCPMessageTransport{
		config:      config,
		logger:      logger.WithComponent("tcp-transport"),
		connections: make(map[string]net.Conn),
		peers:       models.NewPeerList(),
	}
}

// Start begins listening for incoming connections
func (t *TCPMessageTransport) Start(ctx context.Context) error {
	t.logger.Info("Starting TCP message transport")
	
	t.ctx, t.cancel = context.WithCancel(ctx)
	
	// Start listening on the specified port
	var err error
	port := t.config.GetPort()
	if port == 0 {
		port = 0 // Let system choose available port
	}
	
	t.listener, err = net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to start listener: %w", err)
	}
	
	// Get the actual port we're listening on
	addr := t.listener.Addr().(*net.TCPAddr)
	t.port = addr.Port
	
	t.logger.Info("TCP transport started", "address", t.listener.Addr().String())
	
	// Start accepting connections
	go t.acceptConnections()
	
	t.isRunning = true
	return nil
}

// Stop stops the transport service
func (t *TCPMessageTransport) Stop() error {
	t.logger.Info("Stopping TCP message transport")
	
	if t.cancel != nil {
		t.cancel()
	}
	
	if t.listener != nil {
		t.listener.Close()
	}
	
	// Close all active connections
	t.mutex.Lock()
	for peerID, conn := range t.connections {
		conn.Close()
		delete(t.connections, peerID)
	}
	t.mutex.Unlock()
	
	t.isRunning = false
	t.logger.Info("TCP message transport stopped")
	return nil
}

// SendMessage sends a message to a specific peer
func (t *TCPMessageTransport) SendMessage(peerID string, message *models.Message) error {
	t.mutex.RLock()
	conn, exists := t.connections[peerID]
	t.mutex.RUnlock()
	
	if !exists {
		return fmt.Errorf("no connection to peer %s", peerID)
	}
	
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	
	// Add newline as message delimiter
	data = append(data, '\n')
	
	_, err = conn.Write(data)
	if err != nil {
		// Connection might be broken, remove it
		t.mutex.Lock()
		delete(t.connections, peerID)
		t.mutex.Unlock()
		
		if t.messageHandler != nil {
			t.messageHandler.OnConnectionError(peerID, err)
		}
		
		return fmt.Errorf("failed to send message to peer %s: %w", peerID, err)
	}
	
	if t.messageHandler != nil {
		t.messageHandler.OnMessageSent(message)
	}
	
	t.logger.Info("Message sent", "peer", peerID, "type", message.Type, "from", message.From, "to", message.To, "content", message.Content)
	return nil
}

// BroadcastMessage sends a message to all connected peers
func (t *TCPMessageTransport) BroadcastMessage(message *models.Message) error {
	t.mutex.RLock()
	peerIDs := make([]string, 0, len(t.connections))
	for peerID := range t.connections {
		peerIDs = append(peerIDs, peerID)
	}
	t.mutex.RUnlock()
	
	var errs []error
	for _, peerID := range peerIDs {
		if err := t.SendMessage(peerID, message); err != nil {
			errs = append(errs, err)
		}
	}
	
	if len(errs) > 0 {
		return fmt.Errorf("broadcast errors: %v", errs)
	}
	
	return nil
}

// SetMessageHandler sets the message event handler
func (t *TCPMessageTransport) SetMessageHandler(handler interfaces.MessageEventHandler) {
	t.messageHandler = handler
}

// ConnectToPeer establishes a connection to a peer
func (t *TCPMessageTransport) ConnectToPeer(peer *models.Peer) error {
	address := peer.GetAddress()
	
	t.mutex.RLock()
	_, exists := t.connections[peer.ID]
	t.mutex.RUnlock()
	
	if exists {
		return nil // Already connected
	}
	
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to peer %s at %s: %w", peer.ID, address, err)
	}
	
	t.mutex.Lock()
	t.connections[peer.ID] = conn
	peer.Connect()
	t.peers.Add(peer)
	t.mutex.Unlock()
	
	// Start handling messages from this connection
	go t.handleConnection(peer.ID, conn)
	
	t.logger.Info("Connected to peer", "peer", peer.ID, "address", address)
	return nil
}

// DisconnectFromPeer closes connection to a peer
func (t *TCPMessageTransport) DisconnectFromPeer(peerID string) error {
	t.mutex.Lock()
	conn, exists := t.connections[peerID]
	if exists {
		conn.Close()
		delete(t.connections, peerID)
		
		if peer, exists := t.peers.Get(peerID); exists {
			peer.Disconnect()
		}
	}
	t.mutex.Unlock()
	
	if exists {
		t.logger.Info("Disconnected from peer", "peer", peerID)
	}
	
	return nil
}

// GetConnectedPeers returns list of currently connected peers
func (t *TCPMessageTransport) GetConnectedPeers() []*models.Peer {
	return t.peers.GetConnected()
}

// GetListenAddress returns the address this transport is listening on
func (t *TCPMessageTransport) GetListenAddress() net.Addr {
	if t.listener != nil {
		return t.listener.Addr()
	}
	return nil
}

// acceptConnections handles incoming connections
func (t *TCPMessageTransport) acceptConnections() {
	for {
		select {
		case <-t.ctx.Done():
			return
		default:
			conn, err := t.listener.Accept()
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
					t.logger.Warn("Temporary error accepting connection", "error", err)
					continue
				}
				
				// Listener closed or other permanent error
				if t.isRunning {
					t.logger.Error("Error accepting connection", "error", err)
				}
				return
			}
			
			// Handle the new connection
			go t.handleIncomingConnection(conn)
		}
	}
}

// handleIncomingConnection handles an incoming connection
func (t *TCPMessageTransport) handleIncomingConnection(conn net.Conn) {
	peerAddr := conn.RemoteAddr().String()
	t.logger.Info("New incoming connection", "address", peerAddr)
	
	// For incoming connections, we'll use the remote address as peer ID initially
	// The actual peer ID will be determined when we receive the first message
	peerID := peerAddr
	
	t.mutex.Lock()
	t.connections[peerID] = conn
	t.mutex.Unlock()
	
	t.handleConnection(peerID, conn)
}

// handleConnection handles messages from a connection
func (t *TCPMessageTransport) handleConnection(peerID string, conn net.Conn) {
	defer func() {
		conn.Close()
		t.mutex.Lock()
		delete(t.connections, peerID)
		
		// Remove from peers if exists
		if peer, exists := t.peers.Get(peerID); exists {
			peer.Disconnect()
		}
		t.mutex.Unlock()
		
		t.logger.Info("Connection closed", "peer", peerID)
	}()
	
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		select {
		case <-t.ctx.Done():
			return
		default:
			data := scanner.Bytes()
			if len(data) == 0 {
				continue
			}
			
			message, err := models.FromJSON(data)
			if err != nil {
				t.logger.Error("Failed to parse message", "peer", peerID, "error", err)
				continue
			}
			
			// Update peer ID based on message sender
			if message.From != "" && message.From != peerID {
				t.mutex.Lock()
				// Remove old mapping and add new one
				if oldConn, exists := t.connections[peerID]; exists && oldConn == conn {
					delete(t.connections, peerID)
					t.connections[message.From] = conn
				}
				peerID = message.From
				t.mutex.Unlock()
			}
			
			if t.messageHandler != nil {
				t.messageHandler.OnMessageReceived(message)
			}
			
			t.logger.Info("Message received", "peer", peerID, "type", message.Type, "from", message.From, "to", message.To, "content", message.Content)
		}
	}
	
	if err := scanner.Err(); err != nil {
		t.logger.Error("Error reading from connection", "peer", peerID, "error", err)
		if t.messageHandler != nil {
			t.messageHandler.OnConnectionError(peerID, err)
		}
	}
}
