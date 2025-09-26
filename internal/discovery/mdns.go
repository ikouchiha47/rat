package discovery

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/grandcat/zeroconf"
	"rat/internal/interfaces"
	"rat/internal/logger"
	"rat/internal/models"
)

// MDNSDiscoveryService implements the DiscoveryService interface using mDNS
type MDNSDiscoveryService struct {
	config        interfaces.Configuration
	logger        *logger.Logger
	localPeer     *models.Peer
	peers         *models.PeerList
	resolver      *zeroconf.Resolver
	server        *zeroconf.Server
	peerHandler   interfaces.PeerEventHandler
	ctx           context.Context
	cancel        context.CancelFunc
	isRunning     bool
	serviceName   string
	serviceType   string
	domain        string
	listenPort    int
}

// NewMDNSDiscoveryService creates a new mDNS discovery service
func NewMDNSDiscoveryService(config interfaces.Configuration, logger *logger.Logger) interfaces.DiscoveryService {
	return &MDNSDiscoveryService{
		config:      config,
		logger:      logger.WithComponent("mdns-discovery"),
		peers:       models.NewPeerList(),
		serviceName: config.GetServiceName(),
		serviceType: config.GetServiceType(),
		domain:      "local.",
	}
}

// Start begins the discovery service
func (m *MDNSDiscoveryService) Start(ctx context.Context) error {
	m.logger.Info("Starting mDNS discovery service")
	
	m.ctx, m.cancel = context.WithCancel(ctx)
	
	// Create local peer information (preserve if already created)
	m.createOrUpdateLocalPeer()
	
	// Start service registration
	if err := m.registerService(); err != nil {
		return fmt.Errorf("failed to register service: %w", err)
	}
	
	// Start service discovery
	if err := m.startDiscovery(); err != nil {
		return fmt.Errorf("failed to start discovery: %w", err)
	}
	
	m.isRunning = true
	m.logger.Info("mDNS discovery service started successfully")
	return nil
}

// Stop stops the discovery service
func (m *MDNSDiscoveryService) Stop() error {
	m.logger.Info("Stopping mDNS discovery service")
	
	if m.cancel != nil {
		m.cancel()
	}
	
	if m.server != nil {
		m.server.Shutdown()
	}
	
	m.isRunning = false
	m.logger.Info("mDNS discovery service stopped")
	return nil
}

// GetPeers returns the current list of discovered peers
func (m *MDNSDiscoveryService) GetPeers() []*models.Peer {
	return m.peers.GetAll()
}

// SetPeerHandler sets the peer event handler
func (m *MDNSDiscoveryService) SetPeerHandler(handler interfaces.PeerEventHandler) {
	m.peerHandler = handler
}

// GetLocalPeer returns the local peer information
func (m *MDNSDiscoveryService) GetLocalPeer() *models.Peer {
	return m.localPeer
}

// SetListenPort allows setting the listen port before Start is called
func (m *MDNSDiscoveryService) SetListenPort(port int) {
	m.listenPort = port
	if m.localPeer != nil {
		m.localPeer.Port = port
	}
	m.logger.Debug("Listen port set for mDNS", "port", port)
}

// createLocalPeer creates the local peer instance
func (m *MDNSDiscoveryService) createOrUpdateLocalPeer() {
	hostname, _ := os.Hostname()
	
	// Get local IP addresses
	localIPs := m.getLocalIPs()
	var localIP net.IP
	
	// Prefer non-loopback IPv4 addresses
	for _, ip := range localIPs {
		if ip.To4() != nil && !ip.IsLoopback() {
			localIP = ip
			break
		}
	}
	
	// Fallback to loopback if no other IP found
	if localIP == nil && len(localIPs) > 0 {
		localIP = localIPs[0]
	} else if localIP == nil {
		localIP = net.ParseIP("127.0.0.1")
	}
	
	if m.localPeer == nil {
		m.localPeer = models.NewPeer(
			fmt.Sprintf("%s-%s", hostname, m.config.GetUsername()),
			m.config.GetUsername(),
			hostname,
			localIP,
			m.config.GetPort(),
			m.config.GetRoom(),
		)
	} else {
		// Update dynamic fields while preserving any fields already set
		m.localPeer.Hostname = hostname
		m.localPeer.Address = localIP
		if m.localPeer.Room == "" {
			m.localPeer.Room = m.config.GetRoom()
		}
	}
	
	// If a listenPort was provided, ensure we advertise it
	if m.listenPort != 0 {
		m.localPeer.Port = m.listenPort
	}
	
	m.logger.Debug("Created local peer", 
		"id", m.localPeer.ID,
		"username", m.localPeer.Username,
		"hostname", m.localPeer.Hostname,
		"address", localIP.String(),
		"port", m.localPeer.Port,
	)
}

