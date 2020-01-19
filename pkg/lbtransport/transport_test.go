package lbtransport

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"syscall"
	"testing"

	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/prometheus/util/testutil"
)

type response struct {
	host string

	*http.Response
	err error
}
type mockedTransport struct {
	t         *testing.T
	cnt       int
	responses []response
}

func (f *mockedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	defer func() { f.cnt++ }()

	testutil.Equals(f.t, f.responses[f.cnt].host, r.URL.Host)
	return f.responses[f.cnt].Response, f.responses[f.cnt].err
}

func (f *mockedTransport) Reset(res []response) {
	f.cnt = 0
	f.responses = res
}

type mockedDiscovery struct {
	targets []string
}

func (f *mockedDiscovery) Targets() []*Target {
	targets := make([]*Target, 0, len(f.targets))
	for _, target := range f.targets {
		targets = append(targets, &Target{DialAddr: url.URL{Host: target}})
	}
	return targets
}

func (f *mockedDiscovery) Reset(targets []string) {
	f.targets = targets
}

type mockedPicker struct {
	cnt             int
	toPick          []response
	lastSeenTargets []*Target
	excluded        []string
}

func (f *mockedPicker) Pick(targets []*Target) *Target {
	defer func() { f.cnt++ }()

	f.lastSeenTargets = targets
	if len(f.toPick) <= f.cnt {
		return nil
	}
	return &Target{DialAddr: url.URL{Host: f.toPick[f.cnt].host}}
}

func (f *mockedPicker) ExcludeTarget(t *Target) {
	f.excluded = append(f.excluded, t.DialAddr.Host)
}

func (f *mockedPicker) Reset(res []response) {
	f.cnt = 0
	f.toPick = res
	f.lastSeenTargets = []*Target{}
	f.excluded = nil
}

func okResponse(host string) response {
	return response{host: host, Response: &http.Response{Request: &http.Request{URL: &url.URL{Host: host}}}}
}

func TestLoadBalancingTranport(t *testing.T) {
	metrics := NewMetrics(nil)
	discovery := &mockedDiscovery{}
	picker := &mockedPicker{}
	transport := &mockedTransport{t: t}

	lb := &Transport{
		discovery: discovery,
		picker:    picker,
		metrics:   metrics,
		parent:    transport,
	}
	// All reasons are initialised.
	testutil.Equals(t, 4, promtestutil.CollectAndCount(lb.metrics.failures))

	for _, tcase := range []struct {
		targets   []string
		responses []response
		excluded  []string

		expectedHost string
		expectedErr  error

		failedNoTargetAvailable float64
		failedNoTargetResolved  float64
		failedUnknown           float64
		successes               float64
	}{
		{
			targets:      []string{"a"},
			responses:    []response{okResponse("a")},
			expectedHost: "a",

			successes: 1,
		},
		{
			targets:      []string{"a"},
			responses:    []response{okResponse("a")},
			expectedHost: "a",

			successes: 2,
		},
		{
			targets: []string{"a"},
			responses: []response{
				{host: "a", err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}},
			},
			excluded:    []string{"a"},
			expectedErr: errors.New("lb: no target is available"),

			successes: 2, failedNoTargetAvailable: 1,
		},
		{
			targets:     []string{},
			expectedErr: errors.New("lb: no target was resolved"),

			successes: 2, failedNoTargetAvailable: 1, failedNoTargetResolved: 1,
		},
		{
			targets: []string{"a", "b", "c", "d", "e", "f", "g"},
			responses: []response{
				{host: "a", err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}},
				{host: "b", err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}},
				{host: "c", err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}},
				{host: "d", err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}},
				{host: "e", err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}},
				{host: "f", err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}},
				okResponse("g"),
			},
			excluded:     []string{"a", "b", "c", "d", "e", "f"},
			expectedHost: "g",

			successes: 3, failedNoTargetAvailable: 1, failedNoTargetResolved: 1,
		},
		{
			targets: []string{"a"},
			responses: []response{
				{host: "a", err: errors.New("test")},
			},
			expectedErr: errors.New("test"),

			successes: 3, failedNoTargetAvailable: 1, failedNoTargetResolved: 1, failedUnknown: 1,
		},
	} {
		if ok := t.Run("", func(t *testing.T) {
			discovery.Reset(tcase.targets)
			transport.Reset(tcase.responses)
			picker.Reset(tcase.responses)

			resp, err := lb.RoundTrip(httptest.NewRequest("GET", "http://whatever", nil))
			if tcase.expectedErr != nil {
				testutil.NotOk(t, err)
				testutil.Equals(t, tcase.expectedErr.Error(), err.Error())
			} else {
				testutil.Ok(t, err)
				testutil.Equals(t, tcase.expectedHost, resp.Request.URL.Host)
			}
			testutil.Equals(t, tcase.excluded, picker.excluded)
			testutil.Equals(t, discovery.Targets(), picker.lastSeenTargets)

			testutil.Equals(t, tcase.successes, promtestutil.ToFloat64(metrics.successes))
			testutil.Equals(t, tcase.failedNoTargetAvailable, promtestutil.ToFloat64(metrics.failures.WithLabelValues(failedNoTargetAvailable)))
			testutil.Equals(t, tcase.failedNoTargetResolved, promtestutil.ToFloat64(metrics.failures.WithLabelValues(failedNoTargetResolved)))
			testutil.Equals(t, tcase.failedUnknown, promtestutil.ToFloat64(metrics.failures.WithLabelValues(failedUnknown)))
			testutil.Equals(t, float64(0), promtestutil.ToFloat64(metrics.failures.WithLabelValues(failedTimeout)))
			testutil.Equals(t, 4, promtestutil.CollectAndCount(lb.metrics.failures))
		}); !ok {
			return
		}
	}
}

func TestLoadBalancingTransport_Timeout(t *testing.T) {
	metrics := NewMetrics(nil)
	discovery := &mockedDiscovery{targets: []string{"a", "b"}}

	lb := &Transport{
		discovery: discovery,
		metrics:   metrics,
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := lb.RoundTrip(httptest.NewRequest("GET", "http://whatever", nil).WithContext(cancelled))
	testutil.NotOk(t, err)
	testutil.Equals(t, cancelled.Err().Error(), err.Error())

	testutil.Equals(t, float64(0), promtestutil.ToFloat64(metrics.successes))
	testutil.Equals(t, float64(0), promtestutil.ToFloat64(metrics.failures.WithLabelValues(failedNoTargetAvailable)))
	testutil.Equals(t, float64(0), promtestutil.ToFloat64(metrics.failures.WithLabelValues(failedNoTargetResolved)))
	testutil.Equals(t, float64(0), promtestutil.ToFloat64(metrics.failures.WithLabelValues(failedUnknown)))
	testutil.Equals(t, float64(1), promtestutil.ToFloat64(metrics.failures.WithLabelValues(failedTimeout)))
	testutil.Equals(t, 4, promtestutil.CollectAndCount(lb.metrics.failures))
}
