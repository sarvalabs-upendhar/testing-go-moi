package forage

import (
	"github.com/hashicorp/go-hclog"
	"github.com/sarvalabs/go-moi/common/identifiers"
)

type Option = func(job *AccountSyncJob)

func WithLogger(logger hclog.Logger) func(*AccountSyncJob) {
	return func(job *AccountSyncJob) {
		job.logger = logger
	}
}

func WithDB(db store) func(*AccountSyncJob) {
	return func(job *AccountSyncJob) {
		job.db = db
	}
}

func WithJobState(jobState JobState) func(*AccountSyncJob) {
	return func(job *AccountSyncJob) {
		job.jobState = jobState
	}
}

func WithSnapDownloaded(snapDownloaded bool) func(*AccountSyncJob) {
	return func(job *AccountSyncJob) {
		job.snapDownloaded = snapDownloaded
	}
}

func WithCurrentHeight(currentHeight uint64) func(*AccountSyncJob) {
	return func(job *AccountSyncJob) {
		job.currentHeight = currentHeight
	}
}

func WithExpectedHeight(expectedHeight uint64) func(*AccountSyncJob) {
	return func(job *AccountSyncJob) {
		job.expectedHeight = expectedHeight
	}
}

func WithLatticeSyncInProgress(latticeSyncInProgress bool) func(*AccountSyncJob) {
	return func(job *AccountSyncJob) {
		job.latticeSyncInProgress = latticeSyncInProgress
	}
}

func WithTesseractQueue(tq *TesseractQueue) func(*AccountSyncJob) {
	return func(job *AccountSyncJob) {
		job.tesseractQueue = tq
	}
}

func WithBestPeers(m map[identifiers.KramaID]struct{}) func(job *AccountSyncJob) {
	return func(job *AccountSyncJob) {
		job.bestPeers = m
	}
}
