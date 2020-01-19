package exthttp

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type ClientMetrics struct {
	requestsTotal    *prometheus.CounterVec
	requestsInFlight *prometheus.GaugeVec
	requestDuration  *prometheus.HistogramVec
}

// NewClientMetrics provides ClientMetrics.
func NewClientMetrics(reg prometheus.Registerer) *ClientMetrics {
	ins := &ClientMetrics{
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_client_requests_total",
				Help: "Tracks the number of HTTP client requests.",
			}, []string{"target", "code", "method"},
		),
		requestsInFlight: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "http_client_requests_in_flight",
				Help: "Tracks the number of client request currently in flight.",
			}, []string{"target"},
		),
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_client_request_duration_seconds",
				Help:    "Tracks the latencies for HTTP client requests.",
				Buckets: []float64{0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 90, 120},
			},
			[]string{"target", "code", "method"},
		),
	}
	if reg != nil {
		reg.MustRegister(ins.requestDuration, ins.requestsInFlight, ins.requestsTotal)
	}
	return ins
}

// TODO(bwplotka): Comment.
func NewMetricTripperware(metrics *ClientMetrics, target string, next http.RoundTripper) promhttp.RoundTripperFunc {
	return promhttp.InstrumentRoundTripperDuration(
		metrics.requestDuration.MustCurryWith(prometheus.Labels{"target": target}), promhttp.InstrumentRoundTripperCounter(
			metrics.requestsTotal.MustCurryWith(prometheus.Labels{"target": target}), promhttp.InstrumentRoundTripperInFlight(
				metrics.requestsInFlight.WithLabelValues(target), next,
			),
		),
	)
}
