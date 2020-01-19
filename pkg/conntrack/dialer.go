package conntrack

import (
	"context"
	"net"
	"os"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	failedResolution  = "resolution"
	failedConnRefused = "refused"
	failedTimeout     = "timeout"
	failedUnknown     = "unknown"
)

type dialerContextFunc func(context.Context, string, string) (net.Conn, error)

type DialerMetrics struct {
	attemptedTotal       prometheus.Counter
	connEstablishedTotal prometheus.Counter
	connFailedTotal      *prometheus.CounterVec
	connClosedTotal      prometheus.Counter
}

func NewDialerMetrics(reg prometheus.Registerer) *DialerMetrics {
	m := &DialerMetrics{
		attemptedTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Subsystem: "conntrack",
				Name:      "dialer_conn_attempted_total",
				Help:      "Total number of connections attempted by the dialer.",
			}),

		connEstablishedTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Subsystem: "conntrack",
				Name:      "dialer_conn_established_total",
				Help:      "Total number of connections successfully established by the dialer.",
			}),

		connFailedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: "conntrack",
				Name:      "dialer_conn_failed_total",
				Help:      "Total number of connections failed to dial by the dialer.",
			}, []string{"reason"}),

		connClosedTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Subsystem: "conntrack",
				Name:      "dialer_conn_closed_total",
				Help:      "Total number of connections closed which originated from dialer.",
			}),
	}

	if reg != nil {
		reg.MustRegister(m.attemptedTotal, m.connClosedTotal, m.connEstablishedTotal, m.connFailedTotal)
	}

	m.connFailedTotal.WithLabelValues(failedResolution)
	m.connFailedTotal.WithLabelValues(failedConnRefused)
	m.connFailedTotal.WithLabelValues(failedTimeout)
	m.connFailedTotal.WithLabelValues(failedUnknown)

	return m
}

// NewDialContextFunc returns a `DialContext` function that tracks outbound connections.
// The signature is compatible with `http.Tranport.DialContext` and is meant to be used there.
func NewInstrumentedDialContextFunc(parentDialContextFunc dialerContextFunc, metrics *DialerMetrics) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, ntk string, addr string) (net.Conn, error) {
		return dialClientConnTracker(ctx, ntk, addr, metrics, parentDialContextFunc)
	}
}

type clientConnTracker struct {
	net.Conn
	connClosedTotal prometheus.Counter
}

func dialClientConnTracker(ctx context.Context, ntk string, addr string, metrics *DialerMetrics, parentDialContextFunc dialerContextFunc) (net.Conn, error) {
	metrics.attemptedTotal.Inc()

	conn, err := parentDialContextFunc(ctx, ntk, addr)
	if err != nil {
		metrics.connFailedTotal.WithLabelValues(dialErrToReason(err)).Inc()
		return conn, err
	}

	metrics.connEstablishedTotal.Inc()

	return &clientConnTracker{
		Conn:            conn,
		connClosedTotal: metrics.connClosedTotal,
	}, nil
}

func dialErrToReason(err error) string {
	if netErr, ok := err.(*net.OpError); ok {
		switch nestErr := netErr.Err.(type) {
		case *net.DNSError:
			return failedResolution
		case *os.SyscallError:
			if nestErr.Err == syscall.ECONNREFUSED {
				return failedConnRefused
			}

			return failedUnknown
		}

		if netErr.Timeout() {
			return failedTimeout
		}
	} else if err == context.Canceled || err == context.DeadlineExceeded {
		return failedTimeout
	}

	return failedUnknown
}

func (ct *clientConnTracker) Close() error {
	err := ct.Conn.Close()
	ct.connClosedTotal.Inc()

	return err
}
