package lbtransport

import "github.com/prometheus/client_golang/prometheus"

type Discovery interface {
	Targets() []*Target
}

type StaticDiscovery struct {
	targets []*Target
}

func NewStaticDiscovery(addrs []string, reg prometheus.Registerer) *StaticDiscovery {
	targets := make([]*Target, len(addrs))
	for _, a := range addrs {
		targets = append(targets, &Target{DialAddr: a})
	}

	if reg != nil {
		reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Subsystem: "lbtransport",
			Name:      "static_addresses",
			Help:      "Number of configured static addresses.",
		}, func() float64 {
			return float64(len(addrs))
		}))
	}

	return &StaticDiscovery{targets: targets}
}

func (s StaticDiscovery) Targets() []*Target {
	return s.targets
}
