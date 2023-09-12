package forage

import "github.com/sarvalabs/go-moi/common"

type eventDataJobState struct {
	address common.Address
	height  uint64
}

type eventSystemAccounts struct{}

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