// registerService registers the local service with mDNS
func (m *MDNSDiscoveryService) registerService() error {
	if m.localPeer.Port == 0 {
		return fmt.Errorf("no listen port set for mDNS registration")
	}
	
	// TXT records with additional information
	txtRecords := []string{
		fmt.Sprintf("username=%s", m.localPeer.Username),
		fmt.Sprintf("room=%s", m.localPeer.Room),
		fmt.Sprintf("version=%s", m.localPeer.Version),
		fmt.Sprintf("id=%s", m.localPeer.ID),
	}
	
	var err error
	m.server, err = zeroconf.Register(
		m.localPeer.Username,
		m.serviceType,
		m.domain,
		m.localPeer.Port,
		txtRecords,
		nil,
	)
	
	if err != nil {
		return fmt.Errorf("failed to register mDNS service: %w", err)
	}
	
	m.logger.Info("Registered mDNS service", 
		"name", m.localPeer.Username,
		"type", m.serviceType,
		"port", m.localPeer.Port,
	)
	
	return nil
}

// startDiscovery starts browsing for other services
func (m *MDNSDiscoveryService) startDiscovery() error {
	var err error
	m.resolver, err = zeroconf.NewResolver(nil)
	if err != nil {
		return fmt.Errorf("failed to create resolver: %w", err)
	}
	
	entries := make(chan *zeroconf.ServiceEntry)
	
	// Start the discovery goroutine
	go m.handleDiscoveryEntries(entries)
	
	// Start browsing
	go func() {
		err := m.resolver.Browse(m.ctx, m.serviceType, m.domain, entries)
		if err != nil {
			m.logger.Error("Failed to start browsing", "error", err)
		}
	}()
	
	return nil
}

// handleDiscoveryEntries processes discovered service entries
func (m *MDNSDiscoveryService) handleDiscoveryEntries(entries chan *zeroconf.ServiceEntry) {
    for entry := range entries {
        m.logger.Debug("Discovered service",
            "service", entry.Service,
            "instance", entry.Instance,
            "host", entry.HostName,
            "port", entry.Port,
            "addresses", entry.AddrIPv4,
            "txt", entry.Text,
        )

        // Determine if this entry refers to ourselves
        skip := false
        if m.localPeer != nil {
            // Check TXT id
            for _, txt := range entry.Text {
                if strings.HasPrefix(txt, "id=") && txt[3:] == m.localPeer.ID {
                    m.logger.Debug("Skipping self service by id", "id", m.localPeer.ID)
                    skip = true
                    break
                }
            }
            // Hostname fallback
            if !skip && entry.HostName == m.localPeer.Hostname {
                m.logger.Debug("Skipping self service by hostname")
                skip = true
            }
            // Address+port fallback
            if !skip && len(entry.AddrIPv4) > 0 && entry.Port == m.localPeer.Port && entry.AddrIPv4[0].Equal(m.localPeer.Address) {
                m.logger.Debug("Skipping self service by addr:port")
                skip = true
            }
        }
        if skip {
            continue
        }

        peer := m.parseServiceEntry(entry)
        if peer == nil {
            continue
        }
        if m.localPeer != nil && peer.ID == m.localPeer.ID {
            m.logger.Debug("Skipping self service after parse", "id", peer.ID)
            continue
        }
        m.peers.Add(peer)
        if m.peerHandler != nil {
            m.peerHandler.OnPeerDiscovered(peer)
        }
    }

    m.logger.Debug("Discovery entries channel closed")
}

// parseServiceEntry parses a zeroconf service entry into a Peer
func (m *MDNSDiscoveryService) parseServiceEntry(entry *zeroconf.ServiceEntry) *models.Peer {
	if len(entry.AddrIPv4) == 0 {
		m.logger.Warn("Service entry has no IPv4 addresses", "instance", entry.Instance)
		return nil
	}

	// Parse TXT records
	var username, room, version, id string
	for _, txt := range entry.Text {
		switch {
		case len(txt) > 9 && txt[:9] == "username=":
			username = txt[9:]
		case len(txt) > 5 && txt[:5] == "room=":
			room = txt[5:]
		case len(txt) > 8 && txt[:8] == "version=":
			version = txt[8:]
		case len(txt) > 3 && txt[:3] == "id=":
			id = txt[3:]
		}
	}

	// Use instance name as username if not provided in TXT
	if username == "" {
		username = entry.Instance
	}

	// Use hostname as ID if not provided
	if id == "" {
		id = fmt.Sprintf("%s-%s", entry.HostName, username)
	}

	// Default room if not provided
	if room == "" {
		room = "general"
	}

	// Default version if not provided
	if version == "" {
		version = "1.0"
	}

	peer := models.NewPeer(
		id,
		username,
		entry.HostName,
		entry.AddrIPv4[0],
		entry.Port,
		room,
	)
	peer.Version = version

	return peer
}

// getLocalIPs returns all local IP addresses
func (m *MDNSDiscoveryService) getLocalIPs() []net.IP {
	var ips []net.IP
	
	interfaces, err := net.Interfaces()
	if err != nil {
		m.logger.Error("Failed to get network interfaces", "error", err)
		return ips
	}
	
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		
		addrs, err := iface.Addrs()
		if err != nil {
			m.logger.Warn("Failed to get addresses for interface", "interface", iface.Name, "error", err)
			continue
		}
		
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			
			if ip != nil {
				ips = append(ips, ip)
			}
		}
	}
	
	return ips
}
