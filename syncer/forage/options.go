package forage

import (
	"github.com/hashicorp/go-hclog"
	"github.com/sarvalabs/go-legacy-kramaid"
)

type Option = func(job *SyncJob)

func WithLogger(logger hclog.Logger) func(*SyncJob) {
	return func(job *SyncJob) {
		job.logger = logger
	}
}

func WithDB(db store) func(*SyncJob) {
	return func(job *SyncJob) {
		job.db = db
	}
}

func WithJobState(jobState JobState) func(*SyncJob) {
	return func(job *SyncJob) {
		job.jobState = jobState
	}
}

func WithSnapDownloaded(snapDownloaded bool) func(*SyncJob) {
	return func(job *SyncJob) {
		job.snapDownloaded = snapDownloaded
	}
}

func WithCurrentHeight(currentHeight uint64) func(*SyncJob) {
	return func(job *SyncJob) {
		job.currentHeight = currentHeight
	}
}

func WithExpectedHeight(expectedHeight uint64) func(*SyncJob) {
	return func(job *SyncJob) {
		job.expectedHeight = expectedHeight
	}
}

func WithLatticeSyncInProgress(latticeSyncInProgress bool) func(*SyncJob) {
	return func(job *SyncJob) {
		job.latticeSyncInProgress = latticeSyncInProgress
	}
}

func WithTesseractQueue(tq *TesseractQueue) func(*SyncJob) {
	return func(job *SyncJob) {
		job.tesseractQueue = tq
	}
}

func WithBestPeers(m map[kramaid.KramaID]struct{}) func(job *SyncJob) {
	return func(job *SyncJob) {
		job.bestPeers = m
	}
}
