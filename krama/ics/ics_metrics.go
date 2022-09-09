package ics

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

// ICSMetrics represents the metrics captured for ClusterInfo
type ICSMetrics struct {
	// ClusterInfo Size
	ClusterSize metrics.Gauge

	//Time between ICS creation request and success response in Milliseconds
	SuccessInterval metrics.Gauge

	//Response time for ClusterInfo Join request RPC call
	ResponseTime metrics.Histogram

	//Time taken to query random nodes for ClusterInfo creation
	RandomNodesQueryTime metrics.Gauge

	//Time taken to create an ClusterInfo
	CreationTime metrics.Gauge

	//ClusterInfo failure rate
	FailureRate metrics.Gauge
}

// GetPrometheusMetrics return the consensus metrics instance
func GetICSPrometheusMetrics(namespace string, labelsWithValues ...string) *ICSMetrics {
	labels := []string{}

	for i := 0; i < len(labelsWithValues); i += 2 {
		labels = append(labels, labelsWithValues[i])
	}

	return &ICSMetrics{
		ClusterSize: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "ICS",
			Name:      "ics_size",
			Help:      "Number of nodes in ClusterInfo.",
		}, labels).With(labelsWithValues...),
		SuccessInterval: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "ICS",
			Name:      "success_interval",
			Help:      "Time between ICS creation request and success response in Milliseconds.",
		}, labels).With(labelsWithValues...),
		ResponseTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "ICS",
			Name:      "join_response_time",
			Help:      "Response time for ClusterInfo Join request RPC call.",
		}, labels).With(labelsWithValues...),
		RandomNodesQueryTime: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "ICS",
			Name:      "random_nodes_query_time",
			Help:      "Time taken to query random nodes for ClusterInfo creation.",
		}, labels).With(labelsWithValues...),

		CreationTime: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "ICS",
			Name:      "creation_time",
			Help:      "Time between current block and the previous block in seconds.",
		}, labels).With(labelsWithValues...),
		FailureRate: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "ICS",
			Name:      "failure_rate",
			Help:      "ClusterInfo failure rate.",
		}, labels).With(labelsWithValues...),
	}
}

// NilMetrics will return the non operational metrics
func ICSNilMetrics() *ICSMetrics {
	return &ICSMetrics{
		ClusterSize:          discard.NewGauge(),
		SuccessInterval:      discard.NewGauge(),
		ResponseTime:         discard.NewHistogram(),
		RandomNodesQueryTime: discard.NewGauge(),
		CreationTime:         discard.NewGauge(),
		FailureRate:          discard.NewGauge(),
	}
}
