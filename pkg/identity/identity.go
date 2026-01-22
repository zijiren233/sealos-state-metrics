package identity

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"os"
	"sync"
)

var (
	once     sync.Once
	instance string
)

// Get returns the global instance identity.
// It uses sync.Once to ensure it's only computed once.
// Priority: explicit config > POD_NAME env var > hostname
func Get() string {
	once.Do(func() {
		instance = compute()
	})
	return instance
}

// GetWithConfig returns the instance identity, using the provided config if not empty.
// If config is empty, falls back to the global identity.
func GetWithConfig(configIdentity string) string {
	if configIdentity != "" {
		return configIdentity
	}
	return Get()
}

// compute determines the instance identity from environment or system
func compute() string {
	// Priority 1: POD_NAME environment variable
	if podName := os.Getenv("POD_NAME"); podName != "" {
		return podName
	}

	// Priority 2: First non-loopback IP address
	if ip := getOutboundIP(); ip != "" {
		return ip
	}

	// Priority 3: Hostname
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		return hostname
	}

	// Priority 4: Random identifier
	return generateRandomID()
}

// getOutboundIP returns the first non-loopback IP address
func getOutboundIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}

	return ""
}

// generateRandomID generates a random identifier
func generateRandomID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return "instance-" + hex.EncodeToString(b)
}

// Reset resets the cached identity (only for testing)
func Reset() {
	once = sync.Once{}
	instance = ""
}
