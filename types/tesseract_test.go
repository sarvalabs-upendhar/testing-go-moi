package types_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/types"
)

func TestCopyHeader(t *testing.T) {
	testcases := []struct {
		name   string
		header types.TesseractHeader
	}{
		{
			name:   "copy header",
			header: createHeaderWithTestData(t),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedHeader := test.header

			header := test.header.Copy()

			require.Equal(t, expectedHeader, header)
			require.NotEqual(t,
				reflect.ValueOf(test.header.Extra.CommitSignature).Pointer(),
				reflect.ValueOf(header.Extra.CommitSignature).Pointer(),
			)
			require.NotEqual(t,
				reflect.ValueOf(test.header.ContextLock).Pointer(),
				reflect.ValueOf(header.ContextLock).Pointer(),
			)
		})
	}
}

func TestCopyBody(t *testing.T) {
	testcases := []struct {
		name string
		body types.TesseractBody
	}{
		{
			name: "copy body",
			body: createBodyWithTestData(t),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedBody := test.body

			body := test.body.Copy()

			require.Equal(t, expectedBody, body)
			require.NotEqual(t,
				reflect.ValueOf(test.body.ContextDelta).Pointer(),
				reflect.ValueOf(body.ContextDelta).Pointer(),
			)
		})
	}
}

func TestCopyReceipts(t *testing.T) {
	hash := tests.RandomHash(t)

	testcases := []struct {
		name     string
		receipts types.Receipts
	}{
		{
			name:     "copy receipts",
			receipts: createReceiptsWithTestData(t, hash),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedReceipts := test.receipts

			receipts := test.receipts.Copy()

			require.Equal(t, expectedReceipts, receipts)

			require.NotEqual(t, reflect.ValueOf(test.receipts).Pointer(), reflect.ValueOf(receipts).Pointer())
			require.NotEqual(t,
				reflect.ValueOf(test.receipts[hash].ExtraData).Pointer(),
				reflect.ValueOf(receipts[hash].ExtraData).Pointer(),
			)
		})
	}
}

func TestCopyCommitData(t *testing.T) {
	testcases := []struct {
		name       string
		commitData types.CommitData
	}{
		{
			name:       "copy commit data",
			commitData: createCommitDataWithTestData(t),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedCommitData := test.commitData

			commitData := test.commitData.Copy()

			require.Equal(t, expectedCommitData, commitData)
			require.False(t, &test.commitData.VoteSet == &commitData.VoteSet)
			require.False(t, &test.commitData.GridID == &commitData.GridID)
			require.NotEqual(t,
				reflect.ValueOf(test.commitData.CommitSignature).Pointer(),
				reflect.ValueOf(commitData.CommitSignature).Pointer(),
			)
		})
	}
}

func TestCopyContextDelta(t *testing.T) {
	contextDelta := make(types.ContextDelta)
	address := tests.RandomAddress(t)

	contextDelta[address] = &types.DeltaGroup{
		Role:             types.Sender,
		BehaviouralNodes: tests.GetTestKramaIDs(t, 2),
		RandomNodes:      tests.GetTestKramaIDs(t, 2),
		ReplacedNodes:    tests.GetTestKramaIDs(t, 2),
	}

	testcases := []struct {
		name         string
		contextDelta types.ContextDelta
	}{
		{
			name:         "copy context delta",
			contextDelta: contextDelta,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedCtxDelta := test.contextDelta

			ctxDelta := test.contextDelta.Copy()

			require.Equal(t, expectedCtxDelta, ctxDelta)

			require.False(t, &test.contextDelta[address].BehaviouralNodes == &ctxDelta[address].BehaviouralNodes)
			require.False(t, &test.contextDelta[address].RandomNodes == &ctxDelta[address].RandomNodes)
			require.False(t, &test.contextDelta[address].ReplacedNodes == &ctxDelta[address].ReplacedNodes)
		})
	}
}

func TestCopyTesseractGridID(t *testing.T) {
	testcases := []struct {
		name     string
		tsGridID types.TesseractGridID
	}{
		{
			name: "copy tesseract grid id",
			tsGridID: types.TesseractGridID{
				Hash: tests.RandomHash(t),
				Parts: &types.TesseractParts{
					Hashes:  make([]types.Hash, 0),
					Heights: []uint64{2, 3},
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedGridID := test.tsGridID

			copiedGridID := test.tsGridID.Copy()

			require.Equal(t, expectedGridID, *copiedGridID)
			require.NotEqual(t, reflect.ValueOf(expectedGridID.Parts).Pointer(), reflect.ValueOf(copiedGridID.Parts).Pointer())
		})
	}
}

func TestCopyTesseractParts(t *testing.T) {
	testcases := []struct {
		name    string
		tsParts types.TesseractParts
	}{
		{
			name: "copy tesseract parts",
			tsParts: types.TesseractParts{
				Total:   4,
				Hashes:  []types.Hash{tests.RandomHash(t)},
				Heights: []uint64{1, 2, 3},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedTSParts := test.tsParts

			copiedParts := test.tsParts.Copy()

			require.Equal(t, expectedTSParts, *copiedParts)
			require.NotEqual(t, reflect.ValueOf(test.tsParts.Hashes).Pointer(), reflect.ValueOf(copiedParts.Hashes).Pointer())
			require.NotEqual(t, reflect.ValueOf(test.tsParts.Heights).Pointer(), reflect.ValueOf(copiedParts.Heights).Pointer())
		})
	}
}

func TestNewTesseract(t *testing.T) {
	ixParams := tests.GetIxParamsMapWithAddresses(
		[]types.Address{tests.RandomAddress(t)},
		[]types.Address{tests.RandomAddress(t)},
	)

	testcases := []struct {
		name     string
		header   types.TesseractHeader
		body     types.TesseractBody
		ixns     types.Interactions
		receipts types.Receipts
		seal     []byte
	}{
		{
			name:     "copy tesseract parts",
			header:   createHeaderWithTestData(t),
			body:     createBodyWithTestData(t),
			ixns:     tests.CreateIxns(t, 1, ixParams),
			receipts: createReceiptsWithTestData(t, tests.RandomHash(t)),
			seal:     []byte{1, 2, 3},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			tesseract := types.NewTesseract(test.header, test.body, test.ixns, test.receipts, test.seal)

			require.Equal(t, test.header, tesseract.Header())
			require.Equal(t, test.body, tesseract.Body())
			require.Equal(t, test.ixns, tesseract.Interactions())
			require.Equal(t, test.receipts, tesseract.Receipts())
			require.Equal(t, test.seal, tesseract.Seal())

			require.NotEqual(t,
				reflect.ValueOf(test.header.ContextLock).Pointer(),
				reflect.ValueOf(tesseract.ContextLock()).Pointer(),
			)
			require.NotEqual(t,
				reflect.ValueOf(test.body.ContextDelta).Pointer(),
				reflect.ValueOf(tesseract.ContextDelta()).Pointer(),
			)
			require.NotEqual(t,
				reflect.ValueOf(test.receipts).Pointer(),
				reflect.ValueOf(tesseract.Receipts()).Pointer(),
			)
			require.NotEqual(t,
				reflect.ValueOf(test.seal).Pointer(),
				reflect.ValueOf(tesseract.Seal()).Pointer(),
			)
		})
	}
}
