# observable-demo

[![semver](https://img.shields.io/badge/semver--0.0.0-blue.svg?cacheSeconds=2592000)](https://github.com/observatorium/observable-demo/releases) [![Maintenance](https://img.shields.io/maintenance/yes/2020.svg)](https://github.com/observatorium/observable-demo/commits/master) [![Build Status](https://cloud.drone.io/api/badges/observatorium/observable-demo/status.svg)](https://cloud.drone.io/observatorium/observable-demo)[![Go Doc](https://godoc.org/github.com/observatorium/observable-demo?status.svg)](http://godoc.org/github.com/observatorium/observable-demo) [![Go Report Card](https://goreportcard.com/badge/github.com/observatorium/observable-demo)](https://goreportcard.com/report/github.com/observatorium/observable-demo)

This repository includes simple L7 round robin loadbalancer implementation, instrumented with metrics.

> `WARNING` : This is not meant to be production ready L7 loadbalancer. It's missing things like proper retrying, DNS discovery, logging, tracing etc, that might be added in future (:

<img src="https://docs.google.com/drawings/d/e/2PACX-1vSp_CcK4aH5w9uUHUHOnWmtaLdxr6zet5UO6dVN2jXhag0x8UMPEn5sy3acF8B4QnJVRyOVBknp5Eij/pub?w=1440&amp;h=1080">

This implementation was **mainly** used for presentations that showcase best practices for Prometheus based metrics Go instrumentation like:

* [Avoiding globals](https://github.com/observatorium/observable-demo/blob/e89e67a9e3e564d1ab3cb716835c8969f4bc1201/pkg/lbtransport/transport.go#L31).
* [Middlewares](https://github.com/observatorium/observable-demo/blob/e89e67a9e3e564d1ab3cb716835c8969f4bc1201/pkg/exthttp/middleware.go#L82) and [Tripperwares](https://github.com/observatorium/observable-demo/blob/d9cf53a171f4b016512337faa3e36411830525df/pkg/exthttp/tripperware.go#L46).
* Histograms [usage](https://github.com/observatorium/observable-demo/blob/e89e67a9e3e564d1ab3cb716835c8969f4bc1201/pkg/lbtransport/transport.go#L43).
* [Avoiding high cardinality](https://github.com/observatorium/observable-demo/blob/e89e67a9e3e564d1ab3cb716835c8969f4bc1201/pkg/lbtransport/transport.go#L42).
* [Initialising metrics](https://github.com/observatorium/observable-demo/blob/e89e67a9e3e564d1ab3cb716835c8969f4bc1201/pkg/lbtransport/transport.go#L62).
* [Wrapping registries](https://github.com/observatorium/observable-demo/blob/e89e67a9e3e564d1ab3cb716835c8969f4bc1201/pkg/exthttp/middleware.go#L84).
* Naming conventions
* [Testing metrics](https://github.com/observatorium/observable-demo/blob/d9cf53a171f4b016512337faa3e36411830525df/pkg/lbtransport/transport_test.go#L189)

## Using demo

This demo is runable.

To run loadbalancer and Prometheus in docker run:

``` console
make demo
```

After this, you should be able to go via browser to:

* `http://localhost:8080/lb` for load balancing endpoint, that loadbalanced to 3 fake endpoints.
* `http://localhost:8080/metrics` for metric page
* `http://localhost:9090` for Prometheus UI that scrapes loadbalancer every second.

To curl loadbalancer every 500ms to generate traffic run in separate terminal:

``` console
make demo-test
```

