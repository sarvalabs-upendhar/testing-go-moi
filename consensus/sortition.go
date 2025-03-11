package consensus

import (
	"bytes"
	"container/heap"
	"context"
	"fmt"
	"sync"

	"github.com/algorand/sortition"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/crypto/vrf"
	blst "github.com/supranational/blst/bindings/go"
)

var (
	ExpectedSortionSize       = 11
	ErrOperatorNotEligible    = errors.New("operator not eligible")
	ErrInvalidICSProof        = errors.New("invalid ics proof")
	ErrInvalidTotalIncentives = errors.New(
		fmt.Sprintf("total incentives should be greater than or equal to %d", ExpectedSortionSize),
	)
)

const MaxEligibleOperators = 1

type ICSOperators []*types.ICSOperatorInfo

func (r ICSOperators) Has(kramaID identifiers.KramaID) bool {
	for _, v := range r {
		if v.KramaID == kramaID {
			return true
		}
	}

	return false
}

func (r ICSOperators) Peek() *types.ICSOperatorInfo {
	if r.Len() == 0 {
		return nil
	}

	return (r)[0]
}

func (r ICSOperators) Len() int {
	return len(r)
}

func (r ICSOperators) Swap(i, j int) {
	(r)[i], (r)[j] = (r)[j], (r)[i]
}

func (r ICSOperators) Less(i, j int) bool {
	if (r)[i].Priority == (r)[j].Priority {
		return (r)[i].KramaID > (r)[j].KramaID
	}

	return (r)[i].Priority > (r)[j].Priority
}

func (r *ICSOperators) Push(x any) { // where to put the elem?
	*r = append(*r, x.(*types.ICSOperatorInfo)) //nolint
}

func (r *ICSOperators) Pop() interface{} {
	old := *r
	n := len(old)
	x := old[n-1]
	*r = old[0 : n-1]

	return x
}

// LotteryResult holds the result of lottery including
// selection status, ICS output, and ICS proof
type LotteryResult struct {
	isSelected bool
	vrfOutput  [32]byte
	vrfProof   []byte
}

// NewLotteryResult initializes and returns a new LotteryResult instance
func NewLotteryResult(isSelected bool, vrfOutput [32]byte, vrfProof []byte) *LotteryResult {
	return &LotteryResult{
		isSelected: isSelected,
		vrfOutput:  vrfOutput,
		vrfProof:   vrfProof,
	}
}

// OperatorSelection handles the lottery process.
type OperatorSelection struct {
	selfID      identifiers.KramaID
	cache       *lru.Cache
	vault       vault
	state       stateManager
	mtx         sync.RWMutex
	ixOperators *lru.Cache
}

// NewOperatorSelection initializes and returns a new OperatorSelection instance.
func NewOperatorSelection(
	selfID identifiers.KramaID,
	vault vault,
	state stateManager,
) (*OperatorSelection, error) {
	cache, err := lru.New(10)
	if err != nil {
		return nil, errors.Wrap(err, "operator lottery cache intialization failed")
	}

	reqCache, err := lru.New(50)
	if err != nil {
		return nil, errors.Wrap(err, "operator lottery cache intialization failed")
	}

	return &OperatorSelection{
		selfID:      selfID,
		cache:       cache,
		vault:       vault,
		state:       state,
		ixOperators: reqCache,
	}, nil
}

// computeICSSeed calculates the ICS seed using the provided participants' information.
// If the provided participants include any system account, the seed calculation should consider only
// the first system account, ordered by their id.
func (os *OperatorSelection) computeICSSeed(accounts common.Participants) ([32]byte, error) {
	var icsSeed [32]byte

	knownTSHashes := make(map[common.Hash]struct{})

	for id, account := range accounts {
		if account.IsGenesis || account.ExcludeFromICS {
			continue
		}

		if _, exists := knownTSHashes[account.TesseractHash]; exists {
			continue
		}

		knownTSHashes[account.TesseractHash] = struct{}{}

		seed, err := os.state.GetICSSeed(id)
		if err != nil {
			return [32]byte{}, err
		}

		if bytes.Equal(icsSeed[:], seed[:]) {
			continue
		}

		for i := 0; i < len(icsSeed); i++ {
			icsSeed[i] ^= seed[i]
		}
	}

	return icsSeed, nil
}

