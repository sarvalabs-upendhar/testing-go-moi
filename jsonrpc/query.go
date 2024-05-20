package jsonrpc

import (
	"encoding/json"

	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
)

// LogQuery is a query to filter logs
type LogQuery struct {
	StartHeight int64               `json:"start_height"`
	EndHeight   int64               `json:"end_height"`
	Address     identifiers.Address `json:"address"`
	Topics      [][]common.Hash     `json:"topics"`
}

// UnmarshalJSON decodes a json object
func (q *LogQuery) UnmarshalJSON(data []byte) error {
	var obj struct {
		StartHeight int64               `json:"start_height"`
		EndHeight   int64               `json:"end_height"`
		Address     identifiers.Address `json:"address"`
		Topics      []interface{}       `json:"topics"`
	}

	err := json.Unmarshal(data, &obj)
	if err != nil {
		return ErrUnmarshallingData
	}

	q.StartHeight = obj.StartHeight
	q.EndHeight = obj.EndHeight

	if obj.Address == identifiers.NilAddress {
		return common.ErrInvalidAddress
	}

	q.Address = obj.Address

	if obj.Topics != nil {
		topics, err := rpcargs.UnmarshalTopic(obj.Topics)
		if err != nil {
			return err
		}

		q.Topics = topics
	}

	// decode topics
	return nil
}

// MatchTopics Examples:
// {} or nil          matches any topic list
// {{A}}              matches topic A in first position
// {{}, {B}}          matches any topic in first position AND B in second position
// {{A}, {B}}         matches topic A in first position AND B in second position
// {{A, B}, {C, D}}   matches topic (A OR B) in first position AND (C OR D) in second position
// MatchTopics returns whether the log match the query
func (q *LogQuery) MatchTopics(log *common.Log) bool {
	// assuming there are no duplicate topics in logs
	if len(q.Topics) > len(log.Topics) {
		return false
	}

	for i, query := range q.Topics {
		// To allow empty topic query to match
		match := len(query) == 0

		for _, topic := range query {
			if log.Topics[i] == topic {
				match = true

				break
			}
		}

		if !match {
			return false
		}
	}

	return true
}

// decodeFilterQuery decodes the json rpc request parameter made for new tesseract log event subscription
func decodeFilterQuery(data interface{}) (*LogQuery, error) {
	// once the log filter is decoded as map[string]interface we cannot use unmarshal json
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	query := &LogQuery{}
	if err := json.Unmarshal(raw, &query); err != nil {
		return nil, err
	}

	return query, nil
}
