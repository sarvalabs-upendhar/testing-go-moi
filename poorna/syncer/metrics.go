package syncer

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
	BucketSyncTime    metrics.Histogram
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
			Buckets:   []float64{0.1, 0.2, 0.5, 60, 120},
		}, labels).With(labelsWithValues...),
		BucketSyncTime: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "syncer",
			Name:      "bucket_sync_time",
			Help:      "Time taken to sync buckets",
			Buckets:   []float64{5, 10, 15, 20, 25, 30, 60},
		}, labels).With(labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		ActiveJobs:        discard.NewGauge(),
		TotalJobs:         discard.NewGauge(),
		JobProcessingTime: discard.NewHistogram(),
		BucketSyncTime:    discard.NewHistogram(),
	}
}

func (metrics *Metrics) captureActiveJobs(delta float64) {
	metrics.ActiveJobs.Add(delta)
}

func (metrics *Metrics) captureTotalJobs(delta float64) {
	metrics.TotalJobs.Set(delta)
}

func (metrics *Metrics) captureJobProcessingTime(requestTime time.Time) {
	metrics.JobProcessingTime.Observe(time.Since(requestTime).Seconds())
}

func (metrics *Metrics) captureBucketSyncTime(requestTime time.Time) {
	metrics.BucketSyncTime.Observe(time.Since(requestTime).Minutes())
}
