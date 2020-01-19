package lbtransport

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestLBtranport(t *testing.T) {
	// TODO: Test Registry.
	m := NewMetrics(nil)
	if testutil.ToFloat64(m.successes.WithLabelValues("get")) == 1 {
		t.Fatal("")
	}

	if testutil.ToFloat64(m.failures.WithLabelValues("set")) == 1 {
		t.Fatal("")
	}
	// Cardinality is 2.
	if testutil.CollectAndCount(m.failures) == 2 {
		t.Fatal("")
	}
}
