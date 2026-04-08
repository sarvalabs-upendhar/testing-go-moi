package forage

import (
	"time"

	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	ActiveJobs        metrics.Gauge
	TotalJobs         metrics.Gauge
	JobProcessingTime metrics.Histogram
	JobTimeInQueue    metrics.Histogram
	TSTimeInQueue     metrics.Histogram
	BucketSyncTime    metrics.Histogram
	IxMissCount       metrics.Counter
}

func GetPrometheusMetrics(namespace string, labelsWithValues ...string) *Metrics {
	labels := make([]string, 0)

	for i := 0; i < len(labelsWithValues); i += 2 {
		labels = append(labels, labelsWithValues[i])
	}

	return &Metrics{
		ActiveJobs: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "syncer",
			Name:      "active_jobs",
			Help:      "Number of active jobs in the queue",
		}, labels).With(labelsWithValues...),
		TotalJobs: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "syncer",
			Name:      "total_jobs",
			Help:      "Total jobs in the queue",
		}, labels).With(labelsWithValues...),
		JobProcessingTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "syncer",
			Name:      "job_processing_time",
			Help:      "Time taken to process the job",
			Buckets:   []float64{100, 250, 500, 700, 850, 1000, 2000, 3000, 4000, 5000},
		}, labels).With(labelsWithValues...),
		JobTimeInQueue: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "syncer",
			Name:      "job_time_in_queue",
			Help:      "Time spent by job in the queue",
			Buckets:   []float64{100, 250, 500, 700, 850, 1000, 2000, 3000, 4000, 5000},
		}, labels).With(labelsWithValues...),
		TSTimeInQueue: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "syncer",
			Name:      "TS_time_in_queue",
			Help:      "Time spent by tesseract in the queue",
			Buckets:   []float64{100, 250, 500, 700, 850, 1000, 2000, 3000, 4000, 5000},
		}, labels).With(labelsWithValues...),
		BucketSyncTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "syncer",
			Name:      "bucket_sync_time",
			Help:      "Time taken to sync buckets",
			Buckets:   []float64{5, 10, 15, 20, 25, 30, 60},
		}, labels).With(labelsWithValues...),
		IxMissCount: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "syncer",
			Name:      "ix_miss_count",
			Help:      "Number of ixns missing in ixpool",
		}, labels).With(labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		ActiveJobs:        discard.NewGauge(),
		TotalJobs:         discard.NewGauge(),
		JobProcessingTime: discard.NewHistogram(),
		JobTimeInQueue:    discard.NewHistogram(),
		TSTimeInQueue:     discard.NewHistogram(),
		BucketSyncTime:    discard.NewHistogram(),
		IxMissCount:       discard.NewCounter(),
	}
}

func (metrics *Metrics) captureActiveJobs(delta float64) {
	metrics.ActiveJobs.Add(delta)
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

func (metrics *Metrics) captureTSTimeInQueue(requestTime time.Time) {
	metrics.TSTimeInQueue.Observe(float64(time.Since(requestTime).Milliseconds()))
}

func (metrics *Metrics) captureBucketSyncTime(requestTime time.Time) {
	metrics.BucketSyncTime.Observe(time.Since(requestTime).Minutes())
}

func (metrics *Metrics) AddIxMissCount(delta float64) {
	metrics.IxMissCount.Add(delta)
}
