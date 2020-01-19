package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/observatorium/observable-demo/pkg/conntrack"
	"github.com/observatorium/observable-demo/pkg/exthttp"
	"github.com/observatorium/observable-demo/pkg/lbtransport"
	"github.com/observatorium/observable-demo/pkg/lbtransport/lbutils"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
)

func main() {
	var (
		addr             = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
		targets          = flag.String("targets", "", "Comma-separated URLs for target to load balance to.")
		blacklistBackoff = flag.Duration("failed_target_backoff_duration", 5*time.Second, "Backoff duration in case of dial error for given backend.")

		demo1Addr = flag.String("listen-demo1-address", ":8081", "The demo1 address to listen on for HTTP requests.")
		demo2Addr = flag.String("listen-demo2-address", ":8082", "The demo2 address to listen on for HTTP requests.")
		demo3Addr = flag.String("listen-demo3-address", ":8083", "The demo3 address to listen on for HTTP requests.")

		g = &run.Group{}

		// Start our metric registry.
		reg = prometheus.NewRegistry()

		ctx = context.Background()
	)

	flag.Parse()

	// Register standard Go metric collectors, which are by default registered when using global registry.
	reg.MustRegister(
		version.NewCollector(""),
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "configured_failed_target_backoff_duration_seconds",
			Help: "Configured backoff time for unavailable target.",
		}, func() float64 { return blacklistBackoff.Seconds() }), // nolint: gocritic
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

	var targetURLs []url.URL
	for _, addr := range strings.Split(*targets, ",") {
		u, err := url.Parse(addr)
		if err != nil {
			log.Fatalf("failed to parse target %v; err: %v", addr, err)
		}
		targetURLs = append(targetURLs, *u)
	}
	// Server listen for loadbalancer.
	{
		mux := http.NewServeMux()

		static := lbtransport.NewStaticDiscovery(targetURLs, reg)
		picker := lbtransport.NewRoundRobinPicker(ctx, reg, *blacklistBackoff)
		l7LoadBalancer := &httputil.ReverseProxy{
			Director:       func(request *http.Request) {},
			ModifyResponse: func(response *http.Response) error { return nil },
			Transport:      lbtransport.NewLoadBalancingTransport(static, picker, lbtransport.NewMetrics(reg)),
		}

		mux.Handle("/metrics", exthttp.NewMetricsMiddlewareHandler(
			reg, "/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
		))
		mux.Handle("/lb", exthttp.NewMetricsMiddlewareHandler(reg, "/lb", l7LoadBalancer))

		srv := &http.Server{Handler: mux}

		l, err := net.Listen("tcp", *addr)
		if err != nil {
			log.Fatalf("new listener failed %v; exiting\n", err)
		}
		g.Add(func() error {
			return srv.Serve(
				conntrack.NewInstrumentedListener(
					l,
					conntrack.NewListenerMetrics(
						prometheus.WrapRegistererWith(prometheus.Labels{"listener": "lb"}, reg),
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
	// For demo purposes.
	lbutils.CreateDemoEndpoints(reg, g, *demo1Addr, *demo2Addr, *demo3Addr)

	log.Printf("Starting loadbalancer for targets: %v/n", targetURLs)
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
