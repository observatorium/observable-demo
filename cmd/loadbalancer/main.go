package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"

	"github.com/observatorium/observable-demo/pkg/conntrack"
	extpromhttp "github.com/observatorium/observable-demo/pkg/extprom/http"
	"github.com/observatorium/observable-demo/pkg/lbtransport"
)

const namespace = "loadbalancer"

// TODO: Add global metric examples

//nolint: funlen
func main() {
	var (
		addr             = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
		targets          = flag.String("targets", "", "Comma-separated URLs for target to load balance to.")
		blacklistBackoff = flag.Duration("failed_target_backoff_duration", 2*time.Second, "Backoff duration in case of dial error for given backend")

		g = &run.Group{}

		// Start our metric registry.
		reg = prometheus.NewRegistry()

		ctx = context.Background()
	)

	flag.Parse()

	// Register standard Go metric collectors, which are by default registered when using global registry.
	reg.MustRegister(
		version.NewCollector("observable-demo"),
		prometheus.NewBuildInfoCollector(),
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "configured_failed_target_backoff_duration_seconds",
			Help: "Configured backoff time for unavailable target.",
		}, func() float64 { return blacklistBackoff.Seconds() }), // nolint: gocritic
	)

	// TODO: Tripperware metrics

	// Listen for termination signals.
	{
		cancel := make(chan struct{})
		g.Add(func() error {
			return interrupt(cancel)
		}, func(error) {
			close(cancel)
		})
	}
	// Server listen for loadbalancer.
	{
		mux := http.NewServeMux()

		static := lbtransport.NewStaticDiscovery(strings.Split(*targets, ","), reg)
		picker := lbtransport.NewRoundRobinPicker(ctx, *blacklistBackoff)
		l7LoadBalancer := &httputil.ReverseProxy{
			Transport: lbtransport.NewLoadBalancingTransport(static, picker, lbtransport.NewMetrics(reg)),
		}

		ins := extpromhttp.NewInstrumentationMiddleware(reg)
		mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
		mux.Handle("/lb", ins.NewHandler("lb", l7LoadBalancer))

		srv := &http.Server{Addr: *addr, Handler: mux}

		l, err := net.Listen("tcp", *addr)
		if err != nil {
			log.Fatalf("new listener failed %v; exiting\n", err)
		}
		g.Add(func() error {
			return srv.Serve(
				conntrack.NewInstrumentedListener(l, conntrack.NewListenerMetrics(
					prometheus.WrapRegistererWithPrefix(namespace, reg),
				),
				),
			)
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
