// Part of awesome https://github.com/mwitkow/go-conntrack library ported for custom registries and focused on metric only.

package conntrack

import (
	"net"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type ListenerMetrics struct {
	acceptedTotal      prometheus.Counter
	failedAcceptsTotal prometheus.Counter
	closedTotal        prometheus.Counter
}

func NewListenerMetrics(reg prometheus.Registerer) *ListenerMetrics {
	m := &ListenerMetrics{
		acceptedTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Subsystem: "conntrack",
				Name:      "listener_conn_accepted_total",
				Help:      "Total number of connections opened to the listener.",
			}),
		failedAcceptsTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Subsystem: "conntrack",
				Name:      "listener_conn_failed_accepts_total",
				Help:      "Total number of failed accepts of the listener.",
			}),

		closedTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Subsystem: "conntrack",
				Name:      "listener_conn_closed_total",
				Help:      "Total number of connections closed that were made to the listener.",
			}),
	}

	if reg != nil {
		reg.MustRegister(m.acceptedTotal, m.closedTotal)
	}

	return m
}

type connTrackListener struct {
	net.Listener
	metrics *ListenerMetrics
}

// NewInstrumentedListener returns the given listener wrapped in connection listener exposing Prometheus metric.
func NewInstrumentedListener(inner net.Listener, metrics *ListenerMetrics) net.Listener {
	return &connTrackListener{
		Listener: inner,
		metrics:  metrics,
	}
}

func (ct *connTrackListener) Accept() (net.Conn, error) {
	var (
		conn net.Conn
		err  error
	)

	conn, err = ct.Listener.Accept()
	if err != nil {
		ct.metrics.failedAcceptsTotal.Inc()
		return nil, err
	}

	if tcpConn, ok := conn.(*net.TCPConn); ok {
		if err := tcpConn.SetKeepAlive(true); err != nil {
			return nil, err
		}

		if err := tcpConn.SetKeepAlivePeriod(5 * time.Minute); err != nil {
			return nil, err
		}
	}

	ct.metrics.acceptedTotal.Inc()

	return newServerConnTracker(conn, ct.metrics.closedTotal), nil
}

type serverConnTracker struct {
	net.Conn
	closedTotal prometheus.Counter
}

func newServerConnTracker(inner net.Conn, closedTotal prometheus.Counter) net.Conn {
	tracker := &serverConnTracker{
		Conn:        inner,
		closedTotal: closedTotal,
	}

	closedTotal.Inc()

	return tracker
}

func (st *serverConnTracker) Close() error {
	err := st.Conn.Close()
	st.closedTotal.Inc()

	return err
}
