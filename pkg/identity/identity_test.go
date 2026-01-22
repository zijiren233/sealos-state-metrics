package identity //nolint:testpackage // Need to test unexported generateRandomID

import (
	"testing"
)

func TestGet(t *testing.T) {
	// Reset for clean test
	Reset()

	// Test with POD_NAME env var
	t.Setenv("POD_NAME", "test-pod-123")

	id1 := Get()
	if id1 != "test-pod-123" {
		t.Errorf("Expected 'test-pod-123', got '%s'", id1)
	}

	// Verify it's cached (should return same value even after changing env)
	t.Setenv("POD_NAME", "different-pod")

	id2 := Get()
	if id2 != "test-pod-123" {
		t.Errorf("Expected cached 'test-pod-123', got '%s'", id2)
	}
}

func TestGetWithConfig(t *testing.T) {
	Reset()

	t.Setenv("POD_NAME", "test-pod-456")

	// Test with explicit config
	id := GetWithConfig("explicit-config")
	if id != "explicit-config" {
		t.Errorf("Expected 'explicit-config', got '%s'", id)
	}

	// Test with empty config (should fall back to global)
	id2 := GetWithConfig("")
	if id2 != "test-pod-456" {
		t.Errorf("Expected 'test-pod-456', got '%s'", id2)
	}
}

func TestGetFallbackPriority(t *testing.T) {
	Reset()

	// Set POD_NAME to empty to test fallback priority: IP -> hostname -> random
	t.Setenv("POD_NAME", "")

	id := Get()

	// Should get either IP, hostname, or random ID
	if id == "" {
		t.Error("Expected non-empty identity")
	}

	// Log what we got
	t.Logf("Got identity: %s", id)
}

func TestReset(t *testing.T) {
	Reset()

	t.Setenv("POD_NAME", "first-pod")

	id1 := Get()
	if id1 != "first-pod" {
		t.Errorf("Expected 'first-pod', got '%s'", id1)
	}

	// Reset and change env
	Reset()
	t.Setenv("POD_NAME", "second-pod")

	id2 := Get()
	if id2 != "second-pod" {
		t.Errorf("Expected 'second-pod' after reset, got '%s'", id2)
	}
}

func TestComputeFallback(t *testing.T) {
	// Test that compute returns a non-empty string
	// It should try POD_NAME -> hostname -> IP -> random
	Reset()
	t.Setenv("POD_NAME", "")

	id := Get()
	if id == "" {
		t.Error("Expected non-empty identity")
	}

	t.Logf("Computed identity: %s", id)
}

func TestGenerateRandomID(t *testing.T) {
	id1 := generateRandomID()
	id2 := generateRandomID()

	// Should start with prefix
	if id1[:9] != "instance-" {
		t.Errorf("Expected random ID to start with 'instance-', got '%s'", id1)
	}

	// Should be different
	if id1 == id2 {
		t.Errorf("Expected different random IDs, got same: %s", id1)
	}
}
