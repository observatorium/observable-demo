package lbtransport

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/fortytw2/leaktest"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/thanos-io/thanos/pkg/testutil"
)

func TestRoundRobinPicker(t *testing.T) {
	defer leaktest.Check(t)

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	currTime := time.Now()
	rr := NewRoundRobinPicker(cancelledCtx, nil, 2*time.Second)
	rr.timeNow = func() time.Time {
		return currTime
	}

	targets := []*Target{
		{DialAddr: url.URL{Host: "a"}},
		{DialAddr: url.URL{Host: "b"}},
		{DialAddr: url.URL{Host: "c"}},
	}
	for _, tcase := range []struct {
		exclude     int
		expected    int
		blacklisted int

		preClean bool
		addTime  time.Duration
	}{
		{exclude: -1, expected: 1},
		{exclude: -1, expected: 2},
		{exclude: -1, expected: 0},
		{exclude: -1, expected: 1},
		{exclude: 2, expected: 0, blacklisted: 1},
		{exclude: -1, expected: 1, blacklisted: 1},
		{exclude: -1, expected: 0, blacklisted: 1},
		{exclude: -1, expected: 1, blacklisted: 1},
		{exclude: -1, expected: 2, blacklisted: 1, addTime: 3 * time.Second},
		{exclude: -1, expected: 0, blacklisted: 1},
		{exclude: 1, expected: 2, blacklisted: 2},
		{exclude: -1, expected: 0, blacklisted: 2},
		{preClean: true, exclude: -1, expected: 2, blacklisted: 1},
		{exclude: -1, expected: 0, blacklisted: 1},
		{preClean: true, exclude: -1, expected: 1, blacklisted: 0, addTime: 3 * time.Second},
	} {
		if ok := t.Run("", func(t *testing.T) {
			currTime = currTime.Add(tcase.addTime)

			if tcase.preClean {
				rr.cleanUpBlacklist()
			}

			if tcase.exclude > 0 {
				rr.ExcludeTarget(targets[tcase.exclude])
			}
			testutil.Equals(t, targets[tcase.expected], rr.Pick(targets))
			testutil.Equals(t, float64(tcase.blacklisted), promtestutil.ToFloat64(rr.backlistedTargetsNum))
		}); !ok {
			return
		}
	}
}
