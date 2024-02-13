package lattice

import (
	"sort"
	"testing"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/stretchr/testify/require"
)

func TestCreateGenesisTesseract(t *testing.T) {
	participantCount := 3

	testcases := []struct {
		name          string
		addresses     []identifiers.Address
		stateHashes   []common.Hash
		contextHashes []common.Hash
	}{
		{
			name:          "create genesis tesseract successfully",
			addresses:     tests.GetAddresses(t, participantCount),
			stateHashes:   tests.GetHashes(t, participantCount),
			contextHashes: tests.GetHashes(t, participantCount),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			// send copy of stateHashes to avoid modifications
			s := make(common.Hashes, participantCount)
			copy(s, test.stateHashes)

			ts := createGenesisTesseract(test.addresses, s, test.contextHashes)

			participants := ts.Participants()

			require.Equal(t, participantCount, len(participants))

			for i, addr := range test.addresses {
				p, ok := participants[addr]
				require.True(t, ok)

				require.Equal(t, uint64(0), p.Height)
				require.Equal(t, common.NilHash, p.TransitiveLink)
				require.Equal(t, common.NilHash, p.PreviousContext)
				require.Equal(t, test.contextHashes[i], p.LatestContext)
				require.Equal(t, test.stateHashes[i], p.StateHash)
				require.Equal(t, common.DeltaGroup{}, p.ContextDelta)
			}

			require.Equal(t, common.GenesisIdentifier, string(ts.ConsensusInfo().ClusterID))
			require.Equal(t, int32(0), ts.ConsensusInfo().Round)

			ixHashString := "Genesis"

			sort.Slice(test.stateHashes, func(i, j int) bool {
				return test.stateHashes[i].Hex() < test.stateHashes[j].Hex()
			})

			for i := 0; i < len(test.stateHashes); i++ {
				ixHashString += test.stateHashes[i].Hex()
			}

			interactionsHash := common.GetHash([]byte(ixHashString))

			require.Equal(t, interactionsHash, ts.InteractionsHash())
			require.Equal(t, common.GenesisIdentifier, ts.Operator())
		})
	}
}
