package service

import (
	"fmt"
	"sync"
	"time"

	"rat/internal/models"
)

// UserRegistry manages user registration and room membership
type UserRegistry struct {
	users       map[string]*models.Peer
	roomMembers map[string]map[string]bool // room -> set of usernames
	mutex       sync.RWMutex
}

// NewUserRegistry creates a new user registry
func NewUserRegistry() *UserRegistry {
	return &UserRegistry{
		users:       make(map[string]*models.Peer),
		roomMembers: make(map[string]map[string]bool),
	}
}

// RegisterUser registers a new user with a unique username
func (r *UserRegistry) RegisterUser(username string, peer *models.Peer) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Check if username already exists
	if _, exists := r.users[username]; exists {
		return fmt.Errorf("username '%s' is already taken", username)
	}

	// Register the user
	r.users[username] = peer
	
	return nil
}

// UnregisterUser removes a user from the registry
func (r *UserRegistry) UnregisterUser(username string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Remove from all rooms
	for room, members := range r.roomMembers {
		delete(members, username)
		if len(members) == 0 {
			delete(r.roomMembers, room)
		}
	}

	// Remove from users
	delete(r.users, username)
}

// JoinRoom adds a user to a room
func (r *UserRegistry) JoinRoom(username, room string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Check if user exists
	if _, exists := r.users[username]; !exists {
		return fmt.Errorf("user '%s' not found", username)
	}

	// Initialize room if it doesn't exist
	if _, exists := r.roomMembers[room]; !exists {
		r.roomMembers[room] = make(map[string]bool)
	}

	// Add user to room
	r.roomMembers[room][username] = true

	return nil
}

// LeaveRoom removes a user from a room
func (r *UserRegistry) LeaveRoom(username, room string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if members, exists := r.roomMembers[room]; exists {
		delete(members, username)
		if len(members) == 0 {
			delete(r.roomMembers, room)
		}
	}
}

// GetUsersInRoom returns all users in a specific room
func (r *UserRegistry) GetUsersInRoom(room string) []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	members, exists := r.roomMembers[room]
	if !exists {
		return []string{}
	}

	users := make([]string, 0, len(members))
	for username := range members {
		users = append(users, username)
	}

	return users
}

// GetAllRooms returns all active rooms
func (r *UserRegistry) GetAllRooms() []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	rooms := make([]string, 0, len(r.roomMembers))
	for room := range r.roomMembers {
		rooms = append(rooms, room)
	}

	return rooms
}

// IsUsernameTaken checks if a username is already taken
func (r *UserRegistry) IsUsernameTaken(username string) bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	_, exists := r.users[username]
	return exists
}

// GetUser returns user information
func (r *UserRegistry) GetUser(username string) (*models.Peer, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	user, exists := r.users[username]
	return user, exists
}

// GetUserRooms returns all rooms a user is in
func (r *UserRegistry) GetUserRooms(username string) []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	var rooms []string
	for room, members := range r.roomMembers {
		if members[username] {
			rooms = append(rooms, room)
		}
	}

	return rooms
}

// GetRoomCount returns the number of users in a room
func (r *UserRegistry) GetRoomCount(room string) int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return len(r.roomMembers[room])
}

// CleanupExpired removes users that haven't been seen for a while
func (r *UserRegistry) CleanupExpired(timeout time.Duration) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// This would be called periodically to clean up stale users
	// Implementation depends on how we track last activity
}
