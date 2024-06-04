package consensus

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
	ClusterSize                  metrics.Histogram
	ICSJoiningTime               metrics.Histogram
	RequestTurnaroundTime        metrics.Histogram
	RandomNodesQueryTime         metrics.Histogram
	ICSCreationTime              metrics.Histogram
	ICSCreationFailureCount      metrics.Counter
	ICSParticipationFailureCount metrics.Counter
	GridGenerationTime           metrics.Histogram
	AgreementTime                metrics.Histogram
	AgreementFailureCount        metrics.Counter
	SignatureVerificationTime    metrics.Histogram
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
		ClusterSize: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "ics",
			Name:      "cluster_size",
			Help:      "Number of nodes in the ICS cluster.",
			Buckets:   []float64{20, 40, 60, 80, 100, 120},
		}, labels).With(labelsWithValues...),
		ICSJoiningTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "ics",
			Name:      "joining_time",
			Help:      "Time taken to join the ics cluster",
			Buckets:   []float64{200, 400, 600, 800, 1000, 1200, 1400, 1600, 1800, 2000},
		}, labels).With(labelsWithValues...),
		RequestTurnaroundTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "ics",
			Name:      "request_turnaround_time",
			Help:      "Request turnaround time for ICS cluster join request RPC call.",
			Buckets:   []float64{200, 400, 600, 800, 1000, 2000},
		}, labels).With(labelsWithValues...),
		RandomNodesQueryTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "ics",
			Name:      "random_nodes_query_time",
			Help:      "Time taken to query random nodes for ICS cluster creation",
			Buckets:   []float64{50, 100, 150, 200, 250, 300, 350, 400},
		}, labels).With(labelsWithValues...),
		ICSCreationTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "ics",
			Name:      "creation_time",
			Help:      "Time taken to create a ICS cluster successfully.",
			Buckets:   []float64{500, 1000, 1500, 2000, 2500, 3000},
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
			Buckets:   []float64{2, 4, 6, 8, 10, 20},
		}, labels).With(labelsWithValues...),
		AgreementTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "kbft",
			Name:      "agreement_time",
			Help:      "Time taken for a successful consensus decision",
			Buckets:   []float64{200, 400, 600, 800, 1000, 2000, 3000, 4000},
		}, labels).With(labelsWithValues...),
		AgreementFailureCount: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "kbft",
			Name:      "agreement_failure_count",
			Help:      "Consensus agreement failure count",
		}, labels).With(labelsWithValues...),
		SignatureVerificationTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "kbft",
			Name:      "signature_verification_time",
			Help:      "Time taken to verify tesseract signature",
			Buckets:   []float64{5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 100},
		}, labels).With(labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		AvailableOperatorSlots:       discard.NewGauge(),
		AvailableValidatorSlots:      discard.NewGauge(),
		ClusterSize:                  discard.NewHistogram(),
		ICSJoiningTime:               discard.NewHistogram(),
		RequestTurnaroundTime:        discard.NewHistogram(),
		RandomNodesQueryTime:         discard.NewHistogram(),
		ICSCreationTime:              discard.NewHistogram(),
		ICSCreationFailureCount:      discard.NewCounter(),
		ICSParticipationFailureCount: discard.NewCounter(),
		GridGenerationTime:           discard.NewHistogram(),
		AgreementTime:                discard.NewHistogram(),
		AgreementFailureCount:        discard.NewCounter(),
		SignatureVerificationTime:    discard.NewHistogram(),
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
	metrics.ClusterSize.Observe(clusterSize)
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

// methods to capture telemetry metrics
func (metrics *Metrics) captureSignatureVerificationTime(verificationInitTime time.Time) {
	metrics.SignatureVerificationTime.Observe(time.Since(verificationInitTime).Seconds())
}
