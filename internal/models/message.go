package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// MessageType represents the type of message
type MessageType string

const (
	MessageTypeChat        MessageType = "message"
	MessageTypeJoin        MessageType = "join"
	MessageTypeLeave       MessageType = "leave"
	MessageTypeHeartbeat   MessageType = "heartbeat"
	MessageTypeUserList    MessageType = "user_list"
	MessageTypeRegister    MessageType = "register"
	MessageTypePresence    MessageType = "presence"
	MessageTypeUserQuery   MessageType = "user_query"
	MessageTypeUserResponse MessageType = "user_response"
	MessageTypeKeyRequest  MessageType = "key_request"
	MessageTypeKeyResponse MessageType = "key_response"
)

// Message represents a chat message
type Message struct {
	Type      MessageType `json:"type"`
	From      string      `json:"from"`
	To        string      `json:"to"`        // "broadcast" for public messages
	Content   string      `json:"content"`
	Timestamp time.Time   `json:"timestamp"`
	Room      string      `json:"room"`
	MessageID string      `json:"message_id,omitempty"`
	Users     []string    `json:"users,omitempty"`    // For user list messages
	Target    string      `json:"target,omitempty"`   // For targeted messages
}

// NewChatMessage creates a new chat message
func NewChatMessage(from, to, content, room string) *Message {
	return &Message{
		Type:      MessageTypeChat,
		From:      from,
		To:        to,
		Content:   content,
		Timestamp: time.Now(),
		Room:      room,
		MessageID: generateMessageID(),
	}
}

// NewPrivateMessage creates a new private message
func NewPrivateMessage(from, to, content, room string) *Message {
	return &Message{
		Type:      MessageTypeChat,
		From:      from,
		To:        to,
		Content:   content,
		Timestamp: time.Now(),
		Room:      room,
		MessageID: generateMessageID(),
	}
}

// NewJoinMessage creates a new join message
func NewJoinMessage(from, room string) *Message {
	return &Message{
		Type:      MessageTypeJoin,
		From:      from,
		To:        "broadcast",
		Content:   from + " joined the room",
		Timestamp: time.Now(),
		Room:      room,
		MessageID: generateMessageID(),
	}
}

// NewLeaveMessage creates a new leave message
func NewLeaveMessage(from, room string) *Message {
	return &Message{
		Type:      MessageTypeLeave,
		From:      from,
		To:        "broadcast",
		Content:   from + " left the room",
		Timestamp: time.Now(),
		Room:      room,
		MessageID: generateMessageID(),
	}
}

// NewHeartbeatMessage creates a new heartbeat message
func NewHeartbeatMessage(from, room string) *Message {
	return &Message{
		Type:      MessageTypeHeartbeat,
		From:      from,
		To:        "broadcast",
		Content:   "",
		Timestamp: time.Now(),
		Room:      room,
		MessageID: generateMessageID(),
	}
}

// NewRegisterMessage creates a new user registration message
func NewRegisterMessage(from, room string) *Message {
	return &Message{
		Type:      MessageTypeRegister,
		From:      from,
		To:        "broadcast",
		Content:   fmt.Sprintf("User %s registered in room %s", from, room),
		Timestamp: time.Now(),
		Room:      room,
		MessageID: generateMessageID(),
	}
}

// NewPresenceMessage creates a new presence update message
func NewPresenceMessage(from, room string, users []string) *Message {
	return &Message{
		Type:      MessageTypePresence,
		From:      from,
		To:        "broadcast",
		Content:   fmt.Sprintf("Presence update for room %s", room),
		Timestamp: time.Now(),
		Room:      room,
		MessageID: generateMessageID(),
		Users:     users,
	}
}

// NewUserQueryMessage creates a new user query message
func NewUserQueryMessage(from, room string) *Message {
	return &Message{
		Type:      MessageTypeUserQuery,
		From:      from,
		To:        "broadcast",
		Content:   fmt.Sprintf("Requesting user list for room %s", room),
		Timestamp: time.Now(),
		Room:      room,
		MessageID: generateMessageID(),
	}
}

// NewUserResponseMessage creates a new user response message
func NewUserResponseMessage(from, room string, users []string) *Message {
	return &Message{
		Type:      MessageTypeUserResponse,
		From:      from,
		To:        "broadcast",
		Content:   fmt.Sprintf("Users in room %s: %s", room, strings.Join(users, ", ")),
		Timestamp: time.Now(),
		Room:      room,
		MessageID: generateMessageID(),
		Users:     users,
	}
}

// ToJSON converts the message to JSON bytes
func (m *Message) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// FromJSON creates a message from JSON bytes
func FromJSON(data []byte) (*Message, error) {
	var msg Message
	err := json.Unmarshal(data, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// IsPublic returns true if the message is a public broadcast
func (m *Message) IsPublic() bool {
	return m.To == "broadcast"
}

// IsPrivate returns true if the message is a private message
func (m *Message) IsPrivate() bool {
	return !m.IsPublic() && m.Type == MessageTypeChat
}

// IsSystemMessage returns true if the message is a system message
func (m *Message) IsSystemMessage() bool {
	return m.Type == MessageTypeJoin || m.Type == MessageTypeLeave || m.Type == MessageTypeHeartbeat
}

// NewKeyRequestMessage creates a new key request message
func NewKeyRequestMessage(from, room, publicKey string) *Message {
	return &Message{
		Type:      MessageTypeKeyRequest,
		From:      from,
		To:        "broadcast",
		Content:   publicKey, // Base64 encoded public key
		Timestamp: time.Now(),
		Room:      room,
		MessageID: generateMessageID(),
	}
}

// NewKeyResponseMessage creates a new key response message
func NewKeyResponseMessage(from, to, room, encryptedGroupKey string) *Message {
	return &Message{
		Type:      MessageTypeKeyResponse,
		From:      from,
		To:        to,
		Content:   encryptedGroupKey, // Base64 encoded encrypted group key
		Timestamp: time.Now(),
		Room:      room,
		MessageID: generateMessageID(),
	}
}

// generateMessageID generates a unique message ID
func generateMessageID() string {
	return time.Now().Format("20060102150405.000000")
}
