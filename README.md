# Local Chat Application

A terminal-based chat application that allows users on the same WiFi network or Zerotier channel to communicate with each other using mDNS for service discovery and TCP for message transport.

## Features

- **Zero Configuration**: Automatic peer discovery using mDNS/DNS-SD
- **Terminal UI**: Beautiful terminal interface built with Bubble Tea
- **Room-based Chat**: Join and leave chat rooms
- **Real-time Messaging**: Instant message delivery between peers
- **Peer List**: See who's online and in which room
- **SOLID Architecture**: Pluggable, maintainable code structure
- **Comprehensive Logging**: Configurable log levels for debugging

## Architecture

The application follows SOLID principles with a pluggable architecture:

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Bubble Tea    │    │   Chat Service  │    │ Discovery Service│
│   (UI Layer)    │◄──►│  (Business)     │◄──►│    (mDNS)       │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   UI Models     │    │   Message       │    │   Network       │
│   & Commands    │    │   Transport     │    │   Transport     │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## Installation

### Prerequisites

- Go 1.21 or later
- Terminal with color support

### Build from Source

```bash
# Clone the repository
git clone <repository-url>
cd rat

# Install dependencies
go mod tidy

# Build the application
go build -o localchat cmd/main.go

# Run the application
./localchat
```

## Usage

### Basic Usage

```bash
# Run with interactive username prompt
./localchat

# Run with CLI flags
./localchat -username alice -room dev
./localchat -u bob -r gaming

# Run with CLI flags and custom port
./localchat -username alice -room dev -port 8080

# Show help
./localchat -h
```

### CLI Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `-username` | `-u` | Your username | Interactive prompt |
| `-room` | `-r` | Chat room to join | `general` |
| `-port` | | Port to listen on | Random available |
| `-log` | | Log level (DEBUG, INFO, WARN, ERROR) | `INFO` |
| `-service` | | mDNS service name | `LocalChat` |

### Environment Variables (Fallback)

| Variable | Description | Default |
|----------|-------------|---------|
| `CHAT_USERNAME` | Your username | Interactive prompt |
| `CHAT_ROOM` | Default room to join | `general` |
| `CHAT_PORT` | Port to listen on | Random available |
| `LOG_LEVEL` | Logging level (DEBUG, INFO, WARN, ERROR) | `INFO` |
| `CHAT_SERVICE_NAME` | mDNS service name | `LocalChat` |

### Interactive Mode

When no username is provided via CLI flags, the application will prompt:

```
🚀 Local Chat Application
========================

Enter your username [current_user]: alice
```

### Commands

The application now works like IRC with proper user management:

#### User Registration
- `/register <username>` - Register with a unique username (required before messaging)
- Username must be unique across the network and cannot contain spaces

#### Room Management
- `/connect <room>` - Connect to a specific room (like IRC channels)
- `/leave` - Leave the current room
- `/rooms` - Show all available rooms
- `/room` - Show your current room

#### User Discovery
- `/users` - Show all users in your current room
- `@username message` - Send a private message to a specific user

#### Examples
```
/register alice           # Register as 'alice'
/connect general          # Join the 'general' room
/users                    # See who's in the room
@bob hello there!         # Send private message to bob
hello everyone            # Send message to room
/connect dev              # Switch to 'dev' room
```

### Keyboard Shortcuts

- `Enter` - Send message
- `Tab` - Cycle focus between UI elements
- `?` - Toggle help
- `Ctrl+C` or `q` - Quit the application

## Network Requirements

### mDNS Service Discovery

The application uses mDNS (Multicast DNS) for service discovery:

- **Service Type**: `_localchat._tcp`
- **Domain**: `local.`
- **TXT Records**: Contains username, room, version, and peer ID

### Firewall Configuration

Ensure the following ports are open:

- **mDNS**: UDP port 5353 (for service discovery)
- **TCP**: Dynamic port (advertised via mDNS)

### Network Compatibility

- **WiFi Networks**: Fully supported
- **Zerotier Networks**: Fully supported
- **Cloud/VPS**: Limited support (mDNS may be blocked)
- **Corporate Networks**: May require firewall configuration

## Development

### Project Structure

```
rat/
├── cmd/
│   └── main.go              # Application entry point
├── internal/
│   ├── config/              # Configuration management
│   ├── discovery/           # mDNS service discovery
│   ├── logger/              # Logging utilities
│   ├── models/              # Data models
│   ├── service/             # Chat service business logic
│   ├── transport/           # TCP message transport
│   └── ui/                  # Bubble Tea UI components
├── docs/
│   └── adr/                 # Architecture Decision Records
├── go.mod                   # Go module file
└── README.md               # This file
```

### Architecture Components

1. **Discovery Service** (`internal/discovery/`)
   - mDNS service registration and discovery
   - Peer management and connection handling

2. **Message Transport** (`internal/transport/`)
   - TCP-based message sending and receiving
   - Connection management and error handling

3. **Chat Service** (`internal/service/`)
   - Business logic coordination
   - Room management and message routing

4. **UI Layer** (`internal/ui/`)
   - Bubble Tea terminal interface
   - Real-time updates and user interaction

### Adding New Features

The architecture is designed to be pluggable. To add new features:

1. **New Transport Protocol**: Implement `interfaces.MessageTransport`
2. **New Discovery Method**: Implement `interfaces.DiscoveryService`
3. **New UI**: Implement `interfaces.UIModel`
4. **New Chat Features**: Extend `interfaces.ChatService`

### Testing

```bash
# Run tests
go test ./...

# Run with race detection
go test -race ./...

# Run with coverage
go test -cover ./...
```

### Debugging

Enable debug logging:

```bash
LOG_LEVEL=DEBUG ./localchat
```

Common issues:

1. **mDNS not working**: Check firewall settings and network configuration
2. **No peers discovered**: Ensure all users are on the same network
3. **Connection refused**: Verify port accessibility
4. **Messages not delivered**: Check peer connection status

## Troubleshooting

### mDNS Issues

```bash
# Check if mDNS is working
avahi-browse -a
# or
dns-sd -B _localchat._tcp
```

### Network Diagnostics

```bash
# Check listening ports
netstat -tlnp | grep localchat

# Check mDNS services
avahi-browse -r _localchat._tcp
```

### Debug Mode

Run with maximum logging:

```bash
LOG_LEVEL=DEBUG ./localchat
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Ensure all tests pass
6. Submit a pull request

## License

This project is open source. See the LICENSE file for details.

## Acknowledgments

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Terminal UI framework
- [zeroconf](https://github.com/grandcat/zeroconf) - mDNS/DNS-SD library
- [lipgloss](https://github.com/charmbracelet/lipgloss) - Terminal styling
