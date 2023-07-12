package common_test

import (
	"reflect"
	"testing"

	id "github.com/sarvalabs/moichain/common/kramaid"

	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/common/tests"
)

func TestCopyHeader(t *testing.T) {
	testcases := []struct {
		name   string
		header common.TesseractHeader
	}{
		{
			name:   "copy header",
			header: tests.CreateHeaderWithTestData(t),
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
		body common.TesseractBody
	}{
		{
			name: "copy body",
			body: tests.CreateBodyWithTestData(t),
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
		receipts common.Receipts
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
		commitData common.CommitData
	}{
		{
			name:       "copy commit data",
			commitData: tests.CreateCommitDataWithTestData(t),
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
	contextDelta := make(common.ContextDelta)
	address := tests.RandomAddress(t)

	contextDelta[address] = &common.DeltaGroup{
		Role:             common.Sender,
		BehaviouralNodes: tests.GetTestKramaIDs(t, 2),
		RandomNodes:      tests.GetTestKramaIDs(t, 2),
		ReplacedNodes:    tests.GetTestKramaIDs(t, 2),
	}

	testcases := []struct {
		name         string
		contextDelta common.ContextDelta
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
		tsGridID common.TesseractGridID
	}{
		{
			name: "copy tesseract grid id",
			tsGridID: common.TesseractGridID{
				Hash:  tests.RandomHash(t),
				Parts: tests.CreateTesseractPartsWithTestData(t),
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
		tsParts *common.TesseractParts
	}{
		{
			name:    "copy tesseract parts",
			tsParts: tests.CreateTesseractPartsWithTestData(t),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			expectedTSParts := test.tsParts

			copiedParts := test.tsParts.Copy()

			require.Equal(t, expectedTSParts, copiedParts)
			require.NotEqual(t,
				reflect.ValueOf(test.tsParts.Grid).Pointer(),
				reflect.ValueOf(copiedParts.Grid).Pointer(),
			)
		})
	}
}

func TestNewTesseract(t *testing.T) {
	ixParams := tests.GetIxParamsMapWithAddresses(
		[]common.Address{tests.RandomAddress(t)},
		[]common.Address{tests.RandomAddress(t)},
	)

	testcases := []struct {
		name     string
		header   common.TesseractHeader
		body     common.TesseractBody
		ixns     common.Interactions
		receipts common.Receipts
		seal     []byte
		sealer   id.KramaID
	}{
		{
			name:     "copy tesseract parts",
			header:   tests.CreateHeaderWithTestData(t),
			body:     tests.CreateBodyWithTestData(t),
			ixns:     tests.CreateIxns(t, 1, ixParams),
			receipts: createReceiptsWithTestData(t, tests.RandomHash(t)),
			seal:     []byte{1, 2, 3},
			sealer:   tests.GetTestKramaIDs(t, 1)[0],
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			tesseract := common.NewTesseract(test.header, test.body, test.ixns, test.receipts, test.seal, test.sealer)

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

func createReceiptsWithTestData(t *testing.T, hash common.Hash) common.Receipts {
	t.Helper()

	receipts := make(common.Receipts)
	receipts[hash] = tests.CreateReceiptWithTestData(t)

	return receipts
}
