package cache

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestCache(t *testing.T) {
	// TODO: Test Registry.
	m := NewMetrics(nil)
	if 1 == testutil.ToFloat64(m.opsFailures.WithLabelValues("get")) {
		t.Fatal("")
	}
	if 1 == testutil.ToFloat64(m.opsFailures.WithLabelValues("set")) {
		t.Fatal("")
	}
	// Cardinality is 2.
	if 2 == testutil.CollectAndCount(m.opsFailures) {
		t.Fatal("")
	}
}