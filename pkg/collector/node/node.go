package node

import (
	"context"
	"errors"
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

// Collector collects node metrics
type Collector struct {
	*base.BaseCollector

	client   kubernetes.Interface
	config   *Config
	informer cache.SharedIndexInformer
	stopCh   chan struct{}
	logger   *log.Entry

	mu    sync.RWMutex
	nodes map[string]*corev1.Node

	// Metrics
	nodeCondition           *prometheus.Desc
	nodeInfo                *prometheus.Desc
	nodeResourceCapacity    *prometheus.Desc
	nodeResourceAllocatable *prometheus.Desc
	nodeTaints              *prometheus.Desc
	nodeAge                 *prometheus.Desc
}

// initMetrics initializes Prometheus metric descriptors
func (c *Collector) initMetrics(namespace string) {
	c.nodeCondition = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "node", "condition"),
		"Node condition status",
		[]string{"node", "condition", "status"},
		nil,
	)
	c.nodeInfo = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "node", "info"),
		"Node information",
		[]string{"node", "kernel_version", "kubelet_version", "container_runtime", "os_image"},
		nil,
	)
	c.nodeResourceCapacity = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "node", "resource_capacity"),
		"Node resource capacity",
		[]string{"node", "resource", "unit"},
		nil,
	)
	c.nodeResourceAllocatable = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "node", "resource_allocatable"),
		"Node resource allocatable",
		[]string{"node", "resource", "unit"},
		nil,
	)
	c.nodeTaints = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "node", "taints"),
		"Node taints",
		[]string{"node", "key", "value", "effect"},
		nil,
	)
	c.nodeAge = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "node", "age_seconds"),
		"Node age in seconds",
		[]string{"node"},
		nil,
	)

	// Register descriptors
	c.MustRegisterDesc(c.nodeCondition)
	c.MustRegisterDesc(c.nodeInfo)
	c.MustRegisterDesc(c.nodeResourceCapacity)
	c.MustRegisterDesc(c.nodeResourceAllocatable)
	c.MustRegisterDesc(c.nodeTaints)
	c.MustRegisterDesc(c.nodeAge)
}

// Start starts the collector
func (c *Collector) Start(ctx context.Context) error {
	if err := c.BaseCollector.Start(ctx); err != nil {
		return err
	}

	// Create informer factory
	factory := informers.NewSharedInformerFactory(c.client, c.config.IgnoreNewNodeDuration)

	// Create node informer
	c.informer = factory.Core().V1().Nodes().Informer()

	// Add event handlers
	//nolint:errcheck // AddEventHandler returns (registration, error) but error is always nil in client-go
	c.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			node := obj.(*corev1.Node) //nolint:errcheck // Type assertion is safe from informer

			c.mu.Lock()
			c.nodes[node.Name] = node.DeepCopy()
			c.mu.Unlock()
			c.logger.WithField("node", node.Name).Debug("Node added")
		},
		UpdateFunc: func(oldObj, newObj any) {
			node := newObj.(*corev1.Node) //nolint:errcheck // Type assertion is safe from informer

			c.mu.Lock()
			c.nodes[node.Name] = node.DeepCopy()
			c.mu.Unlock()
			c.logger.WithField("node", node.Name).Debug("Node updated")
		},
		DeleteFunc: func(obj any) {
			node := obj.(*corev1.Node) //nolint:errcheck // Type assertion is safe from informer

			c.mu.Lock()
			delete(c.nodes, node.Name)
			c.mu.Unlock()
			c.logger.WithField("node", node.Name).Debug("Node deleted")
		},
	})

	// Start informer
	factory.Start(c.stopCh)

	// Wait for cache sync
	c.logger.Info("Waiting for node informer cache sync")

	if !cache.WaitForCacheSync(c.stopCh, c.informer.HasSynced) {
		return errors.New("failed to sync node informer cache")
	}

	c.logger.Info("Node collector started successfully")

	return nil
}

// Stop stops the collector
func (c *Collector) Stop() error {
	close(c.stopCh)
	return c.BaseCollector.Stop()
}

// HasSynced returns true if the informer has synced
func (c *Collector) HasSynced() bool {
	return c.informer != nil && c.informer.HasSynced()
}

// collect collects metrics
func (c *Collector) collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	ignoreThreshold := now.Add(-c.config.IgnoreNewNodeDuration)

	for _, node := range c.nodes {
		// Skip new nodes if configured
		if node.CreationTimestamp.After(ignoreThreshold) {
			c.logger.WithFields(log.Fields{
				"node": node.Name,
				"age":  now.Sub(node.CreationTimestamp.Time),
			}).Debug("Skipping new node")

			continue
		}

		// Node conditions
		for _, condition := range node.Status.Conditions {
			status := "unknown"
			switch condition.Status {
			case corev1.ConditionTrue:
				status = "true"
			case corev1.ConditionFalse:
				status = "false"
			}

			ch <- prometheus.MustNewConstMetric(
				c.nodeCondition,
				prometheus.GaugeValue,
				boolToFloat64(condition.Status == corev1.ConditionTrue),
				node.Name,
				string(condition.Type),
				status,
			)
		}

		// Node info
		ch <- prometheus.MustNewConstMetric(
			c.nodeInfo,
			prometheus.GaugeValue,
			1,
			node.Name,
			node.Status.NodeInfo.KernelVersion,
			node.Status.NodeInfo.KubeletVersion,
			node.Status.NodeInfo.ContainerRuntimeVersion,
			node.Status.NodeInfo.OSImage,
		)

		// Node resources - capacity
		c.collectResources(ch, node, node.Status.Capacity, c.nodeResourceCapacity)

		// Node resources - allocatable
		c.collectResources(ch, node, node.Status.Allocatable, c.nodeResourceAllocatable)

		// Node taints
		if c.config.IncludeTaints {
			for _, taint := range node.Spec.Taints {
				ch <- prometheus.MustNewConstMetric(
					c.nodeTaints,
					prometheus.GaugeValue,
					1,
					node.Name,
					taint.Key,
					taint.Value,
					string(taint.Effect),
				)
			}
		}

		// Node age
		ch <- prometheus.MustNewConstMetric(
			c.nodeAge,
			prometheus.GaugeValue,
			now.Sub(node.CreationTimestamp.Time).Seconds(),
			node.Name,
		)
	}
}

// collectResources collects resource metrics
func (c *Collector) collectResources(
	ch chan<- prometheus.Metric,
	node *corev1.Node,
	resources corev1.ResourceList,
	desc *prometheus.Desc,
) {
	for resourceName, quantity := range resources {
		var (
			value float64
			unit  string
		)

		switch resourceName {
		case corev1.ResourceCPU:
			value = float64(quantity.MilliValue()) / 1000.0
			unit = "cores"
		case corev1.ResourceMemory:
			value = float64(quantity.Value())
			unit = "bytes"
		case corev1.ResourcePods:
			value = float64(quantity.Value())
			unit = "pods"
		case corev1.ResourceEphemeralStorage:
			value = float64(quantity.Value())
			unit = "bytes"
		default:
			// Handle other resources
			value = float64(quantity.Value())
			unit = string(resourceName)
		}

		ch <- prometheus.MustNewConstMetric(
			desc,
			prometheus.GaugeValue,
			value,
			node.Name,
			string(resourceName),
			unit,
		)
	}
}

// boolToFloat64 converts a boolean to a float64
func boolToFloat64(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}
