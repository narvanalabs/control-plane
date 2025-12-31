package logs

import (
	"sync"

	"github.com/narvanalabs/control-plane/internal/models"
)

const (
	// DefaultMaxLines is the default maximum number of log lines to keep.
	DefaultMaxLines = 5000
)

// Container maintains a bounded collection of log entries.
// It automatically removes the oldest entries when the limit is exceeded.
type Container struct {
	mu       sync.RWMutex
	entries  []*models.LogEntry
	maxLines int
}

// NewContainer creates a new log container with the specified max lines.
func NewContainer(maxLines int) *Container {
	if maxLines <= 0 {
		maxLines = DefaultMaxLines
	}
	return &Container{
		entries:  make([]*models.LogEntry, 0, maxLines),
		maxLines: maxLines,
	}
}

// Add adds a log entry to the container.
// If the container is at capacity, the oldest entry is removed.
func (c *Container) Add(entry *models.LogEntry) {
	if entry == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// If at capacity, remove oldest entries
	if len(c.entries) >= c.maxLines {
		// Remove the oldest 10% to avoid frequent removals
		removeCount := c.maxLines / 10
		if removeCount < 1 {
			removeCount = 1
		}
		c.entries = c.entries[removeCount:]
	}

	c.entries = append(c.entries, entry)
}

// AddBatch adds multiple log entries to the container.
func (c *Container) AddBatch(entries []*models.LogEntry) {
	for _, entry := range entries {
		c.Add(entry)
	}
}

// GetAll returns all log entries in the container.
func (c *Container) GetAll() []*models.LogEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]*models.LogEntry, len(c.entries))
	copy(result, c.entries)
	return result
}

// GetLast returns the last n log entries.
func (c *Container) GetLast(n int) []*models.LogEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if n <= 0 || len(c.entries) == 0 {
		return nil
	}

	if n > len(c.entries) {
		n = len(c.entries)
	}

	start := len(c.entries) - n
	result := make([]*models.LogEntry, n)
	copy(result, c.entries[start:])
	return result
}

// Clear removes all entries from the container.
func (c *Container) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = c.entries[:0]
}

// Len returns the number of entries in the container.
func (c *Container) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// MaxLines returns the maximum number of lines the container can hold.
func (c *Container) MaxLines() int {
	return c.maxLines
}
