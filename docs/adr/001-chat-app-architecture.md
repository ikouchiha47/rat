# ADR-001: Chat App Architecture for Local Network Communication

## Status
Accepted

## Context
We need to build a chat application that allows users on the same WiFi network or Zerotier channel to communicate with each other. The application should have a terminal-based user interface and use service discovery for automatic peer detection.

## Decision
We will build a chat application with the following architecture:

### Core Technologies
- **Bubble Tea**: For the terminal user interface (TUI)
- **Go mDNS**: For service discovery and peer detection
- **TCP/WebSocket**: For reliable message transport
- **JSON**: For message serialization

### Architecture Principles (SOLID)
1. **Single Responsibility Principle**: Each component has one clear responsibility
2. **Open/Closed Principle**: Components are open for extension, closed for modification
3. **Liskov Substitution Principle**: Interfaces can be substituted with implementations
4. **Interface Segregation Principle**: Small, focused interfaces
5. **Dependency Inversion Principle**: Depend on abstractions, not concretions

### Component Architecture

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

### Key Components

#### 1. Discovery Service (mDNS)
- **Responsibility**: Advertise local service and discover peers
- **Interface**: `DiscoveryService`
- **Implementation**: `MDNSDiscoveryService`

#### 2. Message Transport
- **Responsibility**: Handle message sending/receiving between peers
- **Interface**: `MessageTransport`
- **Implementation**: `TCPMessageTransport`

#### 3. Chat Service
- **Responsibility**: Coordinate messaging, user management, and business logic
- **Interface**: `ChatService`
- **Implementation**: `LocalChatService`

#### 4. UI Layer (Bubble Tea)
- **Responsibility**: Handle user interactions and display
- **Components**: 
  - Main model with different views (lobby, chat room)
  - Message input component
  - Peer list component
  - Chat history component

#### 5. Logging System
- **Responsibility**: Structured logging with levels
- **Implementation**: Using `slog` with configurable levels
- **Levels**: DEBUG, INFO, WARN, ERROR
- **Default**: DEBUG in development

### Message Protocol

```json
{
  "type": "message|join|leave|heartbeat",
  "from": "user_id",
  "to": "user_id|broadcast",
  "content": "message_content",
  "timestamp": "2023-01-01T00:00:00Z",
  "room": "room_id"
}
```

### Service Discovery Protocol
- **Service Type**: `_localchat._tcp`
- **Port**: Dynamic (advertised via mDNS)
- **TXT Records**: 
  - `version=1.0`
  - `user=username`
  - `room=default`

### Configuration
- **Environment Variables**: 
  - `LOG_LEVEL` (DEBUG, INFO, WARN, ERROR)
  - `CHAT_PORT` (default: random available port)
  - `USERNAME` (default: system username)
  - `ROOM` (default: "general")

## Consequences

### Positive
- **Pluggable Architecture**: Easy to swap implementations (e.g., WebSocket instead of TCP)
- **SOLID Principles**: Maintainable and extensible code
- **Local Network Focus**: No external dependencies or servers required
- **Rich TUI**: Modern terminal interface with Bubble Tea
- **Automatic Discovery**: Users don't need to manually configure connections

### Negative
- **Network Dependency**: Requires local network connectivity
- **mDNS Limitations**: May not work in all network environments
- **Terminal Only**: No GUI interface
- **Single Room**: Initial implementation supports one room per instance

### Risks & Mitigations
- **Network Firewalls**: Document port requirements and provide troubleshooting
- **mDNS Blocking**: Provide manual peer addition as fallback
- **Message Ordering**: Implement message sequencing for reliability
- **Connection Drops**: Implement reconnection logic with exponential backoff

## Implementation Plan
1. Set up project structure and dependencies
2. Implement logging system
3. Create core interfaces and models
4. Implement mDNS discovery service
5. Implement TCP message transport
6. Create chat service business logic
7. Build Bubble Tea UI components
8. Integration and testing
9. Documentation and examples

## Alternatives Considered
- **gRPC**: Too heavy for simple chat messages
- **UDP**: Less reliable for chat messages
- **Broadcast**: Limited by network topology
- **WebRTC**: Complex setup for local network use
