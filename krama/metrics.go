package krama

import (
	"time"

	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	AvailableOperatorSlots       metrics.Gauge
	AvailableValidatorSlots      metrics.Gauge
	ClusterSize                  metrics.Gauge
	ICSJoiningTime               metrics.Histogram
	RequestTurnaroundTime        metrics.Histogram
	RandomNodesQueryTime         metrics.Histogram
	ICSCreationTime              metrics.Histogram
	ICSCreationFailureCount      metrics.Counter
	ICSParticipationFailureCount metrics.Counter
	GridGenerationTime           metrics.Histogram
	AgreementTime                metrics.Histogram
	AgreementFailureCount        metrics.Counter
}

func GetPrometheusMetrics(namespace string, labelsWithValues ...string) *Metrics {
	labels := make([]string, 0)

	for i := 0; i < len(labelsWithValues); i += 2 {
		labels = append(labels, labelsWithValues[i])
	}

	return &Metrics{
		AvailableOperatorSlots: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "krama",
			Name:      "available_operator_slots",
			Help:      "Number of operator slots available",
		}, labels).With(labelsWithValues...),
		AvailableValidatorSlots: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "krama",
			Name:      "available_validator_slots",
			Help:      "Number of validator slots available",
		}, labels).With(labelsWithValues...),
		ClusterSize: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "ics",
			Name:      "cluster_size",
			Help:      "Number of nodes in the ICS cluster.",
		}, labels).With(labelsWithValues...),
		ICSJoiningTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "ics",
			Name:      "joining_time",
			Help:      "Time taken to join the ics cluster",
		}, labels).With(labelsWithValues...),
		RequestTurnaroundTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "ics",
			Name:      "request_turnaround_time",
			Help:      "Request turnaround time for ICS cluster join request RPC call.",
		}, labels).With(labelsWithValues...),
		RandomNodesQueryTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "ics",
			Name:      "random_nodes_query_time",
			Help:      "Time taken to query random nodes for ICS cluster creation",
		}, labels).With(labelsWithValues...),
		ICSCreationTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "ics",
			Name:      "creation_time",
			Help:      "Time taken to create a ICS cluster successfully.",
		}, labels).With(labelsWithValues...),
		ICSCreationFailureCount: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "ics",
			Name:      "creation_failure_rate",
			Help:      "ICS creation failure count.",
		}, labels).With(labelsWithValues...),
		ICSParticipationFailureCount: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "ics",
			Name:      "participation_failure_count",
			Help:      "ICS participation failure count.",
		}, labels).With(labelsWithValues...),
		GridGenerationTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "krama",
			Name:      "execution_time",
			Help:      "Time taken to create a successful consensus proposal",
		}, labels).With(labelsWithValues...),
		AgreementTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "kbft",
			Name:      "agreement_time",
			Help:      "Time taken for a successful consensus decision",
		}, labels).With(labelsWithValues...),
		AgreementFailureCount: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "kbft",
			Name:      "agreement_failure_count",
			Help:      "Consensus agreement failure count",
		}, labels).With(labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		AvailableOperatorSlots:       discard.NewGauge(),
		AvailableValidatorSlots:      discard.NewGauge(),
		ClusterSize:                  discard.NewGauge(),
		ICSJoiningTime:               discard.NewHistogram(),
		RequestTurnaroundTime:        discard.NewHistogram(),
		RandomNodesQueryTime:         discard.NewHistogram(),
		ICSCreationTime:              discard.NewHistogram(),
		ICSCreationFailureCount:      discard.NewCounter(),
		ICSParticipationFailureCount: discard.NewCounter(),
		GridGenerationTime:           discard.NewHistogram(),
		AgreementTime:                discard.NewHistogram(),
		AgreementFailureCount:        discard.NewCounter(),
	}
}

// methods to capture telemetry metrics
func (metrics *Metrics) initMetrics(operatorSlotsCount float64, validatorSlotCount float64) {
	metrics.AvailableOperatorSlots.Set(operatorSlotsCount)
	metrics.AvailableValidatorSlots.Set(validatorSlotCount)
}

func (metrics *Metrics) captureAvailableOperatorSlots(delta float64) {
	metrics.AvailableOperatorSlots.Add(delta)
}

func (metrics *Metrics) captureAvailableValidatorSlots(delta float64) {
	metrics.AvailableValidatorSlots.Add(delta)
}

func (metrics *Metrics) captureClusterSize(clusterSize float64) {
	metrics.ClusterSize.Set(clusterSize)
}

func (metrics *Metrics) captureICSJoiningTime(requestTime time.Time) {
	metrics.ICSJoiningTime.Observe(float64(time.Since(requestTime).Milliseconds()))
}

func (metrics *Metrics) captureRequestTurnaroundTime(requestTS time.Time) {
	metrics.RequestTurnaroundTime.Observe(float64(time.Since(requestTS).Milliseconds()))
}

func (metrics *Metrics) captureRandomNodesQueryTime(queryInitTime time.Time) {
	metrics.RandomNodesQueryTime.Observe(float64(time.Since(queryInitTime).Milliseconds()))
}

func (metrics *Metrics) captureICSCreationTime(icsReqTime time.Time) {
	metrics.ICSCreationTime.Observe(float64(time.Since(icsReqTime).Milliseconds()))
}

func (metrics *Metrics) captureICSCreationFailureCount(delta float64) {
	metrics.ICSCreationFailureCount.Add(delta)
}

func (metrics *Metrics) captureICSParticipationFailureCount(delta float64) {
	metrics.ICSParticipationFailureCount.Add(delta)
}

func (metrics *Metrics) captureGridGenerationTime(executionReqTS time.Time) {
	metrics.GridGenerationTime.Observe(float64(time.Since(executionReqTS).Milliseconds()))
}

func (metrics *Metrics) captureAgreementTime(consensusInitTS time.Time) {
	metrics.AgreementTime.Observe(float64(time.Since(consensusInitTS).Milliseconds()))
}

func (metrics *Metrics) captureAgreementFailureCount(delta float64) {
	metrics.AgreementFailureCount.Add(delta)
}
