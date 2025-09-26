package models

import (
	"net"
	"strconv"
	"time"
)

// Peer represents a discovered peer in the network
type Peer struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	Hostname    string    `json:"hostname"`
	Address     net.IP    `json:"address"`
	Port        int       `json:"port"`
	Room        string    `json:"room"`
	LastSeen    time.Time `json:"last_seen"`
	IsConnected bool      `json:"is_connected"`
	Version     string    `json:"version"`
}

// NewPeer creates a new peer instance
func NewPeer(id, username, hostname string, address net.IP, port int, room string) *Peer {
	return &Peer{
		ID:          id,
		Username:    username,
		Hostname:    hostname,
		Address:     address,
		Port:        port,
		Room:        room,
		LastSeen:    time.Now(),
		IsConnected: false,
		Version:     "1.0",
	}
}

// UpdateLastSeen updates the last seen timestamp
func (p *Peer) UpdateLastSeen() {
	p.LastSeen = time.Now()
}

// IsExpired checks if the peer hasn't been seen for too long
func (p *Peer) IsExpired(timeout time.Duration) bool {
	return time.Since(p.LastSeen) > timeout
}

// GetAddress returns the full address string
func (p *Peer) GetAddress() string {
	return net.JoinHostPort(p.Address.String(), strconv.Itoa(p.Port))
}

// Connect marks the peer as connected
func (p *Peer) Connect() {
	p.IsConnected = true
	p.UpdateLastSeen()
}

// Disconnect marks the peer as disconnected
func (p *Peer) Disconnect() {
	p.IsConnected = false
}

// PeerList represents a collection of peers
type PeerList struct {
	peers map[string]*Peer
}

// NewPeerList creates a new peer list
func NewPeerList() *PeerList {
	return &PeerList{
		peers: make(map[string]*Peer),
	}
}

// Add adds or updates a peer in the list
func (pl *PeerList) Add(peer *Peer) {
	pl.peers[peer.ID] = peer
}

// Remove removes a peer from the list
func (pl *PeerList) Remove(peerID string) {
	delete(pl.peers, peerID)
}

// Get retrieves a peer by ID
func (pl *PeerList) Get(peerID string) (*Peer, bool) {
	peer, exists := pl.peers[peerID]
	return peer, exists
}

// GetAll returns all peers
func (pl *PeerList) GetAll() []*Peer {
	peers := make([]*Peer, 0, len(pl.peers))
	for _, peer := range pl.peers {
		peers = append(peers, peer)
	}
	return peers
}

// GetConnected returns only connected peers
func (pl *PeerList) GetConnected() []*Peer {
	var connected []*Peer
	for _, peer := range pl.peers {
		if peer.IsConnected {
			connected = append(connected, peer)
		}
	}
	return connected
}

// GetByRoom returns peers in a specific room
func (pl *PeerList) GetByRoom(room string) []*Peer {
	var roomPeers []*Peer
	for _, peer := range pl.peers {
		if peer.Room == room {
			roomPeers = append(roomPeers, peer)
		}
	}
	return roomPeers
}

// CleanExpired removes expired peers
func (pl *PeerList) CleanExpired(timeout time.Duration) {
	for id, peer := range pl.peers {
		if peer.IsExpired(timeout) {
			delete(pl.peers, id)
		}
	}
}

// Count returns the number of peers
func (pl *PeerList) Count() int {
	return len(pl.peers)
}
