package service

import (
	"container/list"
	"encoding/json"
	"io"
	"sync"
	"time"

	"rat/internal/models"
)

// MessageWriter interface for pluggable message persistence
type MessageWriter interface {
	WriteMessage(message *models.Message) error
	Close() error
}

// DevNullWriter writes messages to oblivion (default)
type DevNullWriter struct{}

func (w *DevNullWriter) WriteMessage(message *models.Message) error {
	// Messages go to /dev/null - do nothing
	return nil
}

func (w *DevNullWriter) Close() error {
	return nil
}

// FileWriter writes messages to a file
type FileWriter struct {
	writer io.WriteCloser
}

func NewFileWriter(writer io.WriteCloser) *FileWriter {
	return &FileWriter{writer: writer}
}

func (w *FileWriter) WriteMessage(message *models.Message) error {
	// Write message as JSON line
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.writer.Write(data)
	return err
}

func (w *FileWriter) Close() error {
	return w.writer.Close()
}

// CacheEntry represents a message in the ordered cache
type CacheEntry struct {
	MessageID string
	Timestamp time.Time
	Message   *models.Message
}

// MessageCache implements an ordered sequence with max limit and pluggable writer
type MessageCache struct {
	maxSize    int
	entries    *list.List                // Ordered list of cache entries
	index      map[string]*list.Element  // Fast lookup by message ID
	writer     MessageWriter             // Pluggable writer for evicted messages
	mutex      sync.RWMutex
}

// NewMessageCache creates a new message cache with pluggable writer
func NewMessageCache(maxSize int, writer MessageWriter) *MessageCache {
	if writer == nil {
		writer = &DevNullWriter{} // Default to oblivion
	}
	
	return &MessageCache{
		maxSize: maxSize,
		entries: list.New(),
		index:   make(map[string]*list.Element),
		writer:  writer,
	}
}

// Contains checks if a message ID exists in the cache
func (c *MessageCache) Contains(messageID string) bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	_, exists := c.index[messageID]
	return exists
}

// Add adds a message to the cache, evicting old messages if needed
func (c *MessageCache) Add(message *models.Message) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	// Check if already exists
	if _, exists := c.index[message.MessageID]; exists {
		return nil // Already in cache
	}
	
	// Create new entry
	entry := &CacheEntry{
		MessageID: message.MessageID,
		Timestamp: time.Now(),
		Message:   message,
	}
	
	// Add to front of list (most recent)
	element := c.entries.PushFront(entry)
	c.index[message.MessageID] = element
	
	// Evict old messages if over limit
	for c.entries.Len() > c.maxSize {
		// Remove from back (oldest)
		oldest := c.entries.Back()
		if oldest != nil {
			oldEntry := oldest.Value.(*CacheEntry)
			
			// Write evicted message to pluggable writer
			if err := c.writer.WriteMessage(oldEntry.Message); err != nil {
				// Log error but continue with eviction
				// Could add logger here if needed
			}
			
			// Remove from cache
			c.entries.Remove(oldest)
			delete(c.index, oldEntry.MessageID)
		}
	}
	
	return nil
}

// Size returns the current cache size
func (c *MessageCache) Size() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.entries.Len()
}

// Clear clears the cache, optionally writing all messages to the writer
func (c *MessageCache) Clear(writeAll bool) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	if writeAll {
		// Write all messages to writer before clearing
		for element := c.entries.Front(); element != nil; element = element.Next() {
			entry := element.Value.(*CacheEntry)
			if err := c.writer.WriteMessage(entry.Message); err != nil {
				return err
			}
		}
	}
	
	// Clear cache
	c.entries.Init()
	c.index = make(map[string]*list.Element)
	
	return nil
}

// Close closes the cache and the underlying writer
func (c *MessageCache) Close() error {
	// Write all remaining messages before closing
	if err := c.Clear(true); err != nil {
		return err
	}
	
	return c.writer.Close()
}

// GetRecentMessages returns the N most recent messages
func (c *MessageCache) GetRecentMessages(n int) []*models.Message {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	var messages []*models.Message
	count := 0
	
	for element := c.entries.Front(); element != nil && count < n; element = element.Next() {
		entry := element.Value.(*CacheEntry)
		messages = append(messages, entry.Message)
		count++
	}
	
	return messages
}
