package args

import (
	"encoding/json"

	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
)

type SubscriptionType string

const (
	NewTesseract           SubscriptionType = "newTesseracts"
	NewTesseractsByAccount SubscriptionType = "newTesseractsByAccount"
	NewLogsByFilter        SubscriptionType = "newLogs"
	PendingIxns            SubscriptionType = "newPendingInteractions"
)

type FilterArgs struct {
	FilterID string `json:"id"`
}

type FilterResponse struct {
	FilterID string `json:"id"`
}

type FilterUninstallResponse struct {
	Status bool `json:"status"`
}

type TesseractFilterArgs struct{}

type TesseractByAccountFilterArgs struct {
	ID identifiers.Identifier `json:"id"`
}

type PendingIxnsFilterArgs struct{}

type FilterQueryArgs struct {
	StartHeight *int64                 `json:"start_height"`
	EndHeight   *int64                 `json:"end_height"`
	ID          identifiers.Identifier `json:"id"`
	Topics      [][]common.Hash        `json:"topics"`
}

// UnmarshalJSON decodes a Filter Query json object
func (q *FilterQueryArgs) UnmarshalJSON(data []byte) error {
	var obj struct {
		StartHeight *int64                 `json:"start_height"`
		EndHeight   *int64                 `json:"end_height"`
		ID          identifiers.Identifier `json:"id"`
		Topics      []interface{}          `json:"topics"`
	}

	err := json.Unmarshal(data, &obj)
	if err != nil {
		return err
	}

	if obj.StartHeight == nil {
		q.StartHeight = &LatestTesseractHeight
	} else {
		q.StartHeight = obj.StartHeight
	}

	if obj.EndHeight == nil {
		q.EndHeight = &LatestTesseractHeight
	} else {
		q.EndHeight = obj.EndHeight
	}

	if obj.ID == identifiers.Nil {
		return common.ErrInvalidIdentifier
	}

	q.ID = obj.ID

	if obj.Topics != nil {
		topics, err := UnmarshalTopic(obj.Topics)
		if err != nil {
			return err
		}

		q.Topics = topics
	}

	// decode topics
	return nil
}
