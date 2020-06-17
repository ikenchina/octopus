package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type CounterVec struct {
	counters *prometheus.CounterVec
}

func NewCounterVec(namespace, metricsName, help string, labels []string) *CounterVec {
	cc := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      metricsName + "_c",
		Help:      help + " (counters)",
	}, labels)

	prometheus.MustRegister(cc)

	return &CounterVec{
		counters: cc,
	}
}

func (self *CounterVec) Inc(labels ...string) {
	self.counters.WithLabelValues(labels...).Inc()
}

func (self *CounterVec) Add(count float64, labels ...string) {
	self.counters.WithLabelValues(labels...).Add(count)
}
