# Local Chat Application - Features Summary

## 🚀 **Complete IRC-like Local Chat Application**

### **Core Features Implemented**

#### **1. User Registration & Identity**
- **Unique usernames**: Each user must register with a unique username
- **CLI flags**: `-username alice` or `-u bob`
- **Interactive prompts**: When username not provided via CLI
- **Validation**: No spaces, max 20 chars, network-wide uniqueness

#### **2. Room System (IRC-like)**
- **Dynamic rooms**: Create/join any room by name
- **Room switching**: `/connect <room>` to change rooms
- **Room discovery**: `/rooms` shows all active rooms
- **User presence**: See who's in each room

#### **3. Messaging System**
- **Room messages**: Broadcast to entire room
- **Private messages**: `@username message` for DMs
- **Real-time delivery**: Instant message transmission
- **Message history**: Persistent chat logs

#### **4. User Discovery**
- **Active user list**: `/users` shows room members
- **Real-time updates**: Users appear/disappear as they join/leave
- **Presence notifications**: Join/leave messages

#### **5. Network Discovery**
- **mDNS service discovery**: Automatic peer detection
- **Zero configuration**: No central server needed
- **WiFi/Zerotier compatible**: Works on local networks

### **Usage Examples**

#### **Basic Flow**
```bash
# Start with interactive username prompt
./localchat

# Or specify username via CLI
./localchat -username alice

# In the application:
/register alice          # Register your username
/connect general         # Join the general room
/users                   # See who's in the room
hello everyone           # Send room message
@bob hello there!        # Send private message to bob
/connect dev             # Switch to dev room
/rooms                   # See all active rooms
```

#### **CLI Usage**
```bash
# Full CLI control
./localchat -username alice -room dev -port 8080 -log DEBUG

# Help and usage
./localchat -h
```

### **Commands Reference**

| Command | Description | Example |
|---------|-------------|---------|
| `/register <username>` | Register unique username | `/register alice` |
| `/connect <room>` | Join/connect to room | `/connect general` |
| `/leave` | Leave current room | `/leave` |
| `/users` | Show users in current room | `/users` |
| `/rooms` | Show all active rooms | `/rooms` |
| `/room` | Show current room | `/room` |
| `@username message` | Send private message | `@bob hello!` |
| `?` | Toggle help display | `?` |

### **Architecture Highlights**

#### **SOLID Principles**
- **Single Responsibility**: Each component has one clear purpose
- **Open/Closed**: Extensible for new transport protocols
- **Liskov Substitution**: Interface-based design
- **Interface Segregation**: Small, focused interfaces
- **Dependency Inversion**: Depends on abstractions

#### **Pluggable Components**
- **Discovery Service**: mDNS (can be swapped for other discovery)
- **Message Transport**: TCP (can be swapped for WebSocket, UDP, etc.)
- **UI Layer**: Bubble Tea (can be swapped for GUI, web, etc.)

#### **Logging System**
- **Configurable levels**: DEBUG, INFO, WARN, ERROR
- **Component-based**: Each module has its own logger
- **Development-friendly**: Always DEBUG in dev mode

### **Network Compatibility**

- **WiFi Networks**: Full support
- **Zerotier Networks**: Full support
- **Corporate Networks**: May require firewall configuration
- **mDNS Requirements**: UDP port 5353 for service discovery

### **Build & Run**

```bash
# Build
go build -o localchat cmd/main.go

# Run with interactive prompts
./localchat

# Run with CLI flags
./localchat -username alice -room dev

# Run with debug logging
LOG_LEVEL=DEBUG ./localchat
```

### **File Structure**
```
rat/
├── cmd/main.go              # Application entry point
├── internal/
│   ├── config/              # CLI configuration
│   ├── discovery/           # mDNS service discovery
│   ├── logger/              # Structured logging
│   ├── models/              # Data models & protocols
│   ├── service/             # Business logic & user registry
│   ├── transport/           # TCP message transport
│   └── ui/                  # Bubble Tea terminal UI
├── docs/                    # Architecture documentation
├── Makefile                 # Build automation
└── README.md               # Usage documentation
```

## **Ready to Use!** 🎉

The application is fully functional and ready for local network chat. Simply build and run to start chatting with others on your network!
