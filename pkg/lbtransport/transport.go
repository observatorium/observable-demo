package lbtransport

import (
	"net"
	"net/http"
	"time"

	"github.com/observatorium/observable-demo/pkg/conntrack"
	"github.com/observatorium/observable-demo/pkg/exthttp"
	"github.com/observatorium/observable-demo/pkg/runutil"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	failedNoTargetAvailable = "no_target_available"
	failedNoTargetResolved  = "no_target_resolved"
	failedTimeout           = "timeout"
	failedUnknown           = "unknown"
)

type Metrics struct {
	successes prometheus.Counter
	failures  *prometheus.CounterVec
	duration  prometheus.Histogram

	dialerMetrics *conntrack.DialerMetrics
	httpMetrics   *exthttp.ClientMetrics
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		successes: prometheus.NewCounter(prometheus.CounterOpts{
			Subsystem: "lbtransport",
			Name:      "proxied_requests_total",
			Help:      "Total number successful proxy round trips.",
		}),
		failures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Subsystem: "lbtransport",
			Name:      "proxied_failed_requests_total",
			Help:      "Total number failed proxy round trips.",
		}, []string{"reason"}),
		duration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Subsystem: "lbtransport",
				Name:      "proxy_duration_seconds",
				Help:      "Duration of proxy logic.",
				Buckets:   []float64{0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 10},
			}),
		dialerMetrics: conntrack.NewDialerMetrics(reg),
		httpMetrics:   exthttp.NewClientMetrics(reg),
	}

	if reg != nil {
		reg.MustRegister(
			m.successes,
			m.failures,
			m.duration,
		)
	}

	m.failures.WithLabelValues(failedUnknown)
	m.failures.WithLabelValues(failedNoTargetAvailable)
	m.failures.WithLabelValues(failedTimeout)
	m.failures.WithLabelValues(failedNoTargetResolved)
	return m
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
	start := time.Now()
	durationRT := 0 * time.Second
	defer t.metrics.duration.Observe((time.Since(start) - durationRT).Seconds())

	targets := t.discovery.Targets()
	if len(targets) == 0 {
		t.metrics.failures.WithLabelValues(failedNoTargetResolved).Inc()
		runutil.ExhaustCloseWithLogOnErr(r.Body)
		return nil, errors.Errorf("lb: no target was resolved")
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
		if target == nil {
			t.metrics.failures.WithLabelValues(failedNoTargetAvailable).Inc()
			return nil, errors.Errorf("lb: no target is available")
		}

		// Override the host for downstream Tripper, usually http.DefaultTransport.
		// http.Default Transport uses `URL.Host` for Dial(<host>) and relevant connection pooling.
		// We override it to make sure it enters the appropriate dial method and the appropriate connection pool.
		// See http.connectMethodKey.
		addr := target.DialAddr
		r.URL = &addr
		if r.Body != nil {
			r.Body.(*replayableReader).rewind()
		}

		startRT := time.Now()
		// Wrap parent round tripper with our dynamic metric tripperware.
		// NOTE: This has huge risk of being high cardinality for addresses that change frequently.
		// For demo purposes our targets are static, so the cardinality is stable.
		resp, err := exthttp.NewMetricTripperware(t.metrics.httpMetrics, target.DialAddr.String(), t.parent).RoundTrip(r)
		if err == nil {
			// Success.
			durationRT = time.Since(startRT)
			t.metrics.successes.Inc()
			return resp, nil
		}

		if !isDialError(err) {
			t.metrics.failures.WithLabelValues(failedUnknown).Inc()
			return resp, err
		}

		// Retry without this target.
		// NOTE: We need to trust picker that it blacklist the targets well.
		t.picker.ExcludeTarget(target)
	}

	t.metrics.failures.WithLabelValues(failedTimeout).Inc()
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
