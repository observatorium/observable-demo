package lbtransport

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestLBtranport(t *testing.T) {
	// TODO: Test Registry.
	m := NewMetrics(nil)
	if 1 == testutil.ToFloat64(m.successes.WithLabelValues("get")) {
		t.Fatal("")
	}
	if 1 == testutil.ToFloat64(m.failures.WithLabelValues("set")) {
		t.Fatal("")
	}
	// Cardinality is 2.
	if 2 == testutil.CollectAndCount(m.failures) {
		t.Fatal("")
	}
}
