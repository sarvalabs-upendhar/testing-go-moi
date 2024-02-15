package kbft

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
func createTestNodeSet(t *testing.T, n int) (*common.NodeSet, []*crypto.KramaVault) {
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

	nodeset := common.NewNodeSet(kramaIDs, publicKeys, n)

	for i := 0; i < n; i++ {
		nodeset.Responses.SetIndex(i, true)
	}

	return nodeset, valset
}

// createTestRandomSet return nodeset and vaults
// nodeset has nodes info like krama ID and public key
// vaults has node info like krama ID and private key which can be used to sign votes during consensus
// total tells number of nodes which also includes nodes that are not part of ics
// and actual tells the number of nodes in ics
func createTestRandomSet(t *testing.T, total, actual int) (*common.NodeSet, []*crypto.KramaVault) {
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

	nodeset := common.NewNodeSet(kramaIDs, publicKeys, actual)

	for i := 0; i < actual; i++ {
		nodeset.Responses.SetIndex(i, true)
	}

	return nodeset, valset
}

// createICSNodes returns ICSNodes and vaults of given count of specific nodes
func createICSNodes(
	t *testing.T,
	senderBehaviourSetCount int,
	senderRandomSetCount int,
	receiverBehaviourSetCount int,
	receiverRandomSetCount int,
	randomSetCount int,
	observerSetCount int,
) (*common.ICSNodeSet, [][]*crypto.KramaVault) {
	t.Helper()

	senderBehaviourSet, senderBehaviouralValSet := createTestNodeSet(t, senderBehaviourSetCount)
	senderRandomSet, senderRandomValSet := createTestNodeSet(t, senderRandomSetCount)
	receiverBehaviourSet, receiverBehaviourValSet := createTestNodeSet(t, receiverBehaviourSetCount)
	receiverRandomSet, receiverRandomValSet := createTestNodeSet(t, receiverRandomSetCount)
	// create some grace nodes for random set but quorum field will have nodes that are only part of ICS
	randomSet, randomValSet := createTestRandomSet(t, 2*randomSetCount, randomSetCount)
	observerSet, observerValSet := createTestNodeSet(t, observerSetCount)

	testNodeSets := []*common.NodeSet{
		senderBehaviourSet,
		senderRandomSet,
		receiverBehaviourSet,
		receiverRandomSet,
		randomSet,
		observerSet,
	}

	valset := [][]*crypto.KramaVault{
		senderBehaviouralValSet,
		senderRandomValSet,
		receiverBehaviourValSet,
		receiverRandomValSet,
		randomValSet,
		observerValSet,
	}

	return &common.ICSNodeSet{
		Nodes: testNodeSets,
		Size: senderBehaviourSetCount + senderRandomSetCount + receiverBehaviourSetCount +
			receiverRandomSetCount + randomSetCount + observerSetCount,
	}, valset
}

