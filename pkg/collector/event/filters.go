package event

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// EventFilter filters events based on various criteria
type EventFilter struct {
	maxAge           time.Duration
	blacklistReasons map[string]bool
}

// NewEventFilter creates a new event filter
func NewEventFilter(maxAge time.Duration) *EventFilter {
	return &EventFilter{
		maxAge: maxAge,
		blacklistReasons: map[string]bool{
			// Common noisy events that don't indicate issues
			"SyncKubeConfig":          true,
			"FailedMount":             true, // Often transient
			"NodeNotReady":            true, // Redundant with node metrics
			"NodeReady":               true, // Not an issue
			"RegisteredNode":          true, // Normal operation
			"RemovingNode":            true, // Normal operation
			"DeletingNode":            true, // Normal operation
			"NodeAllocatableEnforced": true, // Normal operation
			"Starting":                true, // Informational
			"Pulled":                  true, // Success event
			"Created":                 true, // Success event
			"Started":                 true, // Success event
			"Killing":                 true, // Normal operation
			"NodeHasSufficientDisk":   true, // Positive
			"NodeHasSufficientMemory": true, // Positive
			"NodeHasNoDiskPressure":   true, // Positive
		},
	}
}

// ShouldInclude returns true if the event should be included
func (f *EventFilter) ShouldInclude(event *corev1.Event) bool {
	// Filter by age
	if f.maxAge > 0 {
		age := time.Since(event.LastTimestamp.Time)
		if age > f.maxAge {
			return false
		}
	}

	// Filter by blacklist
	if f.blacklistReasons[event.Reason] {
		return false
	}

	// Only include Warning and Error events (and some Normal events that indicate issues)
	if event.Type != corev1.EventTypeWarning {
		// Some Normal events that we want to track
		issueReasons := map[string]bool{
			"BackOff":          true,
			"FailedScheduling": true,
			"FailedCreate":     true,
			"FailedDelete":     true,
			"FailedUpdate":     true,
		}
		if !issueReasons[event.Reason] {
			return false
		}
	}

	return true
}
