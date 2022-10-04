package decision

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	prometheus "github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
	"time"
)

type Metrics struct {
	PendingRequests    metrics.Gauge
	RequestProcessTime metrics.Histogram
	CidsPerRequest     metrics.Histogram
	TimedOutRequests   metrics.Counter
	RejectedRequests   metrics.Counter
}

func GetPrometheusMetrics(namespace string, labels []string, labelsWithValues ...string) *Metrics {
	return &Metrics{
		PendingRequests: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "agora",
			Name:      "pending_requests",
			Help:      "Number of pending requests in the queue",
		}, labels).With(labelsWithValues...),
		RequestProcessTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "agora",
			Name:      "request_process_time",
			Help:      "Time taken by a request to get processed",
		}, labels).With(labelsWithValues...),
		CidsPerRequest: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "agora",
			Name:      "average_cids_per_request",
			Help:      "Average number of context ids per request",
		}, labels).With(labelsWithValues...),
		TimedOutRequests: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "agora",
			Name:      "timed_out_requests",
			Help:      "Number of timed out requests",
		}, labels).With(labelsWithValues...),
		RejectedRequests: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "agora",
			Name:      "rejected_requests",
			Help:      "Number of rejected requests",
		}, labels).With(labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		PendingRequests:    discard.NewGauge(),
		RequestProcessTime: discard.NewHistogram(),
		CidsPerRequest:     discard.NewHistogram(),
		TimedOutRequests:   discard.NewCounter(),
		RejectedRequests:   discard.NewCounter(),
	}
}

// methods to capture telemetry metrics
func (metrics *Metrics) initMetrics() {
	// set default value for agora engine gauge metrics
	metrics.PendingRequests.Set(0)
}

func (metrics *Metrics) capturePendingRequests(delta float64) {
	metrics.PendingRequests.Add(delta)
}

func (metrics *Metrics) captureRequestProcessTime(requestTime time.Time) {
	metrics.RequestProcessTime.Observe(time.Since(requestTime).Seconds())
}

func (metrics *Metrics) captureCidsPerRequest(numOfCids float64) {
	metrics.CidsPerRequest.Observe(numOfCids)
}

func (metrics *Metrics) captureTimedOutRequests(delta float64) {
	metrics.TimedOutRequests.Add(delta)
}

func (metrics *Metrics) captureRejectedRequests(delta float64) {
	metrics.RejectedRequests.Add(delta)
}
