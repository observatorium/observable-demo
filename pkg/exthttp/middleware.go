package exthttp

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type ServerMetrics struct {
	requestsTotal    *prometheus.CounterVec
	requestsInFlight prometheus.Gauge
	requestDuration  *prometheus.HistogramVec
	requestSize      *prometheus.SummaryVec
	responseSize     *prometheus.SummaryVec
}

// NewServerMetrics provides ServerMetrics.
func NewServerMetrics(reg prometheus.Registerer) *ServerMetrics {
	ins := &ServerMetrics{
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Tracks the number of HTTP requests.",
			}, []string{"code", "method"},
		),
		requestsInFlight: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "http_requests_in_flight",
				Help: "Tracks the number of HTTP request currently in flight.",
			},
		),
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "Tracks the latencies for HTTP requests.",
				Buckets: []float64{0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 90, 120},
			},
			[]string{"code", "method"},
		),
		requestSize: prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Name: "http_request_size_bytes",
				Help: "Tracks the size of HTTP requests.",
			},
			[]string{"code", "method"},
		),
		responseSize: prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Name: "http_response_size_bytes",
				Help: "Tracks the size of HTTP responses.",
			},
			[]string{"code", "method"},
		),
	}
	reg.MustRegister(ins.requestDuration, ins.requestSize, ins.requestsTotal, ins.requestsInFlight, ins.responseSize)
	return ins
}

func newMetricMiddleware(metrics *ServerMetrics, handler http.Handler) http.HandlerFunc {
	return promhttp.InstrumentHandlerDuration(
		metrics.requestDuration, promhttp.InstrumentHandlerRequestSize(
			metrics.requestSize, promhttp.InstrumentHandlerCounter(
				metrics.requestsTotal, promhttp.InstrumentHandlerInFlight(
					metrics.requestsInFlight, promhttp.InstrumentHandlerResponseSize(
						metrics.responseSize, handler,
					),
				),
			),
		),
	)
}

// NewHandler wraps the given HTTP handler for instrumentation. It
// registers four metric collectors (if not already done) and reports HTTP
// metrics to the (newly or already) registered collectors: http_requests_total
// (CounterVec), http_request_duration_seconds (Histogram),
// http_request_size_bytes (Summary), http_response_size_bytes (Summary). Each
// has a constant label named "handler" with the provided handlerName as
// value. http_requests_total is a metric vector partitioned by HTTP method
// (label name "method") and HTTP status code (label name "code").
func NewMetricsMiddlewareHandler(reg prometheus.Registerer, handlerName string, handler http.Handler) http.HandlerFunc {
	return newMetricMiddleware(
		NewServerMetrics(prometheus.WrapRegistererWith(prometheus.Labels{"handler": handlerName}, reg)),
		handler,
	)
}
