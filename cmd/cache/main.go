package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/observatorium/observable-demo/pkg/cache"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "obs_demo"

func main() {
	var (
		addr            = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
		maxObjectsCount = flag.Int("max-objects", 1000, "Maximum number of objects cache can store.")

		g = &run.Group{}

		// Start command's metric registry.
		reg = prometheus.NewRegistry()
	)
	flag.Parse()

	// Register standard Go metric collectors, which are by default registered when using global registry.
	reg.MustRegister(
		// TODO: version!
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "configured_max_objects",
			Help:      "Configured maximum number of objects cache can take.",
		}, func() float64 { return float64(*maxObjectsCount) }),
	)

	// TODO: Tripperware when creating storage ~ local mocked memcached

	// Setup cache.
	c := cache.NewCache(
		nil,
		cache.NewMetrics(prometheus.WrapRegistererWithPrefix(namespace,reg)),
	)

	// Listen for termination signals.
	{
		cancel := make(chan struct{})
		g.Add(func() error {
			return interrupt(cancel)
		}, func(error) {
			close(cancel)
		})
	}
	// Server listen.
	{
		mux := http.NewServeMux()

		// TODO: Add http middleware: request{code, handler, method}
		// Shared metric - we can share dashboards, alerts.
		mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
		mux.Handle("/set", http.HandlerFunc(c.SetHandler))
		mux.Handle("/get", http.HandlerFunc(c.GetHandler))

		srv := &http.Server{Addr: *addr, Handler: mux}

		g.Add(func() error {
			return srv.ListenAndServe()
		}, func(error) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := srv.Shutdown(ctx); err != nil {
				log.Println("error: server shutdown failed")
			}
		})
	}

	if err := g.Run(); err != nil {
		log.Fatalf("running command failed %v; exiting\n", err)
	}
	log.Println("exiting")
}

func interrupt(cancel <-chan struct{}) error {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	select {
	case s := <-c:
		log.Printf("caught signal %s. Exiting.\n", s)
		return nil
	case <-cancel:
		return errors.New("canceled")
	}
}