func (os *OperatorSelection) computeVRFOutput(icsSeed [32]byte) ([32]byte, []byte, error) {
	secretKey := new(blst.SecretKey)
	secretKey.Deserialize(os.vault.GetConsensusPrivateKey().Bytes())

	signer := vrf.NewVRFSigner(secretKey)

	icsOutput, icsProof, err := signer.Evaluate(icsSeed[:])
	if err != nil {
		return [32]byte{}, nil, err
	}

	return icsOutput, icsProof, nil
}

// Select performs the lottery to determine if the operator is selected
func (os *OperatorSelection) Select(operatorIncentive uint64, icsOutput [32]byte) (uint64, error) {
	totalIncentives, err := os.state.GetTotalIncentives()
	if err != nil {
		return 0, err
	}

	if totalIncentives < uint64(ExpectedSortionSize) {
		return 0, ErrInvalidTotalIncentives
	}

	selection := sortition.Select(operatorIncentive, totalIncentives, float64(ExpectedSortionSize), icsOutput)
	if selection > 0 {
		return selection, nil
	}

	return 0, ErrOperatorNotEligible
}

// VerifySelection verifies the ICS output and proof for a selected operator
func (os *OperatorSelection) VerifySelection(
	operator identifiers.KramaID,
	icsSeed,
	icsOutput [32]byte,
	icsProof []byte,
) (uint64, error) {
	keys, err := os.state.GetPublicKeys(context.Background(), operator)
	if err != nil {
		return 0, err
	}

	pk := new(blst.P1Affine)
	pk.Uncompress(keys[0])

	verifier := vrf.NewVRFVerifier(pk)

	isVerified, err := verifier.Verify(icsOutput, icsSeed[:], icsProof)
	if err != nil {
		return 0, err
	}

	if !isVerified {
		return 0, ErrInvalidICSProof
	}

	operatorIncentive, err := os.state.GetGuardianIncentives(operator)
	if err != nil {
		return 0, err
	}

	return os.Select(operatorIncentive, icsOutput)
}

func (os *OperatorSelection) AddICSOperatorInfo(key common.LotteryKey, kramaID identifiers.KramaID, priority uint64) {
	os.mtx.Lock()
	defer os.mtx.Unlock()

	l, ok := os.ixOperators.Get(key)
	if !ok {
		req := make(ICSOperators, 0)

		heap.Init(&req)

		info := &types.ICSOperatorInfo{
			KramaID:  kramaID,
			Priority: priority,
			Attempts: 1,
		}

		heap.Push(&req, info)
		os.ixOperators.Add(key, req)

		return
	}

	list, ok := l.(ICSOperators)
	if !ok {
		panic("type conversion failed for ics requests")
	}

	for _, v := range list {
		if v.KramaID == kramaID {
			v.Attempts++

			return
		}
	}

	heap.Push(&list, &types.ICSOperatorInfo{
		KramaID:  kramaID,
		Priority: priority,
		Attempts: 1,
	})

	os.ixOperators.Add(key, list)
}

func (os *OperatorSelection) IsEligible(key common.LotteryKey, kramaID identifiers.KramaID) bool {
	os.mtx.RLock()
	defer os.mtx.RUnlock()

	l, ok := os.ixOperators.Get(key)
	if !ok {
		return false // we return false here as this function is called after adding the operator info
	}

	for index, val := range l.(ICSOperators) { //nolint
		if index < MaxEligibleOperators && val.KramaID == kramaID {
			return true
		}
	}

	return false
}

func (os *OperatorSelection) DeleteICS(key common.LotteryKey) {
	os.mtx.Lock()
	defer os.mtx.Unlock()

	os.ixOperators.Remove(key)
}

func (os *OperatorSelection) GetEligibleOperators(key common.LotteryKey) []types.ICSOperatorInfo {
	os.mtx.RLock()
	defer os.mtx.RUnlock()

	l, ok := os.ixOperators.Get(key)
	if !ok {
		return nil
	}

	newList := make([]types.ICSOperatorInfo, 0, MaxEligibleOperators)

	for index, v := range l.(ICSOperators) { //nolint
		if index >= MaxEligibleOperators {
			break
		}

		newList = append(newList, types.ICSOperatorInfo{
			KramaID:  v.KramaID,
			Priority: v.Priority,
		})
	}

	return newList
}
