package lbtransport

import (
	"net"
	"net/http"
	"time"

	"github.com/observatorium/observable-demo/pkg/conntrack"
	"github.com/observatorium/observable-demo/pkg/runutil"
	"github.com/pkg/errors"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	successes *prometheus.CounterVec
	failures  *prometheus.CounterVec
	duration  *prometheus.HistogramVec

	dialerMetrics *conntrack.DialerMetrics
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	var m Metrics

	m.successes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: "lbtransport",
		Name:      "rt_failures_total",
		Help:      "Total number of cache failures against storage.",
	}, []string{"operation"})
	m.failures = prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: "lbtransport",
		Name:      "rt_failures_total",
		Help:      "Total number of cache failures against storage.",
	}, []string{"address"})
	m.dialerMetrics = conntrack.NewDialerMetrics(reg)
	//m.duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	//	Subsystem: "lbtransport",
	//	Name:      "picking_duration_seconds",
	//	Help:      "Time it took to perform cache operation.",
	//	Buckets:   []float64{0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 90, 120, 240, 360, 720},
	//}, []string{"operation"}) // TODO: Failure duration vs success duration.
	// Not sure where duration can be useful here (:

	if reg != nil {
		reg.MustRegister(
			m.successes,
			m.failures,
			m.duration,
		)
	}

	return &m
}

type Transport struct {
	discovery Discovery
	picker    TargetPicker

	metrics *Metrics

	parent http.RoundTripper
}

func NewLoadBalancingTransport(discovery Discovery, picker TargetPicker, metrics *Metrics) *Transport {
	return &Transport{
		discovery: discovery,
		picker:    picker,
		metrics:   metrics,
		parent: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: conntrack.NewInstrumentedDialContextFunc(
				(&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 30 * time.Second,
					DualStack: false,
				}).DialContext,
				metrics.dialerMetrics),
			MaxIdleConns:          4,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}

func (t *Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	targets := t.discovery.Targets()
	if len(targets) == 0 {
		runutil.ExhaustCloseWithLogOnErr(r.Body)
		return nil, errors.Errorf("lb: no target is available")
	}

	if r.Body != nil {
		// We have to own the body for the request because we cannot reuse same reader closer
		// in multiple calls to http.Transport.
		body := r.Body
		defer runutil.ExhaustCloseWithLogOnErr(r.Body)
		r.Body = newReplayableReader(body)
	}

	for r.Context().Err() == nil {
		target := t.picker.Pick(targets)

		// Override the host for downstream Tripper, usually http.DefaultTransport.
		// http.Default Transport uses `URL.Host` for Dial(<host>) and relevant connection pooling.
		// We override it to make sure it enters the appropriate dial method and the appropriate connection pool.
		// See http.connectMethodKey.
		r.URL.Host = target.DialAddr
		if r.Body != nil {
			r.Body.(*replayableReader).rewind()
		}

		resp, err := t.parent.RoundTrip(r)
		if err == nil {
			// Success.
			t.metrics.successes.WithLabelValues(target.DialAddr).Inc()
			return resp, nil
		}

		if !isDialError(err) {
			return resp, err
		}

		t.metrics.failures.WithLabelValues(target.DialAddr).Inc()

		// Retry without this target.
		// NOTE: We need to trust picker that it blacklist the targets well.
		t.picker.ExcludeTarget(target)
	}

	return nil, r.Context().Err()
}

func isDialError(err error) bool {
	if opErr, ok := err.(*net.OpError); ok {
		if opErr.Op == "dial" {
			return true
		}
	}

	return false
}
