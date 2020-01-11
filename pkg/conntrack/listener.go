// Part of awesome https://github.com/mwitkow/go-conntrack library ported for custom registries and focused on metric only.

package conntrack

import (
	"net"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type listenerMetrics struct {
	acceptedTotal      prometheus.Counter
	failedAcceptsTotal prometheus.Counter
	closedTotal        prometheus.Counter
}

func NewListenerMetrics(reg prometheus.Registerer) *listenerMetrics {
	m := &listenerMetrics{
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
	metrics *listenerMetrics
}

// NewInstrumentedListener returns the given listener wrapped in connection listener exposing Promethues metric.
func NewInstrumentedListener(inner net.Listener, metrics *listenerMetrics) net.Listener {
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
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(5 * time.Minute)
	}
	ct.metrics.acceptedTotal.Inc()
	return newServerConnTracker(conn, ct.metrics.closedTotal), nil
}

type serverConnTracker struct {
	net.Conn
	closedTotal prometheus.Counter
	mu          sync.Mutex
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
