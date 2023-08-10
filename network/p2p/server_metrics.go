package p2p

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	libp2pMetrics "github.com/libp2p/go-libp2p/core/metrics"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	BandwidthOut      metrics.Counter
	BandwidthIn       metrics.Counter
	InBoundConnLimit  metrics.Gauge
	OutBoundConnLimit metrics.Gauge
	TotalConnections  metrics.Gauge
}

func GetPrometheusMetrics(namespace string, labelsWithValues ...string) *Metrics {
	labels := make([]string, 0)

	for i := 0; i < len(labelsWithValues); i += 2 {
		labels = append(labels, labelsWithValues[i])
	}

	return &Metrics{
		BandwidthOut: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "server",
			Name:      "outgoing_bandwidth",
			Help:      "Current network outgoing bandwidth",
		}, labels).With(labelsWithValues...),
		BandwidthIn: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "server",
			Name:      "incoming_bandwidth",
			Help:      "Current network incoming bandwidth",
		}, labels).With(labelsWithValues...),
		InBoundConnLimit: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "server",
			Name:      "inbound_conn",
			Help:      "Number of inbound connections",
		}, labels).With(labelsWithValues...),
		OutBoundConnLimit: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "server",
			Name:      "outbound_conn",
			Help:      "Number of outbound connections",
		}, labels).With(labelsWithValues...),
		TotalConnections: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "server",
			Name:      "total_connections",
			Help:      "Total no of connections",
		}, labels).With(labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		BandwidthOut:      discard.NewCounter(),
		BandwidthIn:       discard.NewCounter(),
		InBoundConnLimit:  discard.NewGauge(),
		OutBoundConnLimit: discard.NewGauge(),
		TotalConnections:  discard.NewGauge(),
	}
}

func (metrics *Metrics) CaptureInboundConn(count float64) {
	metrics.InBoundConnLimit.Add(count)
}

func (metrics *Metrics) CaptureOutboundConn(count float64) {
	metrics.OutBoundConnLimit.Add(count)
}

func (metrics *Metrics) CaptureTotalConn(total float64) {
	metrics.TotalConnections.Set(total)
}

type BandwidthReporter struct {
	metrics *Metrics
	*libp2pMetrics.BandwidthCounter
}

func newBandwidthReporter(metrics *Metrics, counter *libp2pMetrics.BandwidthCounter) *BandwidthReporter {
	return &BandwidthReporter{
		metrics:          metrics,
		BandwidthCounter: counter,
	}
}

func (reporter *BandwidthReporter) LogSentMessage(size int64) {
	reporter.metrics.CaptureBandwidthOut(size)
}

func (reporter *BandwidthReporter) LogRecvMessage(size int64) {
	reporter.metrics.CaptureBandwidthIn(size)
}

func (metrics *Metrics) CaptureBandwidthOut(size int64) {
	metrics.BandwidthOut.Add(float64(size))
}

func (metrics *Metrics) CaptureBandwidthIn(size int64) {
	metrics.BandwidthIn.Add(float64(size))
}
