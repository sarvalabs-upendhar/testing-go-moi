package forage

import "github.com/sarvalabs/go-moi-identifiers"

type eventDataJobState struct {
	id     identifiers.Identifier
	height uint64
}

type eventBucketSync struct{}

type eventLoadSyncJobsDB struct{}

type eventSnapSync struct {
	eventDataJobState
}

type eventLatticeSync struct {
	eventDataJobState
}

type eventTesseractSync struct {
	eventDataJobState
}

type eventJobDone struct {
	eventDataJobState
}
