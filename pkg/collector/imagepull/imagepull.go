package imagepull

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/zijiren233/sealos-state-metric/pkg/collector/base"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// ImagePullInfo tracks image pull information
type ImagePullInfo struct {
	Namespace string
	Pod       string
	Container string
	Image     string
	Reason    FailureReason
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
	Failed    bool
}

// Collector collects image pull metrics
type Collector struct {
	*base.BaseCollector

	client        kubernetes.Interface
	config        *Config
	podInformer   cache.SharedIndexInformer
	eventInformer cache.SharedIndexInformer
	classifier    *FailureClassifier
	stopCh        chan struct{}
	logger        *log.Entry

	mu         sync.RWMutex
	pullInfo   map[string]*ImagePullInfo // key: namespace/pod/container
	pullEvents map[string]time.Time      // key: namespace/pod/image (Pulling start time)

	// Metrics
	imagePullFailures *prometheus.Desc
	imagePullSlow     *prometheus.Desc
	imagePullDuration *prometheus.Desc
}

// initMetrics initializes Prometheus metric descriptors
func (c *Collector) initMetrics(namespace string) {
	c.imagePullFailures = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "image", "pull_failures"),
		"Image pull failures",
		[]string{"namespace", "pod", "image", "container", "reason"},
		nil,
	)
	c.imagePullSlow = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "image", "pull_slow"),
		"Slow image pulls (duration > threshold)",
		[]string{"namespace", "pod", "image", "container", "duration_seconds"},
		nil,
	)
	c.imagePullDuration = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "image", "pull_duration_seconds"),
		"Image pull duration histogram",
		[]string{"namespace", "image"},
		nil,
	)

	// Register descriptors
	c.MustRegisterDesc(c.imagePullFailures)
	c.MustRegisterDesc(c.imagePullSlow)
	c.MustRegisterDesc(c.imagePullDuration)
}

// Start starts the collector
func (c *Collector) Start(ctx context.Context) error {
	if err := c.BaseCollector.Start(ctx); err != nil {
		return err
	}

	// Create informer factory
	factory := informers.NewSharedInformerFactory(c.client, 10*time.Minute)

	// Create pod informer
	c.podInformer = factory.Core().V1().Pods().Informer()
	//nolint:errcheck // AddEventHandler returns (registration, error) but error is always nil in client-go
	c.podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.handlePodAdd,
		UpdateFunc: c.handlePodUpdate,
		DeleteFunc: c.handlePodDelete,
	})

	// Create event informer
	c.eventInformer = factory.Core().V1().Events().Informer()
	//nolint:errcheck // AddEventHandler returns (registration, error) but error is always nil in client-go
	c.eventInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.handleEventAdd,
		UpdateFunc: c.handleEventUpdate,
	})

	// Start informers
	factory.Start(c.stopCh)

	// Wait for cache sync
	c.logger.Info("Waiting for imagepull informers cache sync")

	if !cache.WaitForCacheSync(c.stopCh, c.podInformer.HasSynced, c.eventInformer.HasSynced) {
		return errors.New("failed to sync imagepull informers cache")
	}

	// Start cleanup goroutine
	go c.cleanupLoop()

	c.logger.Info("ImagePull collector started successfully")

	return nil
}

// Stop stops the collector
func (c *Collector) Stop() error {
	close(c.stopCh)
	return c.BaseCollector.Stop()
}

// HasSynced returns true if the informers have synced
func (c *Collector) HasSynced() bool {
	return c.podInformer != nil && c.podInformer.HasSynced() &&
		c.eventInformer != nil && c.eventInformer.HasSynced()
}

// handlePodAdd handles pod add events
func (c *Collector) handlePodAdd(obj any) {
	pod := obj.(*corev1.Pod) //nolint:errcheck // Type assertion is safe from informer
	c.processPod(pod)
}

// handlePodUpdate handles pod update events
func (c *Collector) handlePodUpdate(oldObj, newObj any) {
	pod := newObj.(*corev1.Pod) //nolint:errcheck // Type assertion is safe from informer
	c.processPod(pod)
}

// handlePodDelete handles pod delete events
func (c *Collector) handlePodDelete(obj any) {
	pod := obj.(*corev1.Pod) //nolint:errcheck // Type assertion is safe from informer

	c.mu.Lock()
	defer c.mu.Unlock()

	// Clean up all pull info for this pod
	prefix := pod.Namespace + "/" + pod.Name + "/"
	for key := range c.pullInfo {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			delete(c.pullInfo, key)
		}
	}
}

// processPod processes a pod to extract image pull information
func (c *Collector) processPod(pod *corev1.Pod) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, containerStatus := range pod.Status.ContainerStatuses {
		key := pullInfoKey(pod.Namespace, pod.Name, containerStatus.Name)

		// Check for image pull failures
		if containerStatus.State.Waiting != nil {
			waiting := containerStatus.State.Waiting
			reason := c.classifier.Classify(waiting.Reason, waiting.Message)

			// Only track actual pull failures
			if reason != FailureReasonUnknown ||
				waiting.Reason == "ImagePullBackOff" ||
				waiting.Reason == "ErrImagePull" {
				c.pullInfo[key] = &ImagePullInfo{
					Namespace: pod.Namespace,
					Pod:       pod.Name,
					Container: containerStatus.Name,
					Image:     containerStatus.Image,
					Reason:    reason,
					Failed:    true,
				}
			}
		}
	}
}

