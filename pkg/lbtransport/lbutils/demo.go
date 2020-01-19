package lbutils

import (
	"context"
	"log"
	"net"
	"net/http"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/observatorium/observable-demo/pkg/conntrack"
	"github.com/observatorium/observable-demo/pkg/exthttp"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
)

func CreateDemoEndpoints(reg prometheus.Registerer, g *run.Group, addr1, addr2, addr3 string) {
	{
		const name = "demo-ok"

		srv := &http.Server{Handler: exthttp.NewMetricsMiddlewareHandler(reg, name, okTestEndpoint(addr1))}
		l, err := net.Listen("tcp", addr1)
		if err != nil {
			log.Fatalf("new demo1 listener failed %v; exiting\n", err)
		}
		g.Add(func() error {
			return srv.Serve(conntrack.NewInstrumentedListener(
				l,
				conntrack.NewListenerMetrics(
					prometheus.WrapRegistererWith(prometheus.Labels{"listener": name}, reg),
				),
			))
		}, func(error) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := srv.Shutdown(ctx); err != nil {
				log.Println("error: server demo1 shutdown failed")
			}
		})
	}
	{
		const name = "demo-500-sometimes"

		srv := &http.Server{Handler: exthttp.NewMetricsMiddlewareHandler(reg, name, flakyTestEndpoint(addr2))}
		l, err := net.Listen("tcp", addr2)
		if err != nil {
			log.Fatalf("new demo2 listener failed %v; exiting\n", err)
		}
		g.Add(func() error {
			return srv.Serve(conntrack.NewInstrumentedListener(
				l,
				conntrack.NewListenerMetrics(
					prometheus.WrapRegistererWith(prometheus.Labels{"listener": name}, reg),
				),
			))
		}, func(error) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := srv.Shutdown(ctx); err != nil {
				log.Println("error: server demo2 shutdown failed")
			}
		})
	}
	{
		const name = "demo-refused-conn-sometimes"

		handler := exthttp.NewMetricsMiddlewareHandler(reg, name, okTestEndpoint(addr3))
		srv := &http.Server{
			// This ensures no keep-alive and thus always starting new connection for demo purposes.
			IdleTimeout: 1 * time.Nanosecond,
			Handler:     handler,
		}
		ctx, cancel := context.WithCancel(context.Background())
		g.Add(func() error {
			m := conntrack.NewListenerMetrics(
				prometheus.WrapRegistererWith(prometheus.Labels{"listener": name}, reg),
			)

			for ctx.Err() == nil {
				l, err := newFlakyTCPListener(addr3)
				if err != nil {
					return err
				}

				_ = srv.Serve(conntrack.NewInstrumentedListener(l, m))
				if ctx.Err() != nil {
					break
				}
				if err := srv.Shutdown(ctx); err != nil {
					return err
				}
				srv = &http.Server{
					// This ensures no keep-alive and thus always starting new connection for demo purposes.
					IdleTimeout: 1 * time.Nanosecond,
					Handler:     handler,
				}
				time.Sleep(2 * time.Second)
			}
			return ctx.Err()
		}, func(error) {
			cancel()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := srv.Shutdown(ctx); err != nil {
				log.Println("error: server demo3 shutdown failed")
			}
		})
	}
}

func okTestEndpoint(addr string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(addr + " says hello! (:"))
		return
	}
}

func flakyTestEndpoint(addr string) http.HandlerFunc {
	var counter uint64
	return func(w http.ResponseWriter, req *http.Request) {
		cur := atomic.AddUint64(&counter, 1)
		if cur%10 == 0 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(addr + " says I have issues... ):"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(addr + " says hello! (:"))
		return
	}
}

type flakyListener struct {
	net.Listener

	counter uint64
}

func newFlakyTCPListener(address string) (*flakyListener, error) {
	l, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	return &flakyListener{Listener: l}, nil
}

// Set max connection to 0 to always hit this.
func (f *flakyListener) Accept() (net.Conn, error) {
	cur := atomic.AddUint64(&f.counter, 1)
	if cur%5 == 0 {
		return nil, syscall.ECONNREFUSED
	}
	return f.Listener.Accept()

}

func (f *flakyListener) Close() error {
	return f.Listener.Close()
}
