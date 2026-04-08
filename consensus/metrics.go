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
	CurrentView                  metrics.Gauge
	ActiveICSClusters            metrics.Gauge
	ProposalValidationTime       metrics.Histogram
	RequestTurnaroundTime        metrics.Histogram
	RandomNodesQueryTime         metrics.Histogram
	ICSCreationTime              metrics.Histogram
	ICSCreationFailureCount      metrics.Counter
	ICSParticipationFailureCount metrics.Counter
	PrepareQcSigAggregationTime  metrics.Histogram
	AgreementTime                metrics.Histogram
	AgreementFailureCount        metrics.Counter
	SignatureVerificationTime    metrics.Histogram
	TesseractMissCount           metrics.Counter
	ProposalPayloadSize          metrics.Gauge
	CompressionRatio             metrics.Histogram
	CompressionTime              metrics.Histogram
	DeCompressionTime            metrics.Histogram
	TotalJobs                    metrics.Gauge
	JobProcessingTime            metrics.Histogram
	JobTimeInQueue               metrics.Histogram
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
		CurrentView: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "krama",
			Name:      "current_view",
			Help:      "Current view ID",
		}, labels).With(labelsWithValues...),
		ActiveICSClusters: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "krama",
			Name:      "active_ics_clusters",
			Help:      "Number of active ICS clusters",
		}, labels).With(labelsWithValues...),
		ProposalValidationTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
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
		PrepareQcSigAggregationTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
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
		TesseractMissCount: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "kbft",
			Name:      "block_miss_count",
			Help:      "Number of blocks missing",
		}, labels).With(labelsWithValues...),
		ProposalPayloadSize: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "krama",
			Name:      "proposal_payload_size",
			Help:      "Input and compressed payload sizes of proposal",
		}, append(labels, []string{"payload_type"}...)).With(labelsWithValues...),
		CompressionRatio: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "krama",
			Name:      "compression_ratio",
			Help:      "Measure of how much a proposal's payload is reduced in size after compression",
			Buckets:   []float64{10, 30, 50, 70, 80, 90, 100},
		}, labels).With(labelsWithValues...),
		CompressionTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "krama",
			Name:      "compression_time",
			Help:      "Time taken to compress proposal payload",
			Buckets:   []float64{0.5, 1, 2, 3, 4, 5, 6, 10, 15},
		}, labels).With(labelsWithValues...),
		DeCompressionTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "krama",
			Name:      "decompression_time",
			Help:      "Time taken to decompress proposal payload",
			Buckets:   []float64{0.1, 0.3, 0.5, 1, 2, 5, 10, 15},
		}, labels).With(labelsWithValues...),
		TotalJobs: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "krama",
			Name:      "total_jobs",
			Help:      "Total jobs in the queue",
		}, labels).With(labelsWithValues...),
		JobProcessingTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "krama",
			Name:      "job_processing_time",
			Help:      "Time taken to process the job",
			Buckets:   []float64{100, 250, 500, 700, 850, 1000, 2000, 3000, 4000, 5000},
		}, labels).With(labelsWithValues...),
		JobTimeInQueue: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "krama",
			Name:      "job_time_in_queue",
			Help:      "Time spent by job in the queue",
			Buckets:   []float64{100, 250, 500, 700, 850, 1000, 2000, 3000, 4000, 5000},
		}, labels).With(labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		AvailableOperatorSlots:       discard.NewGauge(),
		AvailableValidatorSlots:      discard.NewGauge(),
		ProposalValidationTime:       discard.NewHistogram(),
		RequestTurnaroundTime:        discard.NewHistogram(),
		RandomNodesQueryTime:         discard.NewHistogram(),
		CurrentView:                  discard.NewGauge(),
		ActiveICSClusters:            discard.NewGauge(),
		ICSCreationTime:              discard.NewHistogram(),
		ICSCreationFailureCount:      discard.NewCounter(),
		ICSParticipationFailureCount: discard.NewCounter(),
		PrepareQcSigAggregationTime:  discard.NewHistogram(),
		AgreementTime:                discard.NewHistogram(),
		AgreementFailureCount:        discard.NewCounter(),
		SignatureVerificationTime:    discard.NewHistogram(),
		TesseractMissCount:           discard.NewCounter(),
		ProposalPayloadSize:          discard.NewGauge(),
		CompressionRatio:             discard.NewHistogram(),
		CompressionTime:              discard.NewHistogram(),
		DeCompressionTime:            discard.NewHistogram(),
		TotalJobs:                    discard.NewGauge(),
		JobProcessingTime:            discard.NewHistogram(),
		JobTimeInQueue:               discard.NewHistogram(),
	}
}

// methods to capture telemetry metrics
func (metrics *Metrics) initMetrics(operatorSlotsCount float64, validatorSlotCount float64) {
	metrics.AvailableOperatorSlots.Set(operatorSlotsCount)
	metrics.AvailableValidatorSlots.Set(validatorSlotCount)
	metrics.TesseractMissCount.Add(0)
}

func (metrics *Metrics) captureSlotCount(slotType int, delta float64) {
	if slotType == 0 { // Operator slot
		metrics.AvailableOperatorSlots.Add(delta)
	} else {
		metrics.AvailableValidatorSlots.Add(delta)
	}
}

func (metrics *Metrics) captureActiveICSClusters(delta float64) {
	metrics.ActiveICSClusters.Add(delta)
}

func (metrics *Metrics) captureCurrentView(viewID uint64) {
	metrics.CurrentView.Set(float64(viewID))
}

func (metrics *Metrics) captureProposalValidationTime(requestTime time.Time) {
	metrics.ProposalValidationTime.Observe(float64(time.Since(requestTime).Milliseconds()))
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

func (metrics *Metrics) capturePrepareQCSigAggregationTime(initTime time.Time) {
	metrics.PrepareQcSigAggregationTime.Observe(float64(time.Since(initTime).Milliseconds()))
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

func (metrics *Metrics) AddTesseractMissCount(delta float64) {
	metrics.TesseractMissCount.Add(delta)
}

// methods to capture telemetry metrics
func (metrics *Metrics) captureInputPayloadSize(size float64) {
	metrics.ProposalPayloadSize.With("payload_type", "input").Set(size)
}

func (metrics *Metrics) captureCompressedPayloadSize(size float64) {
	metrics.ProposalPayloadSize.With("payload_type", "compressed").Set(size)
}

func (metrics *Metrics) captureCompressionRatio(ratio float64) {
	metrics.CompressionRatio.Observe(ratio)
}

func (metrics *Metrics) captureCompressionTime(compressionTime time.Time) {
	metrics.CompressionTime.Observe(time.Since(compressionTime).Seconds())
}

func (metrics *Metrics) captureDeCompressionTime(deCompressionTime time.Time) {
	metrics.DeCompressionTime.Observe(time.Since(deCompressionTime).Seconds())
}

func (metrics *Metrics) captureTotalJobs(delta float64) {
	metrics.TotalJobs.Set(delta)
}

func (metrics *Metrics) captureJobProcessingTime(requestTime time.Time) {
	metrics.JobProcessingTime.Observe(float64(time.Since(requestTime).Milliseconds()))
}

func (metrics *Metrics) captureJobTimeInQueue(requestTime time.Time) {
	metrics.JobTimeInQueue.Observe(float64(time.Since(requestTime).Milliseconds()))
}
