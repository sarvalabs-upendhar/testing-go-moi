package lattice

import (
	"math/big"
	"sort"

	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/common"
)

func createGenesisTesseract(
	addresses []identifiers.Address,
	stateHashes, contextHashes []common.Hash,
) *common.Tesseract {
	var (
		ixHashString = "Genesis"
		participants = make(common.Participants)
	)

	for i, addr := range addresses {
		participants[addr] = common.State{
			Height:          0,
			TransitiveLink:  common.NilHash,
			PreviousContext: common.NilHash,
			LatestContext:   contextHashes[i],
			StateHash:       stateHashes[i],
		}
	}

	sort.Slice(stateHashes, func(i, j int) bool {
		return stateHashes[i].Hex() < stateHashes[j].Hex()
	})

	for i := 0; i < len(stateHashes); i++ {
		ixHashString += stateHashes[i].Hex()
	}

	interactionsHash := common.GetHash([]byte(ixHashString))

	poxt := common.PoXtData{
		Round:     0,
		ClusterID: common.GenesisIdentifier,
	}

	return common.NewTesseract(
		participants,
		interactionsHash,
		common.NilHash,
		big.NewInt(0),
		0,
		common.GenesisIdentifier,
		0,
		0,
		poxt,
		nil,
		"",
		nil,
		nil,
	)
}
