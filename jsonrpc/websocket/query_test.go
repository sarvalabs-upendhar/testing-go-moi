package websocket

import (
	"fmt"
	"testing"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalJSON(t *testing.T) {
	addr1 := tests.RandomAddress(t)
	hashes := tests.GetHashes(t, 3)

	testcases := []struct {
		name          string
		args          string
		response      *LogQuery
		expectedError error
	}{
		{
			name:          "Empty query",
			args:          `{}`,
			response:      nil,
			expectedError: common.ErrInvalidAddress,
		},
		{
			name: "Filter query with address",
			args: fmt.Sprintf(`{
				"address": "%s"
			}`, addr1.String()),
			response: &LogQuery{
				Address:     addr1,
				StartHeight: 0,
				EndHeight:   0,
				Topics:      nil,
			},
			expectedError: nil,
		},
		{
			name: "Filter query with address, heights and topics",
			args: fmt.Sprintf(`{
				"address": "%s",
				"start_height": 1,
				"end_height": 2,
				"topics": [
					"0x%s",
					[
						"0x%s"
					],
					[
						"0x%s",
						"0x%s"
					],
					null,
					"0x%s"
				]
			}`,
				addr1.String(),
				hashes[0].String(),
				hashes[0].String(),
				hashes[0].String(),
				hashes[1].String(),
				hashes[2].String(),
			),
			response: &LogQuery{
				Address:     addr1,
				StartHeight: 1,
				EndHeight:   2,
				Topics: [][]common.Hash{
					{
						hashes[0],
					},
					{
						hashes[0],
					},
					{
						hashes[0],
						hashes[1],
					},
					{},
					{
						hashes[2],
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "Filter query with invalid topic",
			args: fmt.Sprintf(`{
				"address": "%s",
				"start_height": 1,
				"end_height": 2,
				"topics": [
					"0x%s",
					[
						"0x%s"
					],
					[
						"abc",
					],
				]
			}`,
				addr1.String(),
				hashes[0].String(),
				hashes[1].String(),
			),
			response:      nil,
			expectedError: ErrUnmarshallingData,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			res := &LogQuery{}
			err := res.UnmarshalJSON([]byte(test.args))

			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.response, res)
		})
	}
}

func TestMatchTopics(t *testing.T) {
	hashes := tests.GetHashes(t, 4)

	testcases := []struct {
		name   string
		filter LogQuery
		log    *common.Log
		match  bool
	}{
		{
			// {} or nil - matches any topic list
			name: "empty query",
			filter: LogQuery{
				Topics: [][]common.Hash{
					{},
				},
			},
			log: &common.Log{
				Topics: []common.Hash{
					hashes[0],
				},
			},
			match: true,
		},
		{
			// {{A}} - matches topic A in first position
			name: "exact match",
			filter: LogQuery{
				Topics: [][]common.Hash{
					{
						hashes[0],
					},
				},
			},
			log: &common.Log{
				Topics: []common.Hash{
					hashes[0],
				},
			},
			match: true,
		},
		{
			name: "query has more hashes than tesseract logs",
			filter: LogQuery{
				Topics: [][]common.Hash{
					{
						hashes[0],
					},
					{
						hashes[0],
					},
				},
			},
			log: &common.Log{
				Topics: []common.Hash{
					hashes[0],
				},
			},
			match: false,
		},
		{
			// {{}, {B}} - matches any topic in first position AND B in second position
			name: "query hash matches one of the tesseract log hash",
			filter: LogQuery{
				Topics: [][]common.Hash{
					{},
					{
						hashes[1],
					},
				},
			},
			log: &common.Log{
				Topics: []common.Hash{
					hashes[0],
					hashes[1],
				},
			},
			match: true,
		},
		{
			// {{A}, {B}} - matches topic A in first position AND B in second position
			name: "multiple topics match",
			filter: LogQuery{
				Topics: [][]common.Hash{
					{
						hashes[0],
					},
					{
						hashes[1],
					},
				},
			},
			log: &common.Log{
				Topics: []common.Hash{
					hashes[0],
					hashes[1],
					hashes[2],
				},
			},
			match: true,
		},
		{
			// {{A, B}, {C, D}} - matches topic (A OR B) in first position AND (C OR D) in second position
			name: "multiple topics match",
			filter: LogQuery{
				Topics: [][]common.Hash{
					{
						hashes[0],
						hashes[1],
					},
					{
						hashes[2],
						hashes[3],
					},
				},
			},
			log: &common.Log{
				Topics: []common.Hash{
					hashes[0],
					hashes[3],
				},
			},
			match: true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.filter.MatchTopics(test.log), test.match)
		})
	}
}