func startTestRound(state *KBFT, heights map[identifiers.Address]uint64, round int32, err chan<- error) {
	state.enterNewRound(heights, round)

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
	round int32,
	msgType ktypes.ConsensusMsgType,
	tsHash common.Hash,
	heights map[identifiers.Address]uint64,
	kramaVault *crypto.KramaVault,
) *ktypes.Vote {
	t.Helper()

	valIndex, _ := kbft.ics.HasKramaID(kramaVault.KramaID())
	require.NotEqual(t, valIndex, -1)

	v := &ktypes.Vote{
		ValidatorIndex:   valIndex,
		ValidatorKramaID: kramaVault.KramaID(),
		TSHash:           tsHash,
		Heights:          heights,
		Round:            round,
		Type:             msgType,
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
	round int32,
	msgType ktypes.ConsensusMsgType,
	tsHash common.Hash,
	heights map[identifiers.Address]uint64,
	kramaVault ...*crypto.KramaVault,
) []*ktypes.Vote {
	t.Helper()

	votes := make([]*ktypes.Vote, len(kramaVault))

	for i, kVault := range kramaVault {
		votes[i] = signVote(t, kbft, round, msgType, tsHash, heights, kVault)
	}

	return votes
}

// signAddVotesSynchronously need to be used when no event is emitted by adding vote
func signAddVotesSynchronously(
	t *testing.T,
	kbft *KBFT,
	round int32,
	msgType ktypes.ConsensusMsgType,
	tsHash common.Hash,
	heights map[identifiers.Address]uint64,
	kramaVault ...*crypto.KramaVault,
) {
	t.Helper()

	if len(kramaVault) == 0 {
		require.FailNow(t, "there are no validators to sign")
	}

	votes := signVotes(t, kbft, round, msgType, tsHash, heights, kramaVault...)

	for i, vote := range votes {
		err := kbft.handleMsg(ktypes.ConsensusMessage{
			PeerID:  kramaVault[i].KramaID(),
			Message: &ktypes.VoteMessage{Vote: vote},
		})
		require.NoError(t, err)
	}
}

func signAddVotes(
	t *testing.T,
	kbft *KBFT,
	round int32,
	msgType ktypes.ConsensusMsgType,
	tsHash common.Hash,
	heights map[identifiers.Address]uint64,
	kramaVault ...*crypto.KramaVault,
) {
	t.Helper()

	if len(kramaVault) == 0 {
		require.FailNow(t, "there are no validators to sign")
	}

	votes := signVotes(t, kbft, round, msgType, tsHash, heights, kramaVault...)

	for i, vote := range votes {
		kbft.inboundMsgChan <- ktypes.ConsensusMessage{
			PeerID:  kramaVault[i].KramaID(),
			Message: &ktypes.VoteMessage{Vote: vote},
		}
	}
}

// sendAndEnsureVotes sends signed vote on outbound channel and ensures if consensus emitted vote event
func sendAndEnsureVotes(
	t *testing.T,
	kbft *KBFT,
	round int32,
	msgType ktypes.ConsensusMsgType,
	tsHash common.Hash,
	heights map[identifiers.Address]uint64,
	voteSub *utils.Subscription,
	expectedRound int32,
	kramaVault ...*crypto.KramaVault,
) {
	t.Helper()

	for _, v := range kramaVault {
		signAddVotes(t, kbft, round, msgType, tsHash, heights, v)
		ensureVote(t, voteSub, tsHash, heights, expectedRound, msgType)
	}
}

// sendAndEnsurePreVote sends the prevote to kbft node and ensures that vote event emitted
func sendAndEnsurePreVote(
	t *testing.T,
	kbft *KBFT,
	round int32,
	tsHash common.Hash,
	heights map[identifiers.Address]uint64,
	voteSub *utils.Subscription,
	expectedRound int32,
	kramaVault ...*crypto.KramaVault,
) {
	t.Helper()

	sendAndEnsureVotes(t, kbft, round, ktypes.PREVOTE, tsHash, heights, voteSub, expectedRound, kramaVault...)
}

// sendAndEnsurePrecommit sends the precommit to kbft node and ensures that vote event emitted
func sendAndEnsurePrecommit(
	t *testing.T,
	kbft *KBFT,
	round int32,
	tsHash common.Hash,
	heights map[identifiers.Address]uint64,
	voteSub *utils.Subscription,
	expectedRound int32,
	kramaVault ...*crypto.KramaVault,
) {
	t.Helper()

	sendAndEnsureVotes(t, kbft, round, ktypes.PRECOMMIT, tsHash, heights, voteSub, expectedRound, kramaVault...)
}

func createIxs(t *testing.T, sender identifiers.Address, receiver identifiers.Address) common.Interactions {
	t.Helper()

	ixParams := map[int]*tests.CreateIxParams{
		0: tests.GetIxParamsWithAddress(sender, receiver),
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
	icsNodes *common.ICSNodeSet,
	newHeights map[identifiers.Address]uint64,
	ixs common.Interactions,
	nonRegisteredReceiver bool,
) *ktypes.ClusterState {
	t.Helper()

	clusterInfo := ktypes.NewICS(
		0,
		nil,
		ixs,
		"cluster1",
		tests.RandomKramaIDs(t, 2)[0],
		time.Now(),
		tests.RandomKramaIDs(t, 2)[1],
	)

	func(clusterInfo *ktypes.ClusterState) {
		clusterInfo.NodeSet = icsNodes

		clusterInfo.AccountInfos = make(map[identifiers.Address]*ktypes.AccountInfo)
		clusterInfo.AccountInfos[ixs[0].Sender()] = &ktypes.AccountInfo{
			Height: newHeights[ixs[0].Sender()] - 1,
		}

		if nonRegisteredReceiver && !ixs[0].Receiver().IsNil() {
			clusterInfo.AccountInfos[common.SargaAddress] = &ktypes.AccountInfo{
				Height: newHeights[common.SargaAddress] - 1,
			}
			clusterInfo.AccountInfos[ixs[0].Receiver()] = &ktypes.AccountInfo{
				Address:       ixs[0].Receiver(),
				AccType:       common.AccTypeFromIxType(ixs[0].Type()),
				TesseractHash: common.NilHash,
				IsGenesis:     true,
				Height:        0,
			}
		} else if !ixs[0].Receiver().IsNil() {
			clusterInfo.AccountInfos[ixs[0].Receiver()] = &ktypes.AccountInfo{
				Height: newHeights[ixs[0].Receiver()] - 1,
			}
		}

		p := make(common.Participants)

		p[ixs[0].Sender()] = common.State{
			Height: newHeights[ixs[0].Sender()],
		}

		if !ixs[0].Receiver().IsNil() {
			p[ixs[0].Receiver()] = common.State{
				Height: newHeights[ixs[0].Receiver()],
			}
		}

		if nonRegisteredReceiver {
			p[common.SargaAddress] = common.State{
				Height: newHeights[common.SargaAddress],
			}
		}

		clusterInfo.Tesseract = common.NewTesseract(
			p,
			common.NilHash,
			common.NilHash,
			big.NewInt(0),
			0,
			"operator",
			4,
			6,
			common.PoXtData{},
			nil,
			clusterInfo.SelfKramaID(),
			nil,
			nil,
		)
	}(clusterInfo)

	return clusterInfo
}

// ensureProposal times out if proposal event not received in time
func ensureProposal(t *testing.T, proposals *utils.Subscription, heights map[identifiers.Address]uint64, round int32) {
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

		if proposal.Round != round {
			require.FailNow(t, fmt.Sprintf("expected round %v, got %v", round, proposal.Round))
		}
	}
}

func validateRoundState(
	t *testing.T,
	roundState eventDataRoundState,
	heights map[identifiers.Address]uint64, round int32,
	step RoundStepType,
) {
	t.Helper()

	if !areHeightsEqual(roundState.Height, heights) {
		require.FailNow(t, fmt.Sprintf("expected height %v, got %v", heights, roundState.Height))
	}

	if roundState.Round != round {
		require.FailNow(t, fmt.Sprintf("expected round %v, got %v", round, roundState.Round))
	}

	require.Equal(t, step.String(), roundState.Step)
}

func ensureNewRound(t *testing.T, roundSub *utils.Subscription, heights map[identifiers.Address]uint64, round int32) {
	t.Helper()

	select {
	case <-time.After(ensureTimeout):
		require.FailNow(t, "Timeout expired while waiting for NewRound event")
	case msg := <-roundSub.Chan():
		newRoundEvent, ok := msg.Data.(eventNewRound)
		if !ok {
			require.FailNow(t, fmt.Sprintf("expected a eventNewRound, got %T. Wrong subscription channel?",
				msg.Data))
		}

		validateRoundState(t, newRoundEvent.eventDataRoundState, heights, round, RoundStepNewHeight)
	}
}

func ensurePolka(t *testing.T, polkaSub *utils.Subscription, heights map[identifiers.Address]uint64, round int32) {
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

		validateRoundState(t, polka.eventDataRoundState, heights, round, RoundStepPrevote)
	}
}

