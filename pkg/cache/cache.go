package cache

import (
	"errors"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	sets            prometheus.Counter
	gets            *prometheus.CounterVec
	storageFailures prometheus.Counter
	opsDuration     *prometheus.HistogramVec
}

func NewMetrics(reg prometheus.Registerer) *metrics {
	var m metrics

	m.sets = prometheus.NewCounter(prometheus.CounterOpts{
		Subsystem:   "cache",
		Name:        "operations_total",
		Help:        "Total number of cache sets.",
		ConstLabels: map[string]string{"operation": "set"},
	})
	m.gets = prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem:   "cache",
		Name:        "operations_total",
		Help:        "Total number of cache hits and misses.",
		ConstLabels: map[string]string{"operation": "get"},
	}, []string{"state"})
	m.storageFailures = prometheus.NewCounter(prometheus.CounterOpts{
		Subsystem: "cache",
		Name:      "storage_failures_total",
		Help:      "Total number of cache failures against storage.",
	})
	m.opsDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Subsystem: "cache",
		Name:      "operation_duration_seconds",
		Help:      "Time it took to perform cache operation.",
		Buckets:   []float64{0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 90, 120, 240, 360, 720},
	}, []string{"operation"})

	if reg != nil {
		reg.MustRegister(
			m.sets,
			m.gets,
			m.storageFailures,
			m.opsDuration,
		)
	}
	return &m
}

var NotFoundErr = errors.New("not found")

type Storage interface {
	Set(key, value string) error
	Get(key string) (string, error)
}

type Cache struct {
	storage Storage
	metrics *metrics
}

func NewCache(storage Storage, metrics *metrics) *Cache {
	return &Cache{
		storage: storage,
		metrics: metrics,
	}
}

func (c *Cache) SetHandler(w http.ResponseWriter, req *http.Request) {
	start := time.Now()
	defer func() { c.metrics.opsDuration.WithLabelValues("set").Observe(float64(time.Since(start))) }()

	if err := req.ParseForm(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	key := req.Form["key"]
	if len(key) != 1 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	value := req.Form["value"]
	if len(value) != 1 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := c.storage.Set(key[0], value[0]); err != nil {
		c.metrics.storageFailures.Inc()
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	c.metrics.sets.Inc()
}

func (c *Cache) GetHandler(w http.ResponseWriter, req *http.Request) {
	start := time.Now()
	defer func() { c.metrics.opsDuration.WithLabelValues("get").Observe(float64(time.Since(start))) }()

	if err := req.ParseForm(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	key := req.Form["key"]
	if len(key) != 1 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	value, err := c.storage.Get(key[0])
	if errors.Is(err, NotFoundErr) {
		c.metrics.gets.WithLabelValues("miss")
		w.Write([]byte(value))
		return
	}
	if err == nil {
		c.metrics.gets.WithLabelValues("hit")
		w.Write([]byte(value))
		return
	}

	c.metrics.storageFailures.Inc()
	w.WriteHeader(http.StatusInternalServerError)
	return
}
