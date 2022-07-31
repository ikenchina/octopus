package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type GaugeVec struct {
	gauges *prometheus.GaugeVec
}

func NewGaugeVec(namespace, subsystem, metricsName, help string, labels []string) *GaugeVec {
	cc := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      metricsName + "_g",
		Help:      help + " (gauges)",
	}, labels)

	prometheus.MustRegister(cc)

	return &GaugeVec{
		gauges: cc,
	}
}

func (self *GaugeVec) Inc(labels ...string) {
	self.gauges.WithLabelValues(labels...).Inc()
}

func (self *GaugeVec) Add(v float64, labels ...string) {
	self.gauges.WithLabelValues(labels...).Add(v)
}

func (self *GaugeVec) Dec(labels ...string) {
	self.gauges.WithLabelValues(labels...).Dec()
}

func (self *GaugeVec) Sub(v float64, labels ...string) {
	self.gauges.WithLabelValues(labels...).Sub(v)
}

func (self *GaugeVec) Set(v float64, labels ...string) {
	self.gauges.WithLabelValues(labels...).Set(v)
}
