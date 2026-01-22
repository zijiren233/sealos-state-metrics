package config

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
)

// ReloadCallback is called when configuration file changes
type ReloadCallback func(configContent []byte) error

// Reloader watches configuration file and triggers reload on changes
type Reloader struct {
	configPath string
	callback   ReloadCallback
	logger     *log.Entry

	watcher  *fsnotify.Watcher
	stopCh   chan struct{}
	stopOnce sync.Once

	// Debounce configuration changes
	debounce time.Duration
	mu       sync.Mutex
	timer    *time.Timer
}

// NewReloader creates a new configuration reloader
func NewReloader(configPath string, callback ReloadCallback) (*Reloader, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Reloader{
		configPath: configPath,
		callback:   callback,
		logger:     log.WithField("component", "config-reloader"),
		watcher:    watcher,
		stopCh:     make(chan struct{}),
		debounce:   3 * time.Second, // 3 second debounce for Kubernetes ConfigMap updates
	}, nil
}

// Start starts watching the configuration file
func (r *Reloader) Start(ctx context.Context) error {
	// Watch the directory containing the config file
	// This is necessary because Kubernetes ConfigMap mounts use symlinks
	// and the actual file is updated atomically by replacing the symlink target
	configDir := filepath.Dir(r.configPath)

	if err := r.watcher.Add(configDir); err != nil {
		return err
	}

	r.logger.WithFields(log.Fields{
		"config_path": r.configPath,
		"watch_dir":   configDir,
	}).Info("Configuration reloader started")

	go r.watchLoop(ctx)

	return nil
}

// Stop stops the configuration reloader
func (r *Reloader) Stop() error {
	r.stopOnce.Do(func() {
		close(r.stopCh)
		if r.watcher != nil {
			_ = r.watcher.Close()
		}

		r.mu.Lock()
		if r.timer != nil {
			r.timer.Stop()
		}
		r.mu.Unlock()
	})

	return nil
}

// watchLoop monitors file system events
func (r *Reloader) watchLoop(ctx context.Context) {
	for {
		select {
		case event, ok := <-r.watcher.Events:
			if !ok {
				return
			}

			// We're watching the directory, so filter events for our config file
			// Kubernetes ConfigMap updates trigger multiple events:
			// - ..data symlink is updated (points to new timestamped dir)
			// - The config file symlink may change
			// We need to handle both direct file changes and symlink updates
			eventPath := filepath.Clean(event.Name)
			configPath := filepath.Clean(r.configPath)
			configBase := filepath.Base(r.configPath)

			// Check if this event is related to our config file
			isConfigFile := eventPath == configPath || filepath.Base(eventPath) == configBase
			isDataSymlink := filepath.Base(eventPath) == "..data"

			r.logger.WithFields(log.Fields{
				"event":         event.Op.String(),
				"path":          event.Name,
				"isConfigFile":  isConfigFile,
				"isDataSymlink": isDataSymlink,
			}).Debug("File system event received")

			if !isConfigFile && !isDataSymlink {
				continue
			}

			// React to Write, Create, and Remove events
			// Remove is important for Kubernetes ConfigMap updates where ..data symlink is removed then recreated
			if event.Op&fsnotify.Write == fsnotify.Write ||
			   event.Op&fsnotify.Create == fsnotify.Create ||
			   event.Op&fsnotify.Remove == fsnotify.Remove {
				r.logger.WithFields(log.Fields{
					"event": event.Op.String(),
					"path":  event.Name,
				}).Debug("Configuration file change detected")

				r.scheduleReload()
			}

		case err, ok := <-r.watcher.Errors:
			if !ok {
				return
			}
			r.logger.WithError(err).Error("File watcher error")

		case <-r.stopCh:
			r.logger.Info("Configuration reloader stopped")
			return

		case <-ctx.Done():
			r.logger.Info("Context cancelled, stopping configuration reloader")
			return
		}
	}
}

// scheduleReload schedules a reload with debouncing
// This prevents multiple rapid reloads when Kubernetes updates ConfigMap
func (r *Reloader) scheduleReload() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Cancel existing timer
	if r.timer != nil {
		r.timer.Stop()
	}

	// Schedule new reload
	r.timer = time.AfterFunc(r.debounce, func() {
		if err := r.reload(); err != nil {
			r.logger.WithError(err).Error("Failed to reload configuration")
		}
	})
}

// reload reads the configuration file and triggers the callback
func (r *Reloader) reload() error {
	r.logger.Info("Reloading configuration")

	// Read configuration file
	content, err := os.ReadFile(r.configPath)
	if err != nil {
		return err
	}

	// Trigger callback
	if err := r.callback(content); err != nil {
		r.logger.WithError(err).Error("Configuration reload callback failed")
		return err
	}

	r.logger.Info("Configuration reloaded successfully")
	return nil
}
