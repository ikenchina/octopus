package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// NewTimer ...
type timerOptions struct {
	buckets []float64
	labels  map[string]string
}

type TimerOption func(*timerOptions)

func WithTimerBuckets(buk []float64) TimerOption {
	return func(o *timerOptions) {
		o.buckets = buk
	}
}

func WithTimerConstLabels(lables map[string]string) TimerOption {
	return func(o *timerOptions) {
		o.labels = lables
	}
}

// NewTimer
func NewTimer(namespace, metricName, help string, labels []string, opts ...TimerOption) *Timer {

	// @todo remove summary
	summary := prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  namespace,
			Name:       metricName + "_s",
			Help:       help + " (summary)",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		labels)

	prometheus.MustRegister(summary)

	// histogram
	timerOpts := timerOptions{}
	for _, opt := range opts {
		opt(&timerOpts)
	}
	hisOpts := prometheus.HistogramOpts{
		Namespace:   namespace,
		Name:        metricName + "_h",
		Help:        help + " (histogram)",
		Buckets:     timerOpts.buckets,
		ConstLabels: timerOpts.labels,
	}

	histogram := prometheus.NewHistogramVec(hisOpts, labels)

	prometheus.MustRegister(histogram)
	return &Timer{
		name:      metricName,
		summary:   summary,
		histogram: histogram,
	}
}

type Timer struct {
	name      string
	summary   *prometheus.SummaryVec
	histogram *prometheus.HistogramVec
}

// Timer 返回一个函数，并且开始计时，结束计时则调用返回的函数
// 请参考timer_test.go 的demo
func (t *Timer) Timer() func(values ...string) {
	if t == nil {
		return func(values ...string) {}
	}

	now := time.Now()

	return func(values ...string) {
		seconds := float64(time.Since(now)) / float64(time.Second)
		t.summary.WithLabelValues(values...).Observe(seconds)
		t.histogram.WithLabelValues(values...).Observe(seconds)
	}
}

// Observe ：传入duration和labels，
func (t *Timer) Observe(duration time.Duration, label ...string) {
	if t == nil {
		return
	}

	seconds := float64(duration) / float64(time.Second)
	t.summary.WithLabelValues(label...).Observe(seconds)
	t.histogram.WithLabelValues(label...).Observe(seconds)
}
