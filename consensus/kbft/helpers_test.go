package kbft

/*
import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"
	"time"

	kramaid "github.com/sarvalabs/go-legacy-kramaid"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/common/utils"
	ktypes "github.com/sarvalabs/go-moi/consensus/types"
	"github.com/sarvalabs/go-moi/crypto"
	mudracommon "github.com/sarvalabs/go-moi/crypto/common"
	"github.com/stretchr/testify/require"
)

const ensureTimeout = time.Millisecond * 200

type chain struct {
	tsCount int
}

func defaultChain() *chain {
	return &chain{
		tsCount: -1,
	}
}

func (c *chain) finalizeTesseractGrid(tesseract *common.Tesseract) error {
	if tesseract == nil {
		return nil
	}

	c.tsCount = len(tesseract.Participants())

	return nil
}

func createTestConsensusConfig() *config.ConsensusConfig {
	return &config.ConsensusConfig{
		DirectoryPath:         "",
		TimeoutPropose:        40 * time.Millisecond,
		TimeoutProposeDelta:   1 * time.Millisecond,
		TimeoutPrevote:        10 * time.Millisecond,
		TimeoutPrevoteDelta:   1 * time.Millisecond,
		TimeoutPrecommit:      10 * time.Millisecond,
		TimeoutPrecommitDelta: 1 * time.Millisecond,
		TimeoutCommit:         10 * time.Millisecond,
		SkipTimeoutCommit:     true,
	}
}

// createKramaIDAndPrivateKey returns kramaID and private key pair
func createKramaIDAndPrivateKey(t *testing.T, nthValidator uint32) (kramaid.KramaID, *crypto.BLSPrivKey) {
	t.Helper()

	var signKey [32]byte

	_, err := crand.Read(signKey[:]) // fill sign key with random bytes
	require.NoError(t, err)

	// get private key and public key
	privKeyBytes, moiPubBytes, err := tests.GetPrivKeysForTest(t, signKey[:])
	require.NoError(t, err)

	kramaID, err := kramaid.NewKramaID( // Create kramaID from private key , public key
		1,
		privKeyBytes[32:],
		nthValidator,
		hex.EncodeToString(moiPubBytes),
		true,
	)
	require.NoError(t, err)

	cPriv := new(crypto.BLSPrivKey)
	cPriv.UnMarshal(privKeyBytes[:32])

	return kramaID, cPriv
}

// createTestNodeSet return nodeset and vaults
// nodeset has nodes info like krama ID and public key
// vaults has node info like krama ID and private key which can be used to sign votes during consensus
func createTestNodeSet(t *testing.T, n int) (*ktypes.NodeSet, []*crypto.KramaVault) {
	t.Helper()

	publicKeys := make([][]byte, n)
	kramaIDs := make([]kramaid.KramaID, n)
	valset := make([]*crypto.KramaVault, n)

	for i := 0; i < n; i++ {
		kramaID, privateKey := createKramaIDAndPrivateKey(t, 0)
		publicKeys[i] = privateKey.GetPublicKeyInBytes()
		kramaIDs[i] = kramaID

		valset[i] = new(crypto.KramaVault)
		valset[i].SetKramaID(kramaID)
		valset[i].SetConsensusPrivateKey(privateKey)
	}

	nodeset := ktypes.NewNodeSet(kramaIDs, publicKeys, uint32(n))

	for i := 0; i < n; i++ {
		nodeset.Responses.SetIndex(i, true)
	}

	return nodeset, valset
}

/*
// createTestRandomSet return nodeset and vaults
// nodeset has nodes info like krama ID and public key
// vaults has node info like krama ID and private key which can be used to sign votes during consensus
// total tells number of nodes which also includes nodes that are not part of ics
// and actual tells the number of nodes in ics
func createTestRandomSet(t *testing.T, total, actual int) (*common.committee, []*crypto.KramaVault) {
	t.Helper()

	publicKeys := make([][]byte, total)
	kramaIDs := make([]kramaid.KramaID, total)
	valset := make([]*crypto.KramaVault, total)

	for i := 0; i < total; i++ {
		kramaID, privateKey := createKramaIDAndPrivateKey(t, 0)
		publicKeys[i] = privateKey.GetPublicKeyInBytes()
		kramaIDs[i] = kramaID

		valset[i] = new(crypto.KramaVault)
		valset[i].SetKramaID(kramaID)
		valset[i].SetConsensusPrivateKey(privateKey)
	}

	nodeset := common.NewNodeSet(kramaIDs, publicKeys, uint32(actual))

	for i := 0; i < actual; i++ {
		nodeset.Responses.SetIndex(i, true)
	}

	return nodeset, valset
}


// createICSNodes returns ICSNodes and vaults of given count of specific nodes
func createICSNodes(
	t *testing.T,
	participants int,
	nodesPerSet int,
	randomNodes, observerNodes int,
) (*ktypes.ICSCommittee, TestVaults) {
	t.Helper()

	vaults := make([][]*crypto.KramaVault, 0, 2*participants+2)

	ics := ktypes.NewICSCommittee(2*participants + 2)

	for i := 0; i < 2*participants; i++ {
		ns, vals := createTestNodeSet(t, nodesPerSet)
		ics.UpdateNodeSet(i, ns)

		vaults = append(vaults, vals)
	}

	randomNs, randomVals := createTestNodeSet(t, randomNodes)
	ics.UpdateNodeSet(ics.StochasticSetPosition(), randomNs)

	vaults = append(vaults, randomVals)

	return ics, vaults
}

type TestVaults [][]*crypto.KramaVault

func (ts TestVaults) GetVaults(participantIndex int, count int, exclude ...kramaid.KramaID) []*crypto.KramaVault {
	vals := make([]*crypto.KramaVault, 0, count)

	for i := participantIndex; i <= participantIndex+1; i++ {
		for _, v := range ts[i] {
			if utils.ContainsKramaID(exclude, v.KramaID()) {
				continue
			}

			vals = append(vals, v)

			if len(vals) == count {
				return vals
			}
		}
	}

	return vals
}

func startTestview(state *KBFT, heights map[identifiers.Identifier]uint64, view uint64, err chan<- error) {
	state.enterNewView(heights, view)

	err1 := state.Start()
	err <- err1
}

func handleOutboundMsgChannel(kbft *KBFT, ctx context.Context, out <-chan ktypes.ConsensusMessage) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-out:
				if !ok {
					kbft.logger.Debug("Outbound message channel close")

					return
				}
			}
		}
	}()
}

// signVote will create a vote message and sign it using the validator consensus key
func signVote(
	t *testing.T,
	kbft *KBFT,
	view uint64,
	msgType common.ConsensusMsgType,
	tsHash common.Hash,
	heights map[identifiers.Identifier]uint64,
	kramaVault *crypto.KramaVault,
) *ktypes.Vote {
	t.Helper()

	valIndex, _, _ := kbft.ics.HasKramaID(kramaVault.KramaID())
	require.NotEqual(t, valIndex, -1)

	v := &ktypes.Vote{
		SignerIndex: valIndex,
		TSHash:      tsHash,
		Heights:     heights,
		View:        view,
		Type:        msgType,
	}

	rawData, err := v.SignBytes()
	require.NoError(t, err)

	sign, err := kramaVault.Sign(rawData, mudracommon.BlsBLST)
	require.NoError(t, err)

	v.Signature = make([]byte, len(sign))
	copy(v.Signature, sign)

	return v
}

func signVotes(
	t *testing.T,
	kbft *KBFT,
	view uint64,
	msgType common.ConsensusMsgType,
	tsHash common.Hash,
	heights map[identifiers.Identifier]uint64,
	kramaVault ...*crypto.KramaVault,
) []*ktypes.Vote {
	t.Helper()

	votes := make([]*ktypes.Vote, len(kramaVault))

	for i, kVault := range kramaVault {
		votes[i] = signVote(t, kbft, view, msgType, tsHash, heights, kVault)
	}

	return votes
}

// signAddVotesSynchronously need to be used when no event is emitted by adding vote
func signAddVotesSynchronously(
	t *testing.T,
	kbft *KBFT,
	view uint64,
	msgType common.ConsensusMsgType,
	tsHash common.Hash,
	heights map[identifiers.Identifier]uint64,
	kramaVault ...*crypto.KramaVault,
) {
	t.Helper()

	if len(kramaVault) == 0 {
		require.FailNow(t, "there are no validators to sign")
	}

	votes := signVotes(t, kbft, view, msgType, tsHash, heights, kramaVault...)

	for i, vote := range votes {
		err := kbft.handleMsg(ktypes.ConsensusMessage{
			PeerID:  kramaVault[i].KramaID(),
			Payload: &ktypes.VoteMessage{Vote: vote},
		})
		require.NoError(t, err)
	}
}

func signAddVotes(
	t *testing.T,
	kbft *KBFT,
	view uint64,
	msgType common.ConsensusMsgType,
	tsHash common.Hash,
	heights map[identifiers.Identifier]uint64,
	kramaVault ...*crypto.KramaVault,
) {
	t.Helper()

	if len(kramaVault) == 0 {
		require.FailNow(t, "there are no validators to sign")
	}

	votes := signVotes(t, kbft, view, msgType, tsHash, heights, kramaVault...)

	for i, vote := range votes {
		kbft.inboundMsgChan <- ktypes.ConsensusMessage{
			PeerID:  kramaVault[i].KramaID(),
			Payload: &ktypes.VoteMessage{Vote: vote},
		}
	}
}

// sendAndEnsureVotes sends signed vote on outbound channel and ensures if consensus emitted vote event
func sendAndEnsureVotes(
	t *testing.T,
	kbft *KBFT,
	view uint64,
	msgType common.ConsensusMsgType,
	tsHash common.Hash,
	heights map[identifiers.Identifier]uint64,
	voteSub *utils.Subscription,
	expectedview uint64,
	kramaVault ...*crypto.KramaVault,
) {
	t.Helper()

	for _, v := range kramaVault {
		signAddVotes(t, kbft, view, msgType, tsHash, heights, v)
		ensureVote(t, voteSub, tsHash, heights, expectedview, msgType)
	}
}

// sendAndEnsurePreVote sends the prevote to kbft node and ensures that vote event emitted
func sendAndEnsurePreVote(
	t *testing.T,
	kbft *KBFT,
	view uint64,
	tsHash common.Hash,
	heights map[identifiers.Identifier]uint64,
	voteSub *utils.Subscription,
	expectedview uint64,
	kramaVault ...*crypto.KramaVault,
) {
	t.Helper()
	sendAndEnsureVotes(t, kbft, view, common.PREVOTE, tsHash, heights, voteSub, expectedview, kramaVault...)
}

// sendAndEnsurePrecommit sends the precommit to kbft node and ensures that vote event emitted
func sendAndEnsurePrecommit(
	t *testing.T,
	kbft *KBFT,
	view uint64,
	tsHash common.Hash,
	heights map[identifiers.Identifier]uint64,
	voteSub *utils.Subscription,
	expectedview uint64,
	kramaVault ...*crypto.KramaVault,
) {
	t.Helper()

	sendAndEnsureVotes(t, kbft, view, common.PRECOMMIT, tsHash, heights, voteSub, expectedview, kramaVault...)
}

func createIxs(t *testing.T, sender identifiers.Identifier, receiver identifiers.Identifier) common.Interactions {
	t.Helper()

	ixParams := map[int]*tests.CreateIxParams{
		0: tests.GetIxParamsWithAddress(t, sender, receiver),
	}

	return tests.CreateIxns(t, 1, ixParams)
}

// createTestClusterInfo takes icsNodes as input and makes them nodes of cluster
// It creates sender account meta info with current height and tesseract with new height
// It creates receiver account meta info with current height and tesseract with new height if receiver is not nil
// If nonRegisteredReceiver is true then we add sarga account to account meta infos
// and also add tesseract for sarga account
func createTestClusterInfo(
	t *testing.T,
	icsNodes *ktypes.ICSCommittee,
	newHeights map[identifiers.Identifier]uint64,
	ixs common.Interactions,
) *ktypes.ClusterState {
	t.Helper()

	ps := make(map[identifiers.Identifier]*common.Participant)
	pStates := make(common.ParticipantsState)

	ps[ixs[0].SenderID()] = &common.Participant{
		ID:         ixs[0].SenderID(),
		IsSigner:        true,
		Height:          newHeights[ixs[0].SenderID()] - 1,
		NodeSetPosition: 0,
		LockType:        common.MutateLock,
		ConsensusQuorum: 6,
	}

	pStates[ixs[0].SenderID()] = common.State{
		Height: newHeights[ixs[0].SenderID()],
	}

	if !ixs[0].Transaction(0).Target().IsNil() {
		ps[ixs[0].Transaction(0).Target()] = &common.Participant{
			Identifier:         ixs[0].Transaction(0).Target(),
			Height:          newHeights[ixs[0].Transaction(0).Target()] - 1,
			NodeSetPosition: 2,
			LockType:        common.MutateLock,
			ConsensusQuorum: 6,
		}

		pStates[ixs[0].Transaction(0).Target()] = common.State{
			Height: newHeights[ixs[0].Transaction(0).Target()],
		}
	}

	clusterInfo := ktypes.NewICS(
		nil,
		ixs,
		"cluster1",
		tests.RandomKramaIDs(t, 2)[0],
		time.Now(),
		tests.RandomKramaIDs(t, 2)[1],
		ps,
		icsNodes,
		common.LotteryKey{},
	)

	clusterInfo.SetTesseract(common.NewTesseract(
		pStates,
		common.NilHash,
		common.NilHash,
		big.NewInt(0),
		0,
		4,
		6,
		common.PoXtData{},
		nil,
		clusterInfo.SelfKramaID(),
		common.Interactions{},
		nil, nil,
	))

	return clusterInfo
}

// ensureProposal times out if proposal event not received in time
func ensureProposal(t *testing.T, proposals *utils.Subscription,
heights map[identifiers.Identifier]uint64, view uint64) {
	t.Helper()

	select {
	case <-time.After(ensureTimeout):
		require.FailNow(t, "Timeout expired while waiting for proposal event")
	case msg := <-proposals.Chan():
		proposalEvent, ok := msg.Data.(eventProposal)
		if !ok {
			require.FailNow(t, fmt.Sprintf("expected a eventProposal, got %T. Wrong subscription channel?",
				msg.Data))
		}

		proposal := proposalEvent.proposal
		if !areHeightsEqual(proposal.Height, heights) {
			require.FailNow(t, fmt.Sprintf("expected height %v, got %v", heights, proposal.Height))
		}

		if proposal.view != view {
			require.FailNow(t, fmt.Sprintf("expected view %v, got %v", view, proposal.view))
		}
	}
}

func validateviewState(
	t *testing.T,
	viewState eventDataViewState,
	heights map[identifiers.Identifier]uint64, view uint64,
	step ViewStepType,
) {
	t.Helper()

	if !areHeightsEqual(viewState.Height, heights) {
		require.FailNow(t, fmt.Sprintf("expected height %v, got %v", heights, viewState.Height))
	}

	if viewState.View != view {
		require.FailNow(t, fmt.Sprintf("expected view %v, got %v", view, viewState.View))
	}

	require.Equal(t, step.String(), viewState.Step)
}

func ensureNewview(t *testing.T, viewSub *utils.Subscription, heights map[identifiers.Identifier]uint64, view uint64) {
	t.Helper()

	select {
	case <-time.After(ensureTimeout):
		require.FailNow(t, "Timeout expired while waiting for Newview event")
	case msg := <-viewSub.Chan():
		newviewEvent, ok := msg.Data.(eventNewview)
		if !ok {
			require.FailNow(t, fmt.Sprintf("expected a eventNewview, got %T. Wrong subscription channel?",
				msg.Data))
		}

		validateviewState(t, newviewEvent.eventDataViewState, heights, view, ViewStepNewHeight)
	}
}

func ensurePolka(t *testing.T, polkaSub *utils.Subscription, heights map[identifiers.Identifier]uint64, view uint64) {
	t.Helper()

	select {
	case <-time.After(ensureTimeout):
		require.FailNow(t, "Timeout expired while waiting for polka event")
	case msg := <-polkaSub.Chan():
		polka, ok := msg.Data.(eventPolka)
		if !ok {
			require.FailNow(t, fmt.Sprintf("expected a eventPolka, got %T. Wrong subscription channel?",
				msg.Data))
		}

		validateviewState(t, polka.eventDataViewState, heights, view, ViewStepPrevote)
	}
}

func ensurePrevoteTimeout(
	t *testing.T,
	timeoutSub *utils.Subscription,
	heights map[identifiers.Identifier]uint64,
	view uint64,
	timeout int64,
) {
	t.Helper()

	timeoutDuration := time.Duration(timeout*10) * time.Nanosecond
	select {
	case <-time.After(timeoutDuration):
		require.FailNow(t, "timeout occurred while waiting for prevote time out wait")
	case msg := <-timeoutSub.Chan():
		eventPrevoteTimeout, ok := msg.Data.(eventTimeoutPrevote)
		if !ok {
			require.FailNow(t, fmt.Sprintf("expected a eventTimeoutPrevote, got %T. Wrong subscription channel?",
				msg.Data))
		}

		validateviewState(t, eventPrevoteTimeout.eventDataViewState, heights, view, viewStepPrevoteWait)
	}
}

func ensurePrecommitTimeout(
	t *testing.T,
	timeoutSub *utils.Subscription,
	heights map[identifiers.Identifier]uint64,
	view uint64,
	timeout int64,
) {
	t.Helper()

	timeoutDuration := time.Duration(timeout*10) * time.Nanosecond
	select {
	case <-time.After(timeoutDuration):
		require.FailNow(t, "timeout occurred while waiting for precommit time out wait")
	case msg := <-timeoutSub.Chan():
		eventPrecommitTimeout, ok := msg.Data.(eventTimeoutPrecommit)
		if !ok {
			require.FailNow(t, fmt.Sprintf("expected a eventTimeoutPrecommit, got %T. Wrong subscription channel?",
				msg.Data))
		}

		validateviewState(t, eventPrecommitTimeout.eventDataViewState, heights, view, ViewStepPrecommit)
	}
}

// ensureVote fails if vote is not received in ensureTimeout duration
func ensureVote(
	t *testing.T,
	voteSub *utils.Subscription,
	tsHash common.Hash,
	heights map[identifiers.Identifier]uint64,
	view uint64,
	voteType common.ConsensusMsgType,
) {
	t.Helper()

	select {
	case <-time.After(ensureTimeout):
		require.FailNow(t, "Timeout expired while waiting for NewVote event")
	case msg := <-voteSub.Chan():
		voteEvent, ok := msg.Data.(eventVote)
		if !ok {
			require.FailNow(t, fmt.Sprintf("expected a EventDataVote, got %T. Wrong subscription channel?",
				msg.Data))
		}

		vote := voteEvent.vote
		if vote.Type != voteType {
			require.FailNow(t, fmt.Sprintf("expected type %v, got %v", voteType, vote.Type))
		}

		if vote.View != view {
			require.FailNow(t, fmt.Sprintf("expected view %v, got %v", view, vote.View))
		}

		require.Equal(t, tsHash, vote.TSHash, "ts hashes doesn't match") // ensures hashes are equal
		require.Equal(t, len(heights), len(vote.Heights), "heights doesn't match")

		// ensures heights are equal
		for addr, expectedHeight := range heights {
			actualHeight, ok := vote.Heights[addr]
			require.True(t, ok)

			require.Equal(t, expectedHeight, actualHeight, "heights doesn't match")
		}
	}
}

func ensurePrevote(
	t *testing.T,
	voteSub *utils.Subscription,
	tsHash common.Hash,
	heights map[identifiers.Identifier]uint64,
	view uint64,
) {
	t.Helper()

	ensureVote(t, voteSub, tsHash, heights, view, common.PREVOTE)
}

func ensurePrecommit(
	t *testing.T,
	voteSub *utils.Subscription,
	tsHash common.Hash,
	heights map[identifiers.Identifier]uint64,
	view uint64,
) {
	t.Helper()

	ensureVote(t, voteSub, tsHash, heights, view, common.PRECOMMIT)
}

// ensureNoVoteReceived fails if a vote received within ensureTimeout duration
func ensureNoVoteReceived(t *testing.T, voteSub *utils.Subscription, message string) {
	t.Helper()

	select {
	case <-time.After(ensureTimeout):
		return
	case msg := <-voteSub.Chan():
		_, ok := msg.Data.(eventVote)
		if !ok {
			require.FailNow(t, fmt.Sprintf("expected a EventDataVote, got %T. Wrong subscription channel?",
				msg.Data))
		}

		require.FailNow(t, message)
	}
}

func ensureNoPrecommitReceived(t *testing.T, voteSub *utils.Subscription) {
	t.Helper()

	ensureNoVoteReceived(t, voteSub, "node shouldn't send precommit")
}

// validatePrevote fetches the prevote of validator in the current view and
// checks if voted ts hash matches
func validatePrevote(t *testing.T, kbft *KBFT, view uint64, val *crypto.KramaVault, tsHash common.Hash) {
	t.Helper()

	var valIndex int32

	var ok bool

	var vote *ktypes.Vote

	prevotes := kbft.Votes.getPrevotes(view)

	if valIndex, _, ok = kbft.ics.HasKramaID(val.KramaID()); !ok {
		require.FailNow(t, "failed to fetch krama id from ics")
	}

	if vote, ok = prevotes.getVote(valIndex, tsHash); !ok {
		require.FailNow(t, "failed to find prevote from validator")
	}

	if tsHash.IsNil() {
		if !vote.TSHash.IsNil() {
			require.FailNow(t, fmt.Sprintf("Expected prevote to be for nil, got %X", vote.TSHash))
		}

		return
	}

	require.Equal(t, tsHash, vote.TSHash)
}

// validatePrecommit fetches the precommit vote of validator in the current view and
// checks if voted ts hash, locked view, locked ts hash matches
func validatePrecommit(
	t *testing.T,
	kbft *KBFT,
	thisview,
	lockview uint64,
	val *crypto.KramaVault,
	tsHash,
	lockedTSHash common.Hash,
) {
	t.Helper()

	var (
		valIndex int32
		ok       bool
		vote     *ktypes.Vote
	)

	precommits := kbft.Votes.getPrecommits(thisview)

	if valIndex, _, ok = kbft.ics.HasKramaID(val.KramaID()); !ok {
		require.FailNow(t, "failed to fetch krama id from ics")
	}

	if vote, ok = precommits.getVote(valIndex, tsHash); !ok {
		require.FailNow(t, "failed to find precommit from validator")
	}

	if tsHash.IsNil() {
		if !vote.TSHash.IsNil() {
			require.FailNow(t, fmt.Sprintf("Expected prevote to be for nil, got %X", vote.TSHash))
		}

		return
	}

	require.Equal(t, tsHash, vote.TSHash)

	if lockedTSHash.IsNil() {
		if kbft.Lockedview != lockview || kbft.LockedTS != nil {
			require.FailNow(t, fmt.Sprintf(
				"Expected to be locked on nil at view %d. Got locked at view %d with block %v",
				lockview,
				kbft.Lockedview,
				kbft.LockedTS))
		}

		return
	}

	if kbft.Lockedview != lockview || !(kbft.LockedTS.Hash() == lockedTSHash) {
		require.FailNow(t, fmt.Sprintf(
			"Expected block to be locked on view %d, got %d. Got locked block %X, expected %X",
			lockview,
			kbft.Lockedview,
			kbft.LockedTS.Hash(),
			lockedTSHash))
	}
}

func ensureNoError(t *testing.T, kbftErr <-chan error) {
	t.Helper()

	err := <-kbftErr
	require.NoError(t, err)
}

func ensureError(t *testing.T, kbftErr <-chan error, expectedError string) {
	t.Helper()

	err := <-kbftErr
	require.EqualError(t, err, expectedError)
}
*/