// handleEventAdd handles event add
func (c *Collector) handleEventAdd(obj any) {
	event := obj.(*corev1.Event) //nolint:errcheck // Type assertion is safe from informer
	c.processEvent(event)
}

// handleEventUpdate handles event update
func (c *Collector) handleEventUpdate(oldObj, newObj any) {
	event := newObj.(*corev1.Event) //nolint:errcheck // Type assertion is safe from informer
	c.processEvent(event)
}

// processEvent processes an event to track image pull timing
func (c *Collector) processEvent(event *corev1.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Only interested in image pulling events
	if event.Reason != "Pulling" && event.Reason != "Pulled" {
		return
	}

	if event.InvolvedObject.Kind != "Pod" {
		return
	}

	// Extract image name from message (format: "Pulling image \"image:tag\"" or "Successfully pulled image...")
	// For simplicity, we'll track by pod and timestamp
	eventKey := event.Namespace + "/" + event.InvolvedObject.Name

	switch event.Reason {
	case "Pulling":
		// Record pull start time
		c.pullEvents[eventKey] = event.FirstTimestamp.Time
		c.logger.WithField("pod", eventKey).Debug("Image pull started")
	case "Pulled":
		// Calculate pull duration
		if startTime, exists := c.pullEvents[eventKey]; exists {
			duration := event.FirstTimestamp.Sub(startTime)

			// Check if it's a slow pull
			if duration > c.config.SlowPullThreshold {
				// Extract image from message
				image := extractImageFromMessage(event.Message)

				// Store slow pull info
				// Note: We don't have container name from events, using pod name
				key := pullInfoKey(event.Namespace, event.InvolvedObject.Name, "")
				if existing, ok := c.pullInfo[key]; ok {
					existing.Duration = duration
					existing.StartTime = startTime
					existing.EndTime = event.FirstTimestamp.Time
				} else {
					c.pullInfo[key] = &ImagePullInfo{
						Namespace: event.Namespace,
						Pod:       event.InvolvedObject.Name,
						Image:     image,
						Duration:  duration,
						StartTime: startTime,
						EndTime:   event.FirstTimestamp.Time,
						Failed:    false,
					}
				}

				c.logger.WithFields(log.Fields{
					"pod":      eventKey,
					"duration": duration,
				}).Debug("Slow image pull detected")
			}

			// Clean up
			delete(c.pullEvents, eventKey)
		}
	}
}

// cleanupLoop periodically cleans up old entries
func (c *Collector) cleanupLoop() {
	ticker := time.NewTicker(c.config.EventRetention)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCh:
			return
		}
	}
}

// cleanup removes old entries
func (c *Collector) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	threshold := now.Add(-c.config.EventRetention)

	// Clean up old pull events
	for key, startTime := range c.pullEvents {
		if startTime.Before(threshold) {
			delete(c.pullEvents, key)
		}
	}

	// Clean up old pull info
	for key, info := range c.pullInfo {
		if !info.EndTime.IsZero() && info.EndTime.Before(threshold) {
			delete(c.pullInfo, key)
		}
	}

	c.logger.WithField("remaining", len(c.pullInfo)).Debug("Cleaned up old image pull entries")
}

// collect collects metrics
func (c *Collector) collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, info := range c.pullInfo {
		if info.Failed {
			// Image pull failure
			ch <- prometheus.MustNewConstMetric(
				c.imagePullFailures,
				prometheus.GaugeValue,
				1,
				info.Namespace,
				info.Pod,
				info.Image,
				info.Container,
				string(info.Reason),
			)
		} else if info.Duration > c.config.SlowPullThreshold {
			// Slow image pull
			ch <- prometheus.MustNewConstMetric(
				c.imagePullSlow,
				prometheus.GaugeValue,
				1,
				info.Namespace,
				info.Pod,
				info.Image,
				info.Container,
				fmt.Sprintf("%.0f", info.Duration.Seconds()),
			)
		}

		// Duration histogram
		if info.Duration > 0 {
			ch <- prometheus.MustNewConstMetric(
				c.imagePullDuration,
				prometheus.GaugeValue,
				info.Duration.Seconds(),
				info.Namespace,
				info.Image,
			)
		}
	}
}

// pullInfoKey generates a unique key for pull info
func pullInfoKey(namespace, pod, container string) string {
	if container == "" {
		return namespace + "/" + pod
	}
	return namespace + "/" + pod + "/" + container
}

// extractImageFromMessage extracts image name from event message
func extractImageFromMessage(message string) string {
	// Simple extraction - in production would use better parsing
	// Example: "Successfully pulled image \"nginx:latest\" in 2.5s"
	start := -1

	end := -1
	for i, c := range message {
		if c == '"' {
			if start == -1 {
				start = i + 1
			} else {
				end = i
				break
			}
		}
	}

	if start != -1 && end != -1 {
		return message[start:end]
	}

	return "unknown"
}