func ensurePrevoteTimeout(
	t *testing.T,
	timeoutSub *utils.Subscription,
	heights map[identifiers.Address]uint64,
	round int32,
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

		validateRoundState(t, eventPrevoteTimeout.eventDataRoundState, heights, round, RoundStepPrevoteWait)
	}
}

func ensurePrecommitTimeout(
	t *testing.T,
	timeoutSub *utils.Subscription,
	heights map[identifiers.Address]uint64,
	round int32,
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

		validateRoundState(t, eventPrecommitTimeout.eventDataRoundState, heights, round, RoundStepPrecommit)
	}
}

// ensureVote fails if vote is not received in ensureTimeout duration
func ensureVote(
	t *testing.T,
	voteSub *utils.Subscription,
	tsHash common.Hash,
	heights map[identifiers.Address]uint64,
	round int32,
	voteType ktypes.ConsensusMsgType,
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

		if vote.Round != round {
			require.FailNow(t, fmt.Sprintf("expected round %v, got %v", round, vote.Round))
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
	heights map[identifiers.Address]uint64,
	round int32,
) {
	t.Helper()

	ensureVote(t, voteSub, tsHash, heights, round, ktypes.PREVOTE)
}

func ensurePrecommit(
	t *testing.T,
	voteSub *utils.Subscription,
	tsHash common.Hash,
	heights map[identifiers.Address]uint64,
	round int32,
) {
	t.Helper()

	ensureVote(t, voteSub, tsHash, heights, round, ktypes.PRECOMMIT)
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

	ensureNoVoteReceived(t, voteSub, "node shouln't send precommit")
}

// validatePrevote fetches the prevote of validator in the current round and
// checks if voted ts hash matches
func validatePrevote(t *testing.T, kbft *KBFT, round int32, val *crypto.KramaVault, tsHash common.Hash) {
	t.Helper()

	var kramaID int32

	var ok bool

	var vote *ktypes.Vote

	prevotes := kbft.Votes.getPrevotes(round)

	if kramaID, ok = kbft.ics.HasKramaID(val.KramaID()); !ok {
		require.FailNow(t, "Failed to fetch krama id from ics")
	}

	if vote, ok = prevotes.getVote(kramaID, tsHash); !ok {
		require.FailNow(t, "Failed to find prevote from validator")
	}

	if tsHash.IsNil() {
		if !vote.TSHash.IsNil() {
			require.FailNow(t, fmt.Sprintf("Expected prevote to be for nil, got %X", vote.TSHash))
		}

		return
	}

	require.Equal(t, tsHash, vote.TSHash)
}

// validatePrecommit fetches the precommit vote of validator in the current round and
// checks if voted ts hash, locked round, locked ts hash matches
func validatePrecommit(
	t *testing.T,
	kbft *KBFT,
	thisRound,
	lockRound int32,
	val *crypto.KramaVault,
	tsHash,
	lockedTSHash common.Hash,
) {
	t.Helper()

	var (
		kramaID int32
		ok      bool
		vote    *ktypes.Vote
	)

	precommits := kbft.Votes.getPrecommits(thisRound)

	if kramaID, ok = kbft.ics.HasKramaID(val.KramaID()); !ok {
		require.FailNow(t, "Failed to fetch krama id from ics")
	}

	if vote, ok = precommits.getVote(kramaID, tsHash); !ok {
		require.FailNow(t, "Failed to find precommit from validator")
	}

	if tsHash.IsNil() {
		if !vote.TSHash.IsNil() {
			require.FailNow(t, fmt.Sprintf("Expected prevote to be for nil, got %X", vote.TSHash))
		}

		return
	}

	require.Equal(t, tsHash, vote.TSHash)

	if lockedTSHash.IsNil() {
		if kbft.LockedRound != lockRound || kbft.LockedTS != nil {
			require.FailNow(t, fmt.Sprintf(
				"Expected to be locked on nil at round %d. Got locked at round %d with block %v",
				lockRound,
				kbft.LockedRound,
				kbft.LockedTS))
		}

		return
	}

	if kbft.LockedRound != lockRound || !(kbft.LockedTS.Hash() == lockedTSHash) {
		require.FailNow(t, fmt.Sprintf(
			"Expected block to be locked on round %d, got %d. Got locked block %X, expected %X",
			lockRound,
			kbft.LockedRound,
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
