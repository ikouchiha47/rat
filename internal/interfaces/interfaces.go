package interfaces

import (
	"context"
	"net"

	"rat/internal/models"
)

// DiscoveryService handles peer discovery using mDNS
type DiscoveryService interface {
	// Start begins advertising this service and discovering peers
	Start(ctx context.Context) error
	
	// Stop stops the discovery service
	Stop() error
	
	// SetListenPort updates the port to advertise for this service. Should be
	// called before Start so mDNS registration has a non-zero port.
	SetListenPort(port int)
	
	// GetPeers returns the current list of discovered peers
	GetPeers() []*models.Peer
	
	// SetPeerHandler sets a callback for when peers are discovered or lost
	SetPeerHandler(handler PeerEventHandler)
	
	// GetLocalPeer returns information about the local peer
	GetLocalPeer() *models.Peer
}

// MessageTransport handles message sending and receiving between peers
type MessageTransport interface {
	// Start begins listening for incoming connections
	Start(ctx context.Context) error
	
	// Stop stops the transport service
	Stop() error
	
	// SendMessage sends a message to a specific peer
	SendMessage(peerID string, message *models.Message) error
	
	// BroadcastMessage sends a message to all connected peers
	BroadcastMessage(message *models.Message) error
	
	// SetMessageHandler sets a callback for incoming messages
	SetMessageHandler(handler MessageEventHandler)
	
	// ConnectToPeer establishes a connection to a peer
	ConnectToPeer(peer *models.Peer) error
	
	// DisconnectFromPeer closes connection to a peer
	DisconnectFromPeer(peerID string) error
	
	// GetConnectedPeers returns list of currently connected peers
	GetConnectedPeers() []*models.Peer
	
	// GetListenAddress returns the address this transport is listening on
	GetListenAddress() net.Addr
}

// ChatService coordinates the overall chat functionality
type ChatService interface {
	// Start initializes and starts the chat service
	Start(ctx context.Context) error
	
	// Stop gracefully shuts down the chat service
	Stop() error
	
	// SendMessage sends a chat message
	SendMessage(content, recipient string) error
	
	// RegisterUser registers the current user with a username
	RegisterUser(username string) error
	
	// ConnectRoom connects to a specific room
	ConnectRoom(roomName string) error
	
	// LeaveRoom leaves the current room
	LeaveRoom() error
	
	// GetMessages returns the message history
	GetMessages() []*models.Message
	
	// GetPeers returns the current peer list
	GetPeers() []*models.Peer
	
	// GetCurrentRoom returns the current room name
	GetCurrentRoom() string
	
	// GetUsername returns the current username
	GetUsername() string
	
	// GetUsersInCurrentRoom returns all users in the current room
	GetUsersInCurrentRoom() []string
	
	// GetAllRooms returns all available rooms
	GetAllRooms() []string
	
	// IsUsernameAvailable checks if a username is available
	IsUsernameAvailable(username string) bool
	
	// SetEventHandler sets the event handler for UI updates
	SetEventHandler(handler ChatEventHandler)
}

// PeerEventHandler handles peer discovery events
type PeerEventHandler interface {
	OnPeerDiscovered(peer *models.Peer)
	OnPeerLost(peerID string)
	OnPeerConnected(peer *models.Peer)
	OnPeerDisconnected(peerID string)
}

// MessageEventHandler handles message events
type MessageEventHandler interface {
	OnMessageReceived(message *models.Message)
	OnMessageSent(message *models.Message)
	OnConnectionError(peerID string, err error)
}

// ChatEventHandler handles chat service events for UI updates
type ChatEventHandler interface {
	OnMessageReceived(message *models.Message)
	OnPeerJoined(peer *models.Peer)
	OnPeerLeft(peerID string)
	OnRoomChanged(roomName string)
	OnError(err error)
}

// UIModel represents the Bubble Tea model interface
type UIModel interface {
	// Update handles Bubble Tea update messages
	Update(msg interface{}) (UIModel, interface{})
	
	// View renders the current UI state
	View() string
	
	// Init returns initial commands for Bubble Tea
	Init() interface{}
}

// Configuration holds application configuration
type Configuration interface {
	GetUsername() string
	GetRoom() string
	GetPort() int
	GetLogLevel() string
	GetServiceName() string
	GetServiceType() string
	GetQuiet() bool
	GetLogFile() string
}
