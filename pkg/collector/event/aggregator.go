package event

import (
	"maps"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// AggregatedEvent represents aggregated event information
type AggregatedEvent struct {
	Namespace     string
	Reason        string
	Type          string
	Kind          string
	Count         int
	LastSeen      time.Time
	FirstSeen     time.Time
	UniqueObjects map[string]bool // object names
}

// EventAggregator aggregates similar events to reduce metric cardinality
type EventAggregator struct {
	mu         sync.RWMutex
	windowSize time.Duration
	maxEvents  int
	aggregated map[string]*AggregatedEvent // key: namespace/reason/kind
	stopCh     chan struct{}
	logger     *log.Entry
}

// NewEventAggregator creates a new event aggregator
func NewEventAggregator(
	windowSize time.Duration,
	maxEvents int,
	logger *log.Entry,
) *EventAggregator {
	return &EventAggregator{
		windowSize: windowSize,
		maxEvents:  maxEvents,
		aggregated: make(map[string]*AggregatedEvent),
		stopCh:     make(chan struct{}),
		logger:     logger,
	}
}

// Start starts the aggregator cleanup goroutine
func (a *EventAggregator) Start() {
	go a.cleanupLoop()

	a.logger.WithFields(log.Fields{
		"windowSize": a.windowSize,
		"maxEvents":  a.maxEvents,
	}).Debug("Event aggregator started")
}

// Stop stops the aggregator
func (a *EventAggregator) Stop() {
	close(a.stopCh)
	a.logger.Debug("Event aggregator stopped")
}

// AddEvent adds an event to the aggregator
func (a *EventAggregator) AddEvent(event *corev1.Event) {
	if event == nil {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Check if we've hit the max events limit
	if len(a.aggregated) >= a.maxEvents {
		// Remove oldest event
		a.removeOldest()
	}

	key := eventAggregationKey(event)
	now := time.Now()

	if agg, exists := a.aggregated[key]; exists {
		agg.Count += int(event.Count)

		agg.LastSeen = now
		if event.InvolvedObject.Name != "" {
			agg.UniqueObjects[event.InvolvedObject.Name] = true
		}
	} else {
		objects := make(map[string]bool)
		if event.InvolvedObject.Name != "" {
			objects[event.InvolvedObject.Name] = true
		}

		a.aggregated[key] = &AggregatedEvent{
			Namespace:     event.Namespace,
			Reason:        event.Reason,
			Type:          event.Type,
			Kind:          event.InvolvedObject.Kind,
			Count:         int(event.Count),
			LastSeen:      now,
			FirstSeen:     event.FirstTimestamp.Time,
			UniqueObjects: objects,
		}
	}
}

// GetAggregated returns all aggregated events
func (a *EventAggregator) GetAggregated() map[string]*AggregatedEvent {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make(map[string]*AggregatedEvent, len(a.aggregated))
	maps.Copy(result, a.aggregated)

	return result
}

// cleanupLoop periodically cleans up old entries
func (a *EventAggregator) cleanupLoop() {
	ticker := time.NewTicker(a.windowSize)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.cleanup()
		case <-a.stopCh:
			return
		}
	}
}

// cleanup removes old entries
func (a *EventAggregator) cleanup() {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	// Keep events for 1 hour
	threshold := now.Add(-1 * time.Hour)

	for key, agg := range a.aggregated {
		if agg.LastSeen.Before(threshold) {
			delete(a.aggregated, key)
			a.logger.WithField("key", key).Debug("Cleaned up aggregated event entry")
		}
	}
}

// removeOldest removes the oldest event from the aggregator
func (a *EventAggregator) removeOldest() {
	var (
		oldestKey  string
		oldestTime time.Time
	)

	for key, agg := range a.aggregated {
		if oldestKey == "" || agg.LastSeen.Before(oldestTime) {
			oldestKey = key
			oldestTime = agg.LastSeen
		}
	}

	if oldestKey != "" {
		delete(a.aggregated, oldestKey)
		a.logger.WithField("key", oldestKey).Debug("Removed oldest event due to max events limit")
	}
}

// eventAggregationKey returns the aggregation key for an event
func eventAggregationKey(event *corev1.Event) string {
	return event.Namespace + "/" + event.Reason + "/" + event.InvolvedObject.Kind
}
