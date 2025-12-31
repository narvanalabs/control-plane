// Package logs provides real-time log streaming functionality.
package logs

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
)

// Subscriber represents a log stream subscriber.
type Subscriber struct {
	ID           string
	AppID        string
	DeploymentID string
	Source       string // "build", "runtime", or "" for all
	Ch           chan *models.LogEntry
	CreatedAt    time.Time
}

// Broker manages log subscriptions and publishing.
type Broker struct {
	mu          sync.RWMutex
	subscribers map[string]*Subscriber // subscriber ID -> subscriber
	logger      *slog.Logger
}

// NewBroker creates a new log broker.
func NewBroker(logger *slog.Logger) *Broker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Broker{
		subscribers: make(map[string]*Subscriber),
		logger:      logger,
	}
}

// Subscribe creates a new subscription for log events.
// Returns a subscriber that can be used to receive logs.
func (b *Broker) Subscribe(ctx context.Context, appID, deploymentID, source string) *Subscriber {
	b.mu.Lock()
	defer b.mu.Unlock()

	sub := &Subscriber{
		ID:           generateSubscriberID(),
		AppID:        appID,
		DeploymentID: deploymentID,
		Source:       source,
		Ch:           make(chan *models.LogEntry, 100), // Buffered channel
		CreatedAt:    time.Now(),
	}

	b.subscribers[sub.ID] = sub
	b.logger.Debug("subscriber added",
		"subscriber_id", sub.ID,
		"app_id", appID,
		"deployment_id", deploymentID,
	)

	return sub
}

// Unsubscribe removes a subscription.
func (b *Broker) Unsubscribe(sub *Subscriber) {
	if sub == nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.subscribers[sub.ID]; exists {
		close(sub.Ch)
		delete(b.subscribers, sub.ID)
		b.logger.Debug("subscriber removed", "subscriber_id", sub.ID)
	}
}

// Publish sends a log entry to all matching subscribers.
func (b *Broker) Publish(entry *models.LogEntry) {
	if entry == nil {
		return
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, sub := range b.subscribers {
		if b.matches(sub, entry) {
			select {
			case sub.Ch <- entry:
				// Successfully sent
			default:
				// Channel full, skip this entry for this subscriber
				b.logger.Warn("subscriber channel full, dropping log entry",
					"subscriber_id", sub.ID,
					"deployment_id", entry.DeploymentID,
				)
			}
		}
	}
}

// PublishBatch sends multiple log entries to all matching subscribers.
func (b *Broker) PublishBatch(entries []*models.LogEntry) {
	for _, entry := range entries {
		b.Publish(entry)
	}
}

// matches checks if a log entry matches a subscriber's filters.
func (b *Broker) matches(sub *Subscriber, entry *models.LogEntry) bool {
	// If subscriber has a specific deployment ID, it must match
	if sub.DeploymentID != "" && sub.DeploymentID != entry.DeploymentID {
		return false
	}

	// If subscriber has a specific source filter, it must match
	if sub.Source != "" && sub.Source != entry.Source {
		return false
	}

	return true
}

// SubscriberCount returns the number of active subscribers.
func (b *Broker) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}

// generateSubscriberID generates a unique subscriber ID.
func generateSubscriberID() string {
	return time.Now().Format("20060102150405.000000000")
}
