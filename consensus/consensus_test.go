package consensus

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/common/utils"
	"github.com/sarvalabs/go-moi/crypto"
	"github.com/sarvalabs/go-moi/network/message"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/consensus/kbft"
	"github.com/sarvalabs/go-moi/consensus/types"
	"github.com/stretchr/testify/require"
)

// Standard test setup format:
// 1. Define the total number of participants across multiple views.
// 2. Specify the number of context nodes per participant.
// 3. Create vaults for all nodes to sign votes.
// 4. Initialize the ICS node set using the created vaults.
// 5. Select the operator node for the view.
// 6. Create identifiers and their corresponding genesis tesseracts.
// 7. Store the genesis tesseracts.
// 8. Generate the required interaction (ixn) for the view.
// 9. Initialize the cluster with:
//    - Loaded view information
//    - Participants
//    - Interactions (ixns)
//    - ICS committee (from context nodes' vaults)
//    - Random cluster ID
// 10. Ensure the ICS committee passed to the cluster is formed from the context node vaults.
// 11. Create the slot on the operator and store the cluster state within it.
// 12. Start the operator's handler to listen for external events.
// 13. Launch the ICS handler and signal it using a new ICS channel associated with the cluster ID.
// 14. The operator sends a `Prepare` message to the ICS committee.
// 15. Listen and respond to events: `Prepare`, `Proposal`, `Prevote`, and `Precommit`, based on test requirements.
//
// Additional notes for multi-node test cases:
// 16. On validator instances, store the ICS committee to initiate consensus locally.
// 17. If a node needs to fetch a tesseract (i.e., if it is not already available):
//     - Validate the tesseract.
//     - Store the ICS committee keyed by the interaction (ixn) hash for validation.
// 18. Update view numbers in Krama instances when transitioning to a new view.
// 19. To control partitions, exclude packets while registering nodes with each other.
// 20. Exiting routines:
//     20.1 Use the exit ICS handler method to stop only the ICS handler routine,
//          ensuring the KBFT routine is not started.
//     20.2 To exit both the ICS handler and the KBFT routine, cancel the context of the Krama instance.

// Format explanation for the consensus trace:
// Example:
// View-1 (operator-N0)
// B0 B0 B0 B0 - B0 B0 B0 B0 // N0 - B1 // B1 B1 B1 B1 - B1 B1 B1 B1 // B1 NIL NIL NIL - NIL NIL NIL NIL
//
// Legend:
// - "View-1" represents both the operator’s view and the validators’ view.
// - Each participant has 4 context nodes; there are two participants in this example.
// - A hyphen ("-") separates the context nodes of different participants.
// - A double slash ("//") separates transitions between consensus stages, in the order:
//   prepared messages // operator-proposed block // prevotes // precommits // commit.

// TestFullRound_WithMultipleNodes verifies that consensus can be achieved for an ixn
// when one node acts as the operator. The operator runs independently, while validator
// behavior is simulated in code.
func TestFullRound_WithMultipleNodes(t *testing.T) {
	t.Parallel()

	wg := sync.WaitGroup{}
	ics, vaults := createICSCommittee(t, 2, 4, 5)

	viewID := uint64(1)
	operatorVault := vaults[0][0]
	k := newKramaInstance(t, 0, operatorVault)

	k.currentView.SetID(viewID)

	var (
		prepareSub  = k.consensusMux.Subscribe(eventPrepare{})
		proposalSub = k.consensusMux.Subscribe(kbft.EventProposal{})
		voteSub     = k.consensusMux.Subscribe(kbft.EventVote{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	viewInfos, err := k.loadViewInfo(ids)
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k, ids)
	require.NoError(t, err)

	clusterState := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k.selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k.pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot, _ := k.slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot.UpdateClusterState(clusterState)

	go startHandler(k, &wg)
	go startICSHandler(k, &wg, k.ctx, clusterID)

	slot.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub, viewID, ixns.Hashes())
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos, vaults.GetVaults(0, 2, 0)...)
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub, viewID, viewInfos, 6, clusterState)

	tsHash := clusterState.Tesseract().Hash()

	ensureVote(t, voteSub, viewID, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub, viewID, common.PREVOTE, tsHash, true)

	ensureVote(t, voteSub, viewID, common.PRECOMMIT, tsHash, false)

	sendVoteMsg(t, k, viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub, viewID, common.PRECOMMIT, tsHash, true)

	ensureEmptySlots(t, k)
	checkForTS(t, k, clusterState.Tesseract().Hash())

	k.ctxCancel()
	wait(t, &wg)
}

// TestFullRound_WithLessThanContextQuorumPrepared verifies that the operator
// does not enter proposal stage without receiving a quorum of prepared votes from any context set.
func TestFullRound_WithLessThanContextQuorumPrepared(t *testing.T) {
	t.Parallel()

	wg := sync.WaitGroup{}
	ics, vaults := createICSCommittee(t, 2, 4, 5)

	operatorVault := vaults[0][0]
	viewID := uint64(1)
	k := newKramaInstance(t, 0, operatorVault)

	k.currentView.SetID(viewID)

	var (
		prepareSub  = k.consensusMux.Subscribe(eventPrepare{})
		proposalSub = k.consensusMux.Subscribe(kbft.EventProposal{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k)
	require.NoError(t, err)

	viewInfos, err := k.loadViewInfo(ids)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k, ids)
	require.NoError(t, err)

	clusterState := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k.selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k.pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot, _ := k.slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot.UpdateClusterState(clusterState)

	go startHandler(k, &wg)
	go startICSHandler(k, &wg, k.ctx, clusterID)

	slot.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub, viewID, ixns.Hashes())
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 2, 0)...)
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos, vaults.GetVaults(1, 2)...)

	ensureNoEventReceived(t, proposalSub)
	exitICSHandler(slot)
	ensureEmptySlots(t, k)

	k.ctxCancel()
	wait(t, &wg)
}

// TestFullRound_WithLessThanQuorumContextPrevotes verifies that the operator
// does not enter precommit stage without receiving a quorum of prevotes from context set.
func TestFullRound_WithLessThanQuorumContextPrevotes(t *testing.T) { //nolint:dupl
	t.Parallel()

	wg := sync.WaitGroup{}
	ics, vaults := createICSCommittee(t, 2, 4, 5)

	operatorVault := vaults[0][0]
	viewID := uint64(1)
	k := newKramaInstance(t, 0, operatorVault)

	k.currentView.SetID(viewID)

	var (
		prepareSub  = k.consensusMux.Subscribe(eventPrepare{})
		proposalSub = k.consensusMux.Subscribe(kbft.EventProposal{})
		voteSub     = k.consensusMux.Subscribe(kbft.EventVote{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k)
	require.NoError(t, err)

	viewInfos, err := k.loadViewInfo(ids)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k, ids)
	require.NoError(t, err)

	clusterState := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k.selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k.pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot, _ := k.slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot.UpdateClusterState(clusterState)

	go startHandler(k, &wg)
	go startICSHandler(k, &wg, k.ctx, clusterID)
	slot.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub, viewID, ixns.Hashes())
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 2, 0)...)
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub, viewID, viewInfos, 6, clusterState)

	tsHash := clusterState.Tesseract().Hash()

	ensureVote(t, voteSub, viewID, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(1, 2)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(2, 4)...)

	ensureNoEventReceived(t, voteSub)
	exitICSHandler(slot)
	ensureEmptySlots(t, k)

	k.ctxCancel()
	wait(t, &wg)
}

// TestFullRound_WithLessThanQuorumRandomPrevotes verifies that the operator
// does not enter precommit stage without receiving a quorum of prevotes from random set.
func TestFullRound_WithLessThanQuorumRandomPrevotes(t *testing.T) { //nolint:dupl
	t.Parallel()

	wg := sync.WaitGroup{}
	ics, vaults := createICSCommittee(t, 2, 4, 5)

	operatorVault := vaults[0][0]
	viewID := uint64(1)
	k := newKramaInstance(t, 0, operatorVault)

	k.currentView.SetID(viewID)

	var (
		prepareSub  = k.consensusMux.Subscribe(eventPrepare{})
		proposalSub = k.consensusMux.Subscribe(kbft.EventProposal{})
		voteSub     = k.consensusMux.Subscribe(kbft.EventVote{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k)
	require.NoError(t, err)

	viewInfos, err := k.loadViewInfo(ids)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k, ids)
	require.NoError(t, err)

	clusterState := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k.selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k.pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot, _ := k.slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot.UpdateClusterState(clusterState)

	go startHandler(k, &wg)
	go startICSHandler(k, &wg, k.ctx, clusterID)
	slot.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub, viewID, ixns.Hashes())
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 2, 0)...)
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub, viewID, viewInfos, 6, clusterState)

	tsHash := clusterState.Tesseract().Hash()

	ensureVote(t, voteSub, viewID, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(2, 3)...)

	ensureNoEventReceived(t, voteSub)
	exitICSHandler(slot)
	ensureEmptySlots(t, k)

	k.ctxCancel()
	wait(t, &wg)
}

// TestFullRound_WithLessThanContextQuorumPrecommits verifies that the operator
// does not enter commit stage without receiving a quorum of precommits from context set.
func TestFullRound_WithLessThanContextQuorumPrecommits(t *testing.T) { //nolint:dupl
	t.Parallel()

	wg := sync.WaitGroup{}
	ics, vaults := createICSCommittee(t, 2, 4, 5)

	operatorVault := vaults[0][0]
	viewID := uint64(1)
	k := newKramaInstance(t, 0, operatorVault)

	k.currentView.SetID(viewID)

	var (
		prepareSub  = k.consensusMux.Subscribe(eventPrepare{})
		proposalSub = k.consensusMux.Subscribe(kbft.EventProposal{})
		voteSub     = k.consensusMux.Subscribe(kbft.EventVote{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k)
	require.NoError(t, err)

	viewInfos, err := k.loadViewInfo(ids)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k, ids)
	require.NoError(t, err)

	clusterState := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k.selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k.pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot, _ := k.slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot.UpdateClusterState(clusterState)

	go startHandler(k, &wg)
	go startICSHandler(k, &wg, k.ctx, clusterID)
	slot.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub, viewID, ixns.Hashes())
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 2, 0)...)
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub, viewID, viewInfos, 6, clusterState)

	tsHash := clusterState.Tesseract().Hash()

	ensureVote(t, voteSub, viewID, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub, viewID, common.PREVOTE, tsHash, true)

	ensureVote(t, voteSub, viewID, common.PRECOMMIT, tsHash, false)

	sendVoteMsg(t, k, viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState, vaults.GetVaults(1, 2)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState, vaults.GetVaults(2, 4)...)

	ensureNoEventReceived(t, voteSub)
	exitICSHandler(slot)
	ensureEmptySlots(t, k)

	k.ctxCancel()
	wait(t, &wg)
}

// TestFullRound_WithLessThanRandomQuorumPrecommits verifies that the operator
// does not enter commit stage without receiving a quorum of precommits from random set.
func TestFullRound_WithLessThanRandomQuorumPrecommits(t *testing.T) { //nolint:dupl
	t.Parallel()

	wg := sync.WaitGroup{}
	ics, vaults := createICSCommittee(t, 2, 4, 5)

	operatorVault := vaults[0][0]
	viewID := uint64(1)
	k := newKramaInstance(t, 0, operatorVault)

	k.currentView.SetID(viewID)

	var (
		prepareSub  = k.consensusMux.Subscribe(eventPrepare{})
		proposalSub = k.consensusMux.Subscribe(kbft.EventProposal{})
		voteSub     = k.consensusMux.Subscribe(kbft.EventVote{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k)
	require.NoError(t, err)

	viewInfos, err := k.loadViewInfo(ids)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k, ids)
	require.NoError(t, err)

	clusterState := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k.selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k.pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot, _ := k.slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot.UpdateClusterState(clusterState)

	go startHandler(k, &wg)
	go startICSHandler(k, &wg, k.ctx, clusterID)
	slot.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub, viewID, ixns.Hashes())
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 2, 0)...)
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub, viewID, viewInfos, 6, clusterState)

	tsHash := clusterState.Tesseract().Hash()

	ensureVote(t, voteSub, viewID, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub, viewID, common.PREVOTE, tsHash, true)

	ensureVote(t, voteSub, viewID, common.PRECOMMIT, tsHash, false)

	sendVoteMsg(t, k, viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState, vaults.GetVaults(2, 3)...)

	ensureNoEventReceived(t, voteSub)
	exitICSHandler(slot)
	ensureEmptySlots(t, k)

	k.ctxCancel()
	wait(t, &wg)
}

// TestFullRound_WithLessThanContextQuorumPrepared verifies that the operator
// does not enter proposal stage without receiving a quorum of valid prepared votes from context set.
// This test injects an invalid prepared message with an incorrect view.
func TestFullRound_WithInvalidPrepared(t *testing.T) {
	t.Parallel()

	wg := sync.WaitGroup{}
	ics, vaults := createICSCommittee(t, 2, 4, 5)

	operatorVault := vaults[0][0]
	viewID := uint64(1)
	k := newKramaInstance(t, 0, operatorVault)

	k.currentView.SetID(viewID)

	var (
		prepareSub  = k.consensusMux.Subscribe(eventPrepare{})
		proposalSub = k.consensusMux.Subscribe(kbft.EventProposal{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k)
	require.NoError(t, err)

	viewInfos, err := k.loadViewInfo(ids)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k, ids)
	require.NoError(t, err)

	clusterState := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k.selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k.pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false)

	slot, _ := k.slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot.UpdateClusterState(clusterState)

	go startHandler(k, &wg)
	go startICSHandler(k, &wg, k.ctx, clusterID)
	slot.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub, viewID, ixns.Hashes())
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 1, 0)...) // send invalid view in prepared msg
	sendPreparedMsg(t, k, clusterID, 9, viewInfos, vaults[0][2:3]...)
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos, vaults.GetVaults(1, 3)...)

	ensureNoEventReceived(t, proposalSub)
	exitICSHandler(slot)
	ensureEmptySlots(t, k)

	k.ctxCancel()
	wait(t, &wg)
}

// TestFullRound_WithLessThanContextQuorumPrevote verifies that the operator
// does not enter precommit stage without receiving a quorum of valid prevotes from context set.
// This test injects an invalid prevote with a random tesseract hash.
func TestFullRound_WithInvalidPrevote(t *testing.T) {
	t.Parallel()

	wg := sync.WaitGroup{}
	ics, vaults := createICSCommittee(t, 2, 4, 5)

	operatorVault := vaults[0][0]
	viewID := uint64(1)
	k := newKramaInstance(t, 0, operatorVault)

	k.currentView.SetID(viewID)

	var (
		prepareSub  = k.consensusMux.Subscribe(eventPrepare{})
		proposalSub = k.consensusMux.Subscribe(kbft.EventProposal{})
		voteSub     = k.consensusMux.Subscribe(kbft.EventVote{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k)
	require.NoError(t, err)

	viewInfos, err := k.loadViewInfo(ids)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k, ids)
	require.NoError(t, err)

	clusterState := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k.selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k.pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot, _ := k.slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot.UpdateClusterState(clusterState)

	go startHandler(k, &wg)
	go startICSHandler(k, &wg, k.ctx, clusterID)
	slot.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub, viewID, ixns.Hashes())
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 2, 0)...)
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub, viewID, viewInfos, 6, clusterState)

	tsHash := clusterState.Tesseract().Hash()

	ensureVote(t, voteSub, viewID, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(1, 2)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tests.RandomHash(t), clusterState, vaults[1][2])
	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(2, 4)...)

	ensureNoEventReceived(t, voteSub)
	exitICSHandler(slot)
	ensureEmptySlots(t, k)

	k.ctxCancel()
	wait(t, &wg)
}

// TestFullRound_WithLessThanContextQuorumPrecommit verifies that the operator
// does not enter commit stage without receiving a quorum of valid precommits from context set.
// This test injects an invalid precommit with a random tesseract hash.
func TestFullRound_WithInvalidPrecommit(t *testing.T) {
	t.Parallel()

	wg := sync.WaitGroup{}
	ics, vaults := createICSCommittee(t, 2, 4, 5)

	operatorVault := vaults[0][0]
	viewID := uint64(1)
	k := newKramaInstance(t, 0, operatorVault)

	k.currentView.SetID(viewID)

	var (
		prepareSub  = k.consensusMux.Subscribe(eventPrepare{})
		proposalSub = k.consensusMux.Subscribe(kbft.EventProposal{})
		voteSub     = k.consensusMux.Subscribe(kbft.EventVote{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k)
	require.NoError(t, err)

	viewInfos, err := k.loadViewInfo(ids)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k, ids)
	require.NoError(t, err)

	clusterState := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k.selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k.pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot, _ := k.slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot.UpdateClusterState(clusterState)

	go startHandler(k, &wg)
	go startICSHandler(k, &wg, k.ctx, clusterID)
	slot.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub, viewID, ixns.Hashes())
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 2, 0)...)
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub, viewID, viewInfos, 6, clusterState)

	tsHash := clusterState.Tesseract().Hash()

	ensureVote(t, voteSub, viewID, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub, viewID, common.PREVOTE, tsHash, true)

	ensureVote(t, voteSub, viewID, common.PRECOMMIT, tsHash, false)

	sendVoteMsg(t, k, viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState, vaults.GetVaults(1, 2)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PRECOMMIT,
		tests.RandomHash(t), clusterState, vaults[1][2:]...)
	sendVoteMsg(t, k, viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState, vaults.GetVaults(2, 4)...)

	ensureNoEventReceived(t, voteSub)
	exitICSHandler(slot)
	ensureEmptySlots(t, k)

	k.ctxCancel()
	wait(t, &wg)
}

// TestFullRound_WithDuplicatePrepared ensures that a duplicate prepare message is not counted as a double vote.
func TestFullRound_WithDuplicatePrepared(t *testing.T) {
	t.Parallel()

	wg := sync.WaitGroup{}
	ics, vaults := createICSCommittee(t, 2, 4, 5)

	operatorVault := vaults[0][0]
	viewID := uint64(1)
	k := newKramaInstance(t, 0, operatorVault)
	k.currentView.SetID(viewID)

	var (
		prepareSub  = k.consensusMux.Subscribe(eventPrepare{})
		proposalSub = k.consensusMux.Subscribe(kbft.EventProposal{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k)
	require.NoError(t, err)

	viewInfos, err := k.loadViewInfo(ids)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k, ids)
	require.NoError(t, err)

	clusterState := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k.selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k.pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot, _ := k.slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot.UpdateClusterState(clusterState)

	go startHandler(k, &wg)
	go startICSHandler(k, &wg, k.ctx, clusterID)
	slot.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub, viewID, ixns.Hashes())
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 1, 0)...)
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 1, 0)...)
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos, vaults.GetVaults(1, 3)...)

	ensureNoEventReceived(t, proposalSub)
	exitICSHandler(slot)
	ensureEmptySlots(t, k)

	k.ctxCancel()
	wait(t, &wg)
}

// TestFullRound_WithDuplicatePrevote ensures that a duplicate prevote is not counted as a double vote.
func TestFullRound_WithDuplicatePrevote(t *testing.T) {
	t.Parallel()

	wg := sync.WaitGroup{}
	ics, vaults := createICSCommittee(t, 2, 4, 5)

	operatorVault := vaults[0][0]
	viewID := uint64(1)
	k := newKramaInstance(t, 0, operatorVault)

	k.currentView.SetID(viewID)

	var (
		prepareSub  = k.consensusMux.Subscribe(eventPrepare{})
		proposalSub = k.consensusMux.Subscribe(kbft.EventProposal{})
		voteSub     = k.consensusMux.Subscribe(kbft.EventVote{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k)
	require.NoError(t, err)

	viewInfos, err := k.loadViewInfo(ids)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k, ids)
	require.NoError(t, err)

	clusterState := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k.selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k.pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot, _ := k.slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot.UpdateClusterState(clusterState)

	go startHandler(k, &wg)
	go startICSHandler(k, &wg, k.ctx, clusterID)
	slot.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub, viewID, ixns.Hashes())
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 2, 0)...)
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub, viewID, viewInfos, 6, clusterState)

	tsHash := clusterState.Tesseract().Hash()

	ensureVote(t, voteSub, viewID, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(1, 2)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(1, 2)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(2, 4)...)

	ensureNoEventReceived(t, voteSub)
	exitICSHandler(slot)
	ensureEmptySlots(t, k)

	k.ctxCancel()
	wait(t, &wg)
}

// TestFullRound_WithDuplicatePrecommit ensures that a duplicate precommit is not counted as a double vote.
func TestFullRound_WithDuplicatePrecommit(t *testing.T) {
	t.Parallel()

	wg := sync.WaitGroup{}
	ics, vaults := createICSCommittee(t, 2, 4, 5)

	operatorVault := vaults[0][0]
	viewID := uint64(1)
	k := newKramaInstance(t, 0, operatorVault)

	k.currentView.SetID(viewID)

	var (
		prepareSub  = k.consensusMux.Subscribe(eventPrepare{})
		proposalSub = k.consensusMux.Subscribe(kbft.EventProposal{})
		voteSub     = k.consensusMux.Subscribe(kbft.EventVote{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k)
	require.NoError(t, err)

	viewInfos, err := k.loadViewInfo(ids)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k, ids)
	require.NoError(t, err)

	clusterState := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k.selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k.pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot, _ := k.slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot.UpdateClusterState(clusterState)

	go startHandler(k, &wg)
	go startICSHandler(k, &wg, k.ctx, clusterID)
	slot.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub, viewID, ixns.Hashes())
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 2, 0)...)
	sendPreparedMsg(t, k, clusterID, viewID, viewInfos, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub, viewID, viewInfos, 6, clusterState)

	tsHash := clusterState.Tesseract().Hash()

	ensureVote(t, voteSub, viewID, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PREVOTE,
		tsHash, clusterState, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub, viewID, common.PREVOTE, tsHash, true)

	ensureVote(t, voteSub, viewID, common.PRECOMMIT, tsHash, false)

	sendVoteMsg(t, k, viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState, vaults.GetVaults(1, 2)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState, vaults.GetVaults(1, 2)...)
	sendVoteMsg(t, k, viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState, vaults.GetVaults(2, 4)...)

	ensureNoEventReceived(t, voteSub)
	exitICSHandler(slot)
	ensureEmptySlots(t, k)

	k.ctxCancel()
	wait(t, &wg)
}

// TestFullRound_WithMultipleNodes_Validator verifies that consensus can be achieved for an ixn
// when one node acts as the operator and one acts as the validator. The operator and validator runs independently,
// while other validators behavior is simulated in code.
func TestFullRound_WithMultipleNodes_Validator(t *testing.T) {
	t.Parallel()

	wg := sync.WaitGroup{}
	ics, vaults := createICSCommittee(t, 2, 4, 5)

	operatorVault := vaults[0][0]
	viewID := uint64(1)
	k := createKramaInstances(t, vaults.GetVaults(0, 2))

	k[0].currentView.SetID(viewID)
	k[1].currentView.SetID(viewID)

	k[0].registerForTest(k[1])
	k[1].registerForTest(k[0])

	var (
		prepareSub0  = k[0].consensusMux.Subscribe(eventPrepare{})
		proposalSub  = k[0].consensusMux.Subscribe(kbft.EventProposal{})
		voteSub0     = k[0].consensusMux.Subscribe(kbft.EventVote{})
		preparedSub1 = k[1].consensusMux.Subscribe(eventPrepared{})
		voteSub1     = k[1].consensusMux.Subscribe(kbft.EventVote{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k...)
	require.NoError(t, err)

	viewInfos, err := k[0].loadViewInfo(ids)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k[0], ids)
	require.NoError(t, err)

	clusterState0 := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k[0].selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k[0].pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot0, _ := k[0].slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot0 == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot0.UpdateClusterState(clusterState0)

	ics1 := createICSCommitteeFromVaults(vaults, []int{0, 1, 2})

	ixnsHash, err := ixns.Hash()
	require.NoError(t, err)

	k[1].storeICSCommitteeForTest(ixnsHash, ics1)
	storeIxns(k[1].pool, &ixns)

	go startHandler(k[0], &wg)
	go startHandler(k[1], &wg)

	go startICSHandler(k[0], &wg, k[0].ctx, clusterID)
	slot0.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub0, viewID, ixns.Hashes())

	k[1].prepareTimeout <- struct{}{}

	ensurePrepared(t, preparedSub1, viewID, viewInfos)

	sendPreparedMsg(t, k[0], clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 1, 0, 1)...)
	sendPreparedMsg(t, k[0], clusterID, viewID, viewInfos, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub, viewID, viewInfos, 6, clusterState0)

	tsHash := clusterState0.Tesseract().Hash()

	ensureVote(t, voteSub0, viewID, common.PREVOTE, tsHash, false)

	ensureVote(t, voteSub1, viewID, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(0, 1, 0, 1)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub0, viewID, common.PREVOTE, tsHash, true)

	ensureVote(t, voteSub0, viewID, common.PRECOMMIT, tsHash, false)
	ensureVote(t, voteSub1, viewID, common.PRECOMMIT, tsHash, false)

	sendVoteMsg(t, k[0], viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState0, vaults.GetVaults(0, 2, 0, 1)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState0, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState0, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub0, viewID, common.PRECOMMIT, tsHash, true)

	ensureEmptySlots(t, k[0])
	ensureEmptySlots(t, k[1])
	checkForTS(t, k[0], clusterState0.Tesseract().Hash())
	checkForTS(t, k[1], clusterState0.Tesseract().Hash())

	k[0].ctxCancel()
	k[1].ctxCancel()

	wait(t, &wg)
}

// TestValidator_MissingProposal ensures that if a validator does not receive a proposal
// but does receive a prevoteQC, it will not send a precommit, even if it is a context node.
func TestValidator_MissingProposal(t *testing.T) {
	t.Parallel()

	wg := sync.WaitGroup{}
	ics, vaults := createICSCommittee(t, 2, 4, 5)

	// View-1 (Operator - N0) (Validator - N1)
	// B0 B0 B0 B0 - B0 B0 B0 B0 // N0 - B1 // B1 NIL B1 B1  - B1 B1 B1 B1 // NIL NIL NIL NIL - NIL NIL NIL NIL

	operatorVault := vaults[0][0]
	viewID := uint64(1)
	k := createKramaInstances(t, vaults.GetVaults(0, 2))

	k[0].currentView.SetID(viewID)
	k[1].currentView.SetID(viewID)
	k[0].registerForTest(k[1], packet{msgType: message.PROPOSAL})
	k[1].registerForTest(k[0])

	var (
		prepareSub0 = k[0].consensusMux.Subscribe(eventPrepare{})
		proposalSub = k[0].consensusMux.Subscribe(kbft.EventProposal{})
		voteSub0    = k[0].consensusMux.Subscribe(kbft.EventVote{})
		voteSub1    = k[1].consensusMux.Subscribe(kbft.EventVote{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k...)
	require.NoError(t, err)

	viewInfos, err := k[0].loadViewInfo(ids)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k[0], ids)
	require.NoError(t, err)

	clusterState0 := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k[0].selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k[0].pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot0, _ := k[0].slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot0 == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot0.UpdateClusterState(clusterState0)

	ics1 := createICSCommitteeFromVaults(vaults, []int{0, 1, 2})

	ixnsHash, err := ixns.Hash()
	require.NoError(t, err)

	k[1].storeICSCommitteeForTest(ixnsHash, ics1)
	storeIxns(k[1].pool, &ixns)

	go startHandler(k[0], &wg)
	go startHandler(k[1], &wg)

	go startICSHandler(k[0], &wg, k[0].ctx, clusterID)
	slot0.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub0, viewID, ixns.Hashes())
	sendPreparedMsg(t, k[0], clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 1, 0, 1)...)
	sendPreparedMsg(t, k[0], clusterID, viewID, viewInfos, vaults.GetVaults(1, 3)...)

	k[1].prepareTimeout <- struct{}{}

	ensureProposal(t, proposalSub, viewID, viewInfos, 6, clusterState0)

	tsHash := clusterState0.Tesseract().Hash()

	ensureVote(t, voteSub0, viewID, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(0, 2, 0, 1)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub0, viewID, common.PREVOTE, tsHash, true)

	ensureVote(t, voteSub0, viewID, common.PRECOMMIT, tsHash, false)

	ensureNoEventReceived(t, voteSub1)
	exitICSHandler(slot0)
	ensureEmptySlots(t, k[0])
	ensureEmptySlots(t, k[1])

	k[0].ctxCancel()
	k[1].ctxCancel()
	wait(t, &wg)
}

// TestValidator_MissingPrevoteQC ensures that if a validator does not receive a prevoteQC
// but does receive a precommitQC, it still commits the tesseract.
func TestValidator_MissingPrevoteQC(t *testing.T) {
	t.Parallel()

	wg := sync.WaitGroup{}
	ics, vaults := createICSCommittee(t, 2, 4, 5)

	// View-1 (Operator - N0) (Validator - N1)
	// B0 B0 B0 B0 - B0 B0 B0 B0 // N0 - B1 // B1 B1 B1 B1  - B1 B1 B1 B1 // B1 B1 B1 B1  - B1 B1 B1 B1
	// B1 B1 B1 B1  - B1 B1 B1 B1

	operatorVault := vaults[0][0]
	viewID := uint64(1)
	k := createKramaInstances(t, vaults.GetVaults(0, 2))

	k[0].currentView.SetID(viewID)
	k[1].currentView.SetID(viewID)

	k[0].registerForTest(k[1], packet{msgType: message.VOTEMSG, voteType: common.PREVOTE})
	k[1].registerForTest(k[0])

	var (
		prepareSub0  = k[0].consensusMux.Subscribe(eventPrepare{})
		proposalSub  = k[0].consensusMux.Subscribe(kbft.EventProposal{})
		voteSub0     = k[0].consensusMux.Subscribe(kbft.EventVote{})
		preparedSub1 = k[1].consensusMux.Subscribe(eventPrepared{})
		voteSub1     = k[1].consensusMux.Subscribe(kbft.EventVote{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k...)
	require.NoError(t, err)

	viewInfos, err := k[0].loadViewInfo(ids)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k[0], ids)
	require.NoError(t, err)

	clusterState0 := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k[0].selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k[0].pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot0, _ := k[0].slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot0 == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot0.UpdateClusterState(clusterState0)

	ics1 := createICSCommitteeFromVaults(vaults, []int{0, 1, 2})

	ixnsHash, err := ixns.Hash()
	require.NoError(t, err)

	k[1].storeICSCommitteeForTest(ixnsHash, ics1)
	storeIxns(k[1].pool, &ixns)

	go startHandler(k[0], &wg)
	go startHandler(k[1], &wg)

	go startICSHandler(k[0], &wg, k[0].ctx, clusterID)
	slot0.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub0, viewID, ixns.Hashes())

	k[1].prepareTimeout <- struct{}{}

	ensurePrepared(t, preparedSub1, viewID, viewInfos)
	sendPreparedMsg(t, k[0], clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 1, 0, 1)...)
	sendPreparedMsg(t, k[0], clusterID, viewID, viewInfos, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub, viewID, viewInfos, 6, clusterState0)

	tsHash := clusterState0.Tesseract().Hash()

	ensureVote(t, voteSub0, viewID, common.PREVOTE, tsHash, false)

	ensureVote(t, voteSub1, viewID, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(0, 1, 0, 1)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub0, viewID, common.PREVOTE, tsHash, true)

	ensureVote(t, voteSub0, viewID, common.PRECOMMIT, tsHash, false)

	sendVoteMsg(t, k[0], viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState0, vaults.GetVaults(0, 2, 0, 1)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState0, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState0, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub0, viewID, common.PRECOMMIT, tsHash, true)

	ensureEmptySlots(t, k[0])
	ensureEmptySlots(t, k[1])

	checkForTS(t, k[0], clusterState0.Tesseract().Hash())
	checkForTS(t, k[1], clusterState0.Tesseract().Hash())

	k[0].ctxCancel()
	k[1].ctxCancel()

	wait(t, &wg)
}

// TestFullRound_LockSeen_ByOperator ensures that if an operator has observed a lock on B1 in view-1,
// then as the operator in view-2, it proposes the locked block B1 and finalizes B1 in view-2.
func TestFullRound_LockSeen_ByOperator(t *testing.T) {
	t.Parallel()

	wg := sync.WaitGroup{}
	ics1, vaults := createICSCommittee(t, 2, 4, 5)

	// View-1 (operator-N0)
	// B0 B0 B0 B0 - B0 B0 B0 B0 // N0 - B1 // B1 B1 B1 B1  - B1 B1 B1 B1 // B1 NIL NIL NIL - NIL NIL NIL NIL
	operatorVault := vaults[0][0]
	view1 := uint64(1)
	k1 := newKramaInstance(t, 0, operatorVault)

	k1.currentView.SetID(view1)

	var (
		prepareSub  = k1.consensusMux.Subscribe(eventPrepare{})
		proposalSub = k1.consensusMux.Subscribe(kbft.EventProposal{})
		voteSub     = k1.consensusMux.Subscribe(kbft.EventVote{})
		cleanupSub  = k1.consensusMux.Subscribe(eventCleanup{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k1)
	require.NoError(t, err)

	viewInfos1, err := k1.loadViewInfo(ids)
	require.NoError(t, err)

	clusterID1, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k1, ids)
	require.NoError(t, err)

	clusterState1 := types.NewICS(
		ixns,
		clusterID1,
		operatorVault.KramaID(),
		time.Now(),
		k1.selfID,
		ics1,
		nil,
		ps,
		viewInfos1,
		types.NewView(view1, viewTime(0, view1, k1.pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot, _ := k1.slots.CreateSlotAndLockAccounts(clusterID1, types.OperatorSlot, ixns.Locks())
	if slot == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot.UpdateClusterState(clusterState1)

	go startHandler(k1, &wg)

	ctx, ctxCancel := context.WithCancel(context.Background())
	go startICSHandler(k1, &wg, ctx, clusterID1)

	slot.NewICSChan <- clusterID1

	ensurePrepare(t, prepareSub, view1, ixns.Hashes())
	sendPreparedMsg(t, k1, clusterID1, view1, viewInfos1,
		vaults.GetVaults(0, 2, 0)...)
	sendPreparedMsg(t, k1, clusterID1, view1, viewInfos1, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub, view1, viewInfos1, 6, clusterState1)

	tsHash := clusterState1.Tesseract().Hash()

	ensureVote(t, voteSub, view1, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k1, view1, clusterID1, common.PREVOTE,
		tsHash, clusterState1, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k1, view1, clusterID1, common.PREVOTE,
		tsHash, clusterState1, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k1, view1, clusterID1, common.PREVOTE,
		tsHash, clusterState1, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub, view1, common.PREVOTE, tsHash, true)
	ensureVote(t, voteSub, view1, common.PRECOMMIT, tsHash, false)

	ctxCancel()

	ensureClusterCleanup(t, cleanupSub, clusterID1)
	cleanupSub.Unsubscribe()

	// View-2 (operator-N0)
	// B1 B0 B0 B0 - B0 B0 B0 B0 // N0 - B1 // B1 B1 B1 B1 - B1 B1 B1 B1 // B1 B1 B1 B1 - B1 B1 B1 B1
	view2 := uint64(2)
	k1.currentView.SetID(view2)

	clusterID2, err := types.GenerateClusterID()
	require.NoError(t, err)

	viewInfos2, err := k1.loadViewInfo(ids)
	require.NoError(t, err)

	clusterState2 := types.NewICS(
		ixns,
		clusterID2,
		operatorVault.KramaID(),
		time.Now(),
		k1.selfID,
		createICSCommitteeFromVaults(vaults, []int{0, 1, 2}),
		nil,
		ps,
		viewInfos2,
		types.NewView(view2, viewTime(0, view2, k1.pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	ixnsHash, err := ixns.Hash()
	require.NoError(t, err)

	// useful in creating cluster state for validator or validating locks received
	k1.storeICSCommitteeForTest(ixnsHash, createICSCommitteeFromVaults(vaults, []int{0, 1, 2}))

	slot, _ = k1.slots.CreateSlotAndLockAccounts(clusterID2, types.OperatorSlot, ixns.Locks())
	if slot == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot.UpdateClusterState(clusterState2)

	go startICSHandler(k1, &wg, k1.ctx, clusterID2)

	slot.NewICSChan <- clusterID2

	ensurePrepare(t, prepareSub, view2, ixns.Hashes())

	sendPreparedMsg(t, k1, clusterID2, view2, viewInfos1,
		vaults.GetVaults(0, 2, 0)...)
	sendPreparedMsg(t, k1, clusterID2, view2, viewInfos1, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub, view2, viewInfos2, 1, clusterState1)

	ensureVote(t, voteSub, view2, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k1, view2, clusterID2, common.PREVOTE,
		tsHash, clusterState1, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k1, view2, clusterID2, common.PREVOTE,
		tsHash, clusterState1, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k1, view2, clusterID2, common.PREVOTE,
		tsHash, clusterState1, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub, view2, common.PREVOTE, tsHash, true)

	ensureVote(t, voteSub, view2, common.PRECOMMIT, tsHash, false)

	sendVoteMsg(t, k1, view2, clusterID2, common.PRECOMMIT,
		tsHash, clusterState2, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k1, view2, clusterID2, common.PRECOMMIT,
		tsHash, clusterState2, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k1, view2, clusterID2, common.PRECOMMIT,
		tsHash, clusterState2, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub, view2, common.PRECOMMIT, tsHash, true)

	ensureEmptySlots(t, k1)
	checkForTS(t, k1, tsHash)

	k1.ctxCancel()

	wait(t, &wg)
}

// TestFullRound_LockSeen_ByValidator_Syncing ensures that if an operator observes a lock on B1 in view-1,
// then in the next view, if another operator learns about this lock from the previous view’s operator,
// it fetches the locked block, proposes it, and finalizes it.
func TestFullRound_LockSeen_ByValidator_Syncing(t *testing.T) {
	t.Parallel()

	wg := sync.WaitGroup{}
	ics, vaults := createICSCommittee(t, 2, 4, 5)

	// View-1 (operator-N0)
	// B0 B0 B0 B0 - B0 B0 B0 B0 // N0 - B1 // B1 B1 B1 B1 - B1 B1 B1 B1 // B1 NIL NIL NIL - NIL NIL NIL NIL

	operatorVault := vaults[0][0]
	viewID := uint64(1)
	k := createKramaInstances(t, vaults.GetVaults(0, 2))

	k[0].currentView.SetID(viewID)
	k[1].currentView.SetID(viewID)

	var (
		prepareSub0  = k[0].consensusMux.Subscribe(eventPrepare{})
		proposalSub0 = k[0].consensusMux.Subscribe(kbft.EventProposal{})
		voteSub0     = k[0].consensusMux.Subscribe(kbft.EventVote{})
		cleanupSub0  = k[0].consensusMux.Subscribe(eventCleanup{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k...)
	require.NoError(t, err)

	viewInfos, err := k[0].loadViewInfo(ids)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k[0], ids)
	require.NoError(t, err)

	clusterState0 := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k[0].selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k[0].pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot0, _ := k[0].slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot0 == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot0.UpdateClusterState(clusterState0)

	storeIxns(k[0].pool, &ixns)

	go startHandler(k[0], &wg)

	ctx, ctxCancel := context.WithCancel(context.Background())
	go startICSHandler(k[0], &wg, ctx, clusterID)

	slot0.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub0, viewID, ixns.Hashes())
	sendPreparedMsg(t, k[0], clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 2, 0)...)
	sendPreparedMsg(t, k[0], clusterID, viewID, viewInfos, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub0, viewID, viewInfos, 6, clusterState0)

	tsHash := clusterState0.Tesseract().Hash()

	ensureVote(t, voteSub0, viewID, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub0, viewID, common.PREVOTE, tsHash, true)

	ensureVote(t, voteSub0, viewID, common.PRECOMMIT, tsHash, false)

	ctxCancel()

	ensureClusterCleanup(t, cleanupSub0, clusterID)
	cleanupSub0.Unsubscribe()

	// View-2 (operator-N2)
	// B1(L) B0 B0 B0 - B0 B0 B0 B0 // N1 - B1 // B1 B1 B1 B1 - B1 B1 B1 B1 //  B1 B1 B1 B1 - B1 B1 B1 B1

	view2 := uint64(2)
	operatorVault = vaults[0][1]

	k[0].currentView.SetID(view2)
	k[1].currentView.SetID(view2)

	k[0].registerForTest(k[1])
	k[1].registerForTest(k[0])

	var (
		prepareSub1  = k[1].consensusMux.Subscribe(eventPrepare{})
		preparedSub0 = k[0].consensusMux.Subscribe(eventPrepared{})
		proposalSub1 = k[1].consensusMux.Subscribe(kbft.EventProposal{})
		voteSub1     = k[1].consensusMux.Subscribe(kbft.EventVote{})
	)

	ics0 := createICSCommitteeFromVaults(vaults, []int{0, 1, 2})

	ixnsHash, err := ixns.Hash()
	require.NoError(t, err)

	// useful in creating cluster state for validator or validating locks received
	k[0].storeICSCommitteeForTest(ixnsHash, ics0)

	clusterID1, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps1, err := getParticipantsInfo(k[0], ids)
	require.NoError(t, err)

	clusterState1 := types.NewICS(
		ixns,
		clusterID1,
		operatorVault.KramaID(),
		time.Now(),
		k[1].selfID,
		createICSCommitteeFromVaults(vaults, []int{0, 1, 2}),
		nil,
		ps1,
		viewInfos,
		types.NewView(view2, viewTime(0, view2, k[0].pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	// useful while validating tesseract fetched through krama worker
	k[1].storeICSCommitteeForTest(ixnsHash, clusterState1.Committee())

	slot1, _ := k[1].slots.CreateSlotAndLockAccounts(clusterID1, types.OperatorSlot, ixns.Locks())
	if slot1 == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot1.UpdateClusterState(clusterState1)

	go startHandler(k[1], &wg)
	go startICSHandler(k[1], &wg, k[1].ctx, clusterID1)

	k[1].startWorkers()

	slot1.NewICSChan <- clusterID1

	ensurePrepare(t, prepareSub1, view2, ixns.Hashes())

	lockedViewInfos, err := k[0].loadViewInfo(ids)
	require.NoError(t, err)

	k[0].prepareTimeout <- struct{}{}

	ensurePrepared(t, preparedSub0, view2, lockedViewInfos)
	sendPreparedMsg(t, k[1], clusterID1, view2, viewInfos,
		vaults.GetVaults(0, 1, 0, 1)...)
	sendPreparedMsg(t, k[1], clusterID1, view2, viewInfos, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub1, view2, lockedViewInfos, 1, clusterState1)

	ensureVote(t, voteSub1, view2, common.PREVOTE, tsHash, false)
	ensureVote(t, voteSub0, view2, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k[1], view2, clusterID1, common.PREVOTE,
		tsHash, clusterState1, vaults.GetVaults(0, 1, 0, 1)...)
	sendVoteMsg(t, k[1], view2, clusterID1, common.PREVOTE,
		tsHash, clusterState1, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[1], view2, clusterID1, common.PREVOTE,
		tsHash, clusterState1, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub1, view2, common.PREVOTE, tsHash, true)

	ensureVote(t, voteSub0, view2, common.PRECOMMIT, tsHash, false)
	ensureVote(t, voteSub1, view2, common.PRECOMMIT, tsHash, false)

	sendVoteMsg(t, k[1], view2, clusterID1, common.PRECOMMIT,
		tsHash, clusterState1, vaults.GetVaults(0, 1, 0, 1)...)
	sendVoteMsg(t, k[1], view2, clusterID1, common.PRECOMMIT,
		tsHash, clusterState1, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[1], view2, clusterID1, common.PRECOMMIT,
		tsHash, clusterState1, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub1, view2, common.PRECOMMIT, tsHash, true)

	ensureEmptySlots(t, k[0])
	ensureEmptySlots(t, k[1])

	checkForTS(t, k[0], tsHash)
	checkForTS(t, k[1], tsHash)

	k[0].ctxCancel()
	k[1].ctxCancel()

	wait(t, &wg)
}

// TestFullRound_LockNotSeen_OutdatedLock verifies that when a lock on B1 (ixn-1) is created by N0 in view V1,
// and in view V2 block B2 (ixn-2) is committed on the same participants
// due to a partition from the previous lock holder,
// then in view V3, when N3 proposes a new block B3 for ixn-3, N0 should ignore the outdated B1 lock and sync to B2.
func TestFullRound_LockNotSeen_OutdatedLock(t *testing.T) {
	t.Parallel()

	wg := sync.WaitGroup{}
	ics, vaults := createICSCommittee(t, 2, 4, 5)

	// View-1 (operator-N0)
	// B0 B0 B0 B0 - B0 B0 B0 B0 // N0 - B1 // B1 B1 B1 B1  - B1 B1 B1 B1 // B1 NIL NIL NIL - NIL NIL NIL NIL

	operatorVault := vaults[0][0]
	viewID := uint64(1)
	k := createKramaInstances(t, vaults.GetVaults(0, 2))

	k[0].currentView.SetID(viewID)
	k[1].currentView.SetID(viewID)

	var (
		prepareSub0  = k[0].consensusMux.Subscribe(eventPrepare{})
		proposalSub0 = k[0].consensusMux.Subscribe(kbft.EventProposal{})
		voteSub0     = k[0].consensusMux.Subscribe(kbft.EventVote{})
		cleanupSub0  = k[0].consensusMux.Subscribe(eventCleanup{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k...)
	require.NoError(t, err)

	viewInfos, err := k[0].loadViewInfo(ids)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k[0], ids)
	require.NoError(t, err)

	clusterState0 := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k[0].selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k[0].pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot0, _ := k[0].slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot0 == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot0.UpdateClusterState(clusterState0)

	go startHandler(k[0], &wg)

	ctx, ctxCancel := context.WithCancel(context.Background())
	go startICSHandler(k[0], &wg, ctx, clusterID)
	slot0.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub0, viewID, ixns.Hashes())
	sendPreparedMsg(t, k[0], clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 2, 0)...)
	sendPreparedMsg(t, k[0], clusterID, viewID, viewInfos, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub0, viewID, viewInfos, 6, clusterState0)

	tsHash := clusterState0.Tesseract().Hash()

	ensureVote(t, voteSub0, viewID, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub0, viewID, common.PREVOTE, tsHash, true)

	ensureVote(t, voteSub0, viewID, common.PRECOMMIT, tsHash, false)

	ctxCancel()

	ensureClusterCleanup(t, cleanupSub0, clusterID)
	cleanupSub0.Unsubscribe()

	// View-2 (operator-N1)
	// E B0 B0 B0 - B0 B0 B0 B0 // N1 - B2 // E B2 B2 B2 - B2 B2 B2 B2 //  E B2 B2 B2 - B2 B2 B2 B2

	view2 := uint64(2)
	operatorVault = vaults[0][1]

	k[0].currentView.SetID(view2)
	k[1].currentView.SetID(view2)

	var (
		prepareSub1  = k[1].consensusMux.Subscribe(eventPrepare{})
		proposalSub1 = k[1].consensusMux.Subscribe(kbft.EventProposal{})
		voteSub1     = k[1].consensusMux.Subscribe(kbft.EventVote{})
		cleanupSub1  = k[1].consensusMux.Subscribe(eventCleanup{})
	)

	ics0 := createICSCommitteeFromVaults(vaults, []int{0, 1, 2})

	ixnsHash, err := ixns.Hash()
	require.NoError(t, err)

	// useful in creating cluster state for validator or validating locks received
	k[0].storeICSCommitteeForTest(ixnsHash, ics0)

	clusterID1, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps1, err := getParticipantsInfo(k[0], ids)
	require.NoError(t, err)

	clusterState1 := types.NewICS(
		ixns,
		clusterID1,
		operatorVault.KramaID(),
		time.Now(),
		k[1].selfID,
		createICSCommitteeFromVaults(vaults, []int{0, 1, 2}),
		nil,
		ps1,
		viewInfos,
		types.NewView(view2, viewTime(0, view2, k[0].pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	// useful while validating tesseract fetched through krama worker
	k[1].storeICSCommitteeForTest(ixnsHash, clusterState1.Committee())

	slot1, _ := k[1].slots.CreateSlotAndLockAccounts(clusterID1, types.OperatorSlot, ixns.Locks())
	if slot1 == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot1.UpdateClusterState(clusterState1)

	go startHandler(k[1], &wg)
	go startICSHandler(k[1], &wg, k[1].ctx, clusterID1)

	slot1.NewICSChan <- clusterID1

	ensurePrepare(t, prepareSub1, view2, ixns.Hashes())

	sendPreparedMsg(t, k[1], clusterID1, view2, viewInfos,
		vaults.GetVaults(0, 2, 0, 1)...)
	sendPreparedMsg(t, k[1], clusterID1, view2, viewInfos, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub1, view2, viewInfos, 6, clusterState1)

	tsHash2 := clusterState1.Tesseract().Hash()

	require.NotEqual(t, tsHash, tsHash2)

	ensureVote(t, voteSub1, view2, common.PREVOTE, tsHash2, false)

	sendVoteMsg(t, k[1], view2, clusterID1, common.PREVOTE,
		tsHash2, clusterState1, vaults.GetVaults(0, 2, 0, 1)...)
	sendVoteMsg(t, k[1], view2, clusterID1, common.PREVOTE,
		tsHash2, clusterState1, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[1], view2, clusterID1, common.PREVOTE,
		tsHash2, clusterState1, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub1, view2, common.PREVOTE, tsHash2, true)

	ensureVote(t, voteSub1, view2, common.PRECOMMIT, tsHash2, false)

	sendVoteMsg(t, k[1], view2, clusterID1, common.PRECOMMIT,
		tsHash2, clusterState1, vaults.GetVaults(0, 2, 0, 1)...)
	sendVoteMsg(t, k[1], view2, clusterID1, common.PRECOMMIT,
		tsHash2, clusterState1, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[1], view2, clusterID1, common.PRECOMMIT,
		tsHash2, clusterState1, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub1, view2, common.PRECOMMIT, tsHash2, true)

	ensureClusterCleanup(t, cleanupSub1, clusterID1)
	cleanupSub1.Unsubscribe()

	checkForTS(t, k[1], tsHash2)

	// View-3 (operator-N2)
	// B1(L) B2 B2 B2 - B2 B2 B2 B2 // N2 - B3 // NIL B3 B3 B3 - B3 B3 B3 B3 // NIL B3 B3 B3 - B3 B3 B3 B3

	view3 := uint64(3)
	operatorVault = vaults[0][1]

	k[0].currentView.SetID(view3)
	k[1].currentView.SetID(view3)

	k[0].registerForTest(k[1])
	k[1].registerForTest(k[0])

	var (
		preparedSub0    = k[0].consensusMux.Subscribe(eventPrepared{})
		syncRequestSub0 = k[0].mux.Subscribe(utils.SyncRequestEvent{})
	)

	ics3 := createICSCommitteeFromVaults(vaults, []int{0, 1, 2})

	ix2 := tests.CreateIXWithParticipants(t, ids, 1, nil)
	ixns2 := common.NewInteractionsWithLeaderCheck(false, ix2)

	ixnsHash2, err := ixns2.Hash()
	require.NoError(t, err)

	// useful in creating cluster state for validator or validating locks received
	k[0].storeICSCommitteeForTest(ixnsHash2, ics3)
	storeIxns(k[0].pool, &ixns2)

	clusterID3, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps3, err := getParticipantsInfo(k[1], ids)
	require.NoError(t, err)

	viewInfos2, err := k[1].loadViewInfo(ids)
	require.NoError(t, err)

	clusterState1 = types.NewICS(
		ixns2,
		clusterID3,
		operatorVault.KramaID(),
		time.Now(),
		k[1].selfID,
		createICSCommitteeFromVaults(vaults, []int{0, 1, 2}),
		nil,
		ps3,
		viewInfos2,
		types.NewView(view3, viewTime(0, view3, k[0].pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	// useful while validating tesseract fetched through krama worker
	k[1].storeICSCommitteeForTest(ixnsHash2, clusterState1.Committee())

	slot1, _ = k[1].slots.CreateSlotAndLockAccounts(clusterID3, types.OperatorSlot, ixns2.Locks())
	if slot1 == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot1.UpdateClusterState(clusterState1)

	go startICSHandler(k[1], &wg, k[1].ctx, clusterID3)

	k[0].startWorkers()
	k[1].startWorkers()

	slot1.NewICSChan <- clusterID3

	ensurePrepare(t, prepareSub1, view3, ixns2.Hashes())

	lockedViewInfos, err := k[0].loadViewInfo(ids)
	require.NoError(t, err)

	k[0].prepareTimeout <- struct{}{}

	ensurePrepared(t, preparedSub0, view3, lockedViewInfos)
	sendPreparedMsg(t, k[1], clusterID3, view3, viewInfos2,
		vaults.GetVaults(0, 1, 0, 1)...)
	sendPreparedMsg(t, k[1], clusterID3, view3, viewInfos2, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub1, view3, lockedViewInfos, 1, clusterState1)

	tsHash3 := clusterState1.Tesseract().Hash()

	ensureVote(t, voteSub1, view3, common.PREVOTE, tsHash3, false)
	ensureSyncEvent(t, syncRequestSub0, ids, 1)
	ensureSyncEvent(t, syncRequestSub0, ids, 1)

	k[0].ctxCancel()
	k[1].ctxCancel()

	ensureEmptySlots(t, k[0])
	ensureEmptySlots(t, k[1])

	wait(t, &wg)
}

// TestMultipleLocks_ViewPriority verifies that when a lock on B1 (ixn-1) is created by N0 in view V1,
// and in view V2 N1, as the operator, creates a lock on B2 for ixn-1,
// then in view V3 N1 prioritizes B2 over B1 since B2 was created in the latest view, and re-proposes and finalizes B2.
func TestMultipleLocks_ViewPriority(t *testing.T) {
	t.Parallel()

	wg := sync.WaitGroup{}
	ics, vaults := createICSCommittee(t, 2, 4, 5)

	// View-1 (operator-N0)
	// B0 B0 B0 B0 - B0 B0 B0 B0 // N0 - B1 // B1 B1 B1 B1  - B1 B1 B1 B1 // B1 NIL NIL NIL - NIL NIL NIL NIL

	operatorVault := vaults[0][0]
	viewID := uint64(1)
	k := createKramaInstances(t, vaults.GetVaults(0, 3))

	k[0].currentView.SetID(viewID)
	k[1].currentView.SetID(viewID)

	var (
		prepareSub0  = k[0].consensusMux.Subscribe(eventPrepare{})
		proposalSub0 = k[0].consensusMux.Subscribe(kbft.EventProposal{})
		voteSub0     = k[0].consensusMux.Subscribe(kbft.EventVote{})
		cleanupSub0  = k[0].consensusMux.Subscribe(eventCleanup{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k...)
	require.NoError(t, err)

	viewInfos, err := k[0].loadViewInfo(ids)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k[0], ids)
	require.NoError(t, err)

	clusterState0 := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k[0].selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k[0].pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot0, _ := k[0].slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot0 == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot0.UpdateClusterState(clusterState0)

	go startHandler(k[0], &wg)

	ctx, ctxCancel := context.WithCancel(context.Background())
	go startICSHandler(k[0], &wg, ctx, clusterID)
	slot0.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub0, viewID, ixns.Hashes())
	sendPreparedMsg(t, k[0], clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 2, 0)...)
	sendPreparedMsg(t, k[0], clusterID, viewID, viewInfos, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub0, viewID, viewInfos, 6, clusterState0)

	tsHash := clusterState0.Tesseract().Hash()

	ensureVote(t, voteSub0, viewID, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub0, viewID, common.PREVOTE, tsHash, true)

	ensureVote(t, voteSub0, viewID, common.PRECOMMIT, tsHash, false)

	ctxCancel()

	ensureClusterCleanup(t, cleanupSub0, clusterID)

	// View-2 (operator-N1)
	// E B0 B0 B0 - B0 B0 B0 B0 // N1 - B2 // E B2 B2 B2 - B2 B2 B2 B2 //  E B2 NIL NIL - NIL NIL NIL NIL

	view2 := uint64(2)
	operatorVault = vaults[0][1]

	k[0].currentView.SetID(view2)
	k[1].currentView.SetID(view2)

	var (
		prepareSub1  = k[1].consensusMux.Subscribe(eventPrepare{})
		proposalSub1 = k[1].consensusMux.Subscribe(kbft.EventProposal{})
		voteSub1     = k[1].consensusMux.Subscribe(kbft.EventVote{})
		cleanupSub1  = k[1].consensusMux.Subscribe(eventCleanup{})
	)

	ixnsHash, err := ixns.Hash()
	require.NoError(t, err)

	// useful in creating cluster state for validator or validating locks received
	k[0].storeICSCommitteeForTest(ixnsHash, createICSCommitteeFromVaults(vaults, []int{0, 1, 2}))

	clusterID1, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps1, err := getParticipantsInfo(k[0], ids)
	require.NoError(t, err)

	clusterState1 := types.NewICS(
		ixns,
		clusterID1,
		operatorVault.KramaID(),
		time.Now(),
		k[1].selfID,
		createICSCommitteeFromVaults(vaults, []int{0, 1, 2}),
		nil,
		ps1,
		viewInfos,
		types.NewView(view2, viewTime(0, view2, k[0].pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	// useful while validating tesseract fetched through krama worker
	k[1].storeICSCommitteeForTest(ixnsHash, clusterState1.Committee())

	slot1, _ := k[1].slots.CreateSlotAndLockAccounts(clusterID1, types.OperatorSlot, ixns.Locks())
	if slot1 == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot1.UpdateClusterState(clusterState1)

	go startHandler(k[1], &wg)

	ctx, ctxCancel = context.WithCancel(context.Background())

	go startICSHandler(k[1], &wg, ctx, clusterID1)

	slot1.NewICSChan <- clusterID1

	ensurePrepare(t, prepareSub1, view2, ixns.Hashes())

	sendPreparedMsg(t, k[1], clusterID1, view2, viewInfos,
		vaults.GetVaults(0, 2, 0, 1)...)
	sendPreparedMsg(t, k[1], clusterID1, view2, viewInfos, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub1, view2, viewInfos, 6, clusterState1)

	tsHash2 := clusterState1.Tesseract().Hash()

	require.NotEqual(t, tsHash, tsHash2)

	ensureVote(t, voteSub1, view2, common.PREVOTE, tsHash2, false)

	sendVoteMsg(t, k[1], view2, clusterID1, common.PREVOTE,
		tsHash2, clusterState1, vaults.GetVaults(0, 2, 0, 1)...)
	sendVoteMsg(t, k[1], view2, clusterID1, common.PREVOTE,
		tsHash2, clusterState1, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[1], view2, clusterID1, common.PREVOTE,
		tsHash2, clusterState1, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub1, view2, common.PREVOTE, tsHash2, true)

	ensureVote(t, voteSub1, view2, common.PRECOMMIT, tsHash2, false)

	ctxCancel()
	ensureClusterCleanup(t, cleanupSub1, clusterID1)

	// View-3 (operator-N2)
	// B1(L) B2(L) B0 B0 - B0 B0 B0 B0 // N2 - B2 // B2 B2 B2 B2 - B2 B2 B2 B2 //  B2 B2 B2 B2 - B2 B2 B2 B2

	view3 := uint64(3)
	operatorVault = vaults[0][2]

	k[0].currentView.SetID(view3)
	k[1].currentView.SetID(view3)
	k[2].currentView.SetID(view3)

	k[0].registerForTest(k[1])
	k[0].registerForTest(k[2])
	k[1].registerForTest(k[0])
	k[1].registerForTest(k[2])
	k[2].registerForTest(k[0])
	k[2].registerForTest(k[1])

	var (
		preparedSub0 = k[0].consensusMux.Subscribe(eventPrepared{})
		preparedSub1 = k[1].consensusMux.Subscribe(eventPrepared{})
		prepareSub2  = k[2].consensusMux.Subscribe(eventPrepare{})
		proposalSub2 = k[2].consensusMux.Subscribe(kbft.EventProposal{})
		voteSub2     = k[2].consensusMux.Subscribe(kbft.EventVote{})
		cleanupSub2  = k[2].consensusMux.Subscribe(eventCleanup{})
	)

	// useful in creating cluster state for validator or validating locks received
	k[0].storeICSCommitteeForTest(ixnsHash, createICSCommitteeFromVaults(vaults, []int{0, 1, 2}))
	k[1].storeICSCommitteeForTest(ixnsHash, createICSCommitteeFromVaults(vaults, []int{0, 1, 2}))

	clusterID3, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps3, err := getParticipantsInfo(k[2], ids)
	require.NoError(t, err)

	viewInfos2, err := k[2].loadViewInfo(ids)
	require.NoError(t, err)

	clusterState2 := types.NewICS(
		ixns,
		clusterID3,
		operatorVault.KramaID(),
		time.Now(),
		k[2].selfID,
		createICSCommitteeFromVaults(vaults, []int{0, 1, 2}),
		nil,
		ps3,
		viewInfos2,
		types.NewView(view3, viewTime(0, view3, k[0].pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	// useful while validating tesseract fetched through krama worker
	k[2].storeICSCommitteeForTest(ixnsHash, clusterState1.Committee())
	storeIxns(k[0].pool, &ixns)
	storeIxns(k[1].pool, &ixns)

	slot2, _ := k[2].slots.CreateSlotAndLockAccounts(clusterID3, types.OperatorSlot, ixns.Locks())
	if slot2 == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot2.UpdateClusterState(clusterState2)

	go startHandler(k[2], &wg)
	go startICSHandler(k[2], &wg, k[2].ctx, clusterID3)

	k[0].startWorkers()
	k[1].startWorkers()
	k[2].startWorkers()

	slot2.NewICSChan <- clusterID3

	ensurePrepare(t, prepareSub2, view3, ixns.Hashes())

	k[0].prepareTimeout <- struct{}{}
	k[1].prepareTimeout <- struct{}{}

	lockedViewInfos0, err := k[0].loadViewInfo(ids)
	require.NoError(t, err)

	lockedViewInfos1, err := k[1].loadViewInfo(ids)
	require.NoError(t, err)

	ensurePrepared(t, preparedSub0, view3, lockedViewInfos0)
	ensurePrepared(t, preparedSub1, view3, lockedViewInfos1)

	sendPreparedMsg(t, k[2], clusterID3, view3, viewInfos2, vaults.GetVaults(1, 3)...)
	sendPreparedMsg(t, k[2], clusterID3, view3, viewInfos2, vaults.GetVaults(2, 4)...)

	ensureProposal(t, proposalSub2, view3, lockedViewInfos1, 1, clusterState2)

	tsHash3 := clusterState2.Tesseract().Hash()

	require.Equal(t, tsHash2, tsHash3)
	require.NotEqual(t, tsHash, tsHash3)

	ensureVote(t, voteSub2, view3, common.PREVOTE, tsHash3, false)
	ensureVote(t, voteSub0, view3, common.PREVOTE, tsHash3, false)
	ensureVote(t, voteSub1, view3, common.PREVOTE, tsHash3, false)

	sendVoteMsg(t, k[2], view3, clusterID3, common.PREVOTE,
		tsHash3, clusterState2, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[2], view3, clusterID3, common.PREVOTE,
		tsHash3, clusterState2, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub2, view3, common.PREVOTE, tsHash3, true)

	ensureVote(t, voteSub0, view3, common.PRECOMMIT, tsHash3, false)
	ensureVote(t, voteSub1, view3, common.PRECOMMIT, tsHash3, false)
	ensureVote(t, voteSub2, view3, common.PRECOMMIT, tsHash3, false)

	sendVoteMsg(t, k[2], view3, clusterID3, common.PRECOMMIT,
		tsHash3, clusterState2, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[2], view3, clusterID3, common.PRECOMMIT,
		tsHash3, clusterState2, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub2, view3, common.PRECOMMIT, tsHash3, true)

	ensureClusterCleanup(t, cleanupSub2, clusterID3)
	ensureClusterCleanup(t, cleanupSub1, clusterID3)
	ensureClusterCleanup(t, cleanupSub0, clusterID3)

	ensureEmptySlots(t, k[0])
	ensureEmptySlots(t, k[1])
	ensureEmptySlots(t, k[2])

	checkForTS(t, k[2], tsHash3)
	checkForTS(t, k[0], tsHash3)
	checkForTS(t, k[1], tsHash3)

	k[0].ctxCancel()
	k[1].ctxCancel()
	k[2].ctxCancel()

	wait(t, &wg)
}

// TestRelock_HiddenLock verifies that when a lock on B1 (ixn-1) is created by N0 in view V1,
// and in view V2 N1, as the operator, creates a lock on B2 for ixn-1,
// then in view V3 N2, as the operator, is unaware of the B2 lock and re-proposes B1.
// N0 still votes for B1 despite having locked B2, and eventually all nodes commit B1.
func TestRelock_HiddenLock(t *testing.T) {
	t.Parallel()

	wg := sync.WaitGroup{}
	ics, vaults := createICSCommittee(t, 2, 4, 5)

	// View-1 (operator-N0)
	// B0 B0 B0 B0 - B0 B0 B0 B0 // N0 - B1 // B1 B1 B1 B1  - B1 B1 B1 B1 // B1 NIL NIL NIL - NIL NIL NIL NIL

	operatorVault := vaults[0][0]
	viewID := uint64(1)
	k := createKramaInstances(t, vaults.GetVaults(0, 3))

	k[0].currentView.SetID(viewID)
	k[1].currentView.SetID(viewID)

	var (
		prepareSub0  = k[0].consensusMux.Subscribe(eventPrepare{})
		proposalSub0 = k[0].consensusMux.Subscribe(kbft.EventProposal{})
		voteSub0     = k[0].consensusMux.Subscribe(kbft.EventVote{})
		cleanupSub0  = k[0].consensusMux.Subscribe(eventCleanup{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k...)
	require.NoError(t, err)

	viewInfos, err := k[0].loadViewInfo(ids)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k[0], ids)
	require.NoError(t, err)

	clusterState0 := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k[0].selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k[0].pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot0, _ := k[0].slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot0 == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot0.UpdateClusterState(clusterState0)

	go startHandler(k[0], &wg)

	ctx, ctxCancel := context.WithCancel(context.Background())
	go startICSHandler(k[0], &wg, ctx, clusterID)
	slot0.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub0, viewID, ixns.Hashes())
	sendPreparedMsg(t, k[0], clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 2, 0)...)
	sendPreparedMsg(t, k[0], clusterID, viewID, viewInfos, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub0, viewID, viewInfos, 6, clusterState0)

	tsHash := clusterState0.Tesseract().Hash()

	ensureVote(t, voteSub0, viewID, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub0, viewID, common.PREVOTE, tsHash, true)

	ensureVote(t, voteSub0, viewID, common.PRECOMMIT, tsHash, false)

	ctxCancel()

	ensureClusterCleanup(t, cleanupSub0, clusterID)

	// View-2 (operator-N1)
	// E B0 B0 B0 - B0 B0 B0 B0 // N1 - B2 // E B2 B2 B2 - B2 B2 B2 B2 //  E B2 NIL NIL - NIL NIL NIL NIL

	view2 := uint64(2)
	operatorVault = vaults[0][1]

	k[0].currentView.SetID(view2)
	k[1].currentView.SetID(view2)

	var (
		prepareSub1  = k[1].consensusMux.Subscribe(eventPrepare{})
		proposalSub1 = k[1].consensusMux.Subscribe(kbft.EventProposal{})
		voteSub1     = k[1].consensusMux.Subscribe(kbft.EventVote{})
		cleanupSub1  = k[1].consensusMux.Subscribe(eventCleanup{})
	)

	ics0 := createICSCommitteeFromVaults(vaults, []int{0, 1, 2})

	ixnsHash, err := ixns.Hash()
	require.NoError(t, err)

	// useful in creating cluster state for validator or validating locks received
	k[0].storeICSCommitteeForTest(ixnsHash, ics0)

	clusterID1, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps1, err := getParticipantsInfo(k[0], ids)
	require.NoError(t, err)

	clusterState1 := types.NewICS(
		ixns,
		clusterID1,
		operatorVault.KramaID(),
		time.Now(),
		k[1].selfID,
		createICSCommitteeFromVaults(vaults, []int{0, 1, 2}),
		nil,
		ps1,
		viewInfos,
		types.NewView(view2, viewTime(0, view2, k[0].pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	// useful while validating tesseract fetched through krama worker
	k[1].storeICSCommitteeForTest(ixnsHash, clusterState1.Committee())

	slot1, _ := k[1].slots.CreateSlotAndLockAccounts(clusterID1, types.OperatorSlot, ixns.Locks())
	if slot1 == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot1.UpdateClusterState(clusterState1)

	go startHandler(k[1], &wg)

	ctx, ctxCancel = context.WithCancel(context.Background())
	go startICSHandler(k[1], &wg, ctx, clusterID1)

	slot1.NewICSChan <- clusterID1

	ensurePrepare(t, prepareSub1, view2, ixns.Hashes())

	sendPreparedMsg(t, k[1], clusterID1, view2, viewInfos,
		vaults.GetVaults(0, 2, 0, 1)...)
	sendPreparedMsg(t, k[1], clusterID1, view2, viewInfos, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub1, view2, viewInfos, 6, clusterState1)

	tsHash2 := clusterState1.Tesseract().Hash()

	require.NotEqual(t, tsHash, tsHash2)

	ensureVote(t, voteSub1, view2, common.PREVOTE, tsHash2, false)

	sendVoteMsg(t, k[1], view2, clusterID1, common.PREVOTE,
		tsHash2, clusterState1, vaults.GetVaults(0, 2, 0, 1)...)
	sendVoteMsg(t, k[1], view2, clusterID1, common.PREVOTE,
		tsHash2, clusterState1, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[1], view2, clusterID1, common.PREVOTE,
		tsHash2, clusterState1, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub1, view2, common.PREVOTE, tsHash2, true)

	ensureVote(t, voteSub1, view2, common.PRECOMMIT, tsHash2, false)

	ctxCancel()
	ensureClusterCleanup(t, cleanupSub1, clusterID1)

	// View-3 (operator-N2)
	// B1 E B0 B0 - B0 B0 B0 B0 // N2 - B1 // B1 B1 B1 B1 - B1 B1 B1 B1 // B1 B1 B1 B1 - B1 B1 B1 B1

	view3 := uint64(3)

	k[0].currentView.SetID(view3)
	k[1].currentView.SetID(view3)
	k[2].currentView.SetID(view3)

	k[0].registerForTest(k[1])
	k[0].registerForTest(k[2])
	k[1].registerForTest(k[0])
	k[1].registerForTest(k[2])
	k[2].registerForTest(k[0])
	k[2].registerForTest(k[1], packet{
		msgType: message.PREPARE,
	})

	operatorVault = vaults[0][2]

	var (
		preparedSub0 = k[0].consensusMux.Subscribe(eventPrepared{})
		prepareSub2  = k[2].consensusMux.Subscribe(eventPrepare{})
		proposalSub2 = k[2].consensusMux.Subscribe(kbft.EventProposal{})
		voteSub2     = k[2].consensusMux.Subscribe(kbft.EventVote{})
		cleanupSub2  = k[2].consensusMux.Subscribe(eventCleanup{})
	)

	ics0 = createICSCommitteeFromVaults(vaults, []int{0, 1, 2})
	ics1 := createICSCommitteeFromVaults(vaults, []int{0, 1, 2})

	// useful in creating cluster state for validator or validating locks received
	k[0].storeICSCommitteeForTest(ixnsHash, ics0)
	k[1].storeICSCommitteeForTest(ixnsHash, ics1)

	clusterID3, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps3, err := getParticipantsInfo(k[2], ids)
	require.NoError(t, err)

	viewInfos2, err := k[2].loadViewInfo(ids)
	require.NoError(t, err)

	clusterState2 := types.NewICS(
		ixns,
		clusterID3,
		operatorVault.KramaID(),
		time.Now(),
		k[2].selfID,
		createICSCommitteeFromVaults(vaults, []int{0, 1, 2}),
		nil,
		ps3,
		viewInfos2,
		types.NewView(view3, viewTime(0, view3, k[0].pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	// useful while validating tesseract fetched through krama worker
	k[2].storeICSCommitteeForTest(ixnsHash, clusterState1.Committee())
	storeIxns(k[0].pool, &ixns)

	slot2, _ := k[2].slots.CreateSlotAndLockAccounts(clusterID3, types.OperatorSlot, ixns.Locks())
	if slot2 == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot2.UpdateClusterState(clusterState2)

	go startHandler(k[2], &wg)
	go startICSHandler(k[2], &wg, k[2].ctx, clusterID3)

	k[0].startWorkers()
	k[1].startWorkers()
	k[2].startWorkers()

	slot2.NewICSChan <- clusterID3

	ensurePrepare(t, prepareSub2, view3, ixns.Hashes())

	k[0].prepareTimeout <- struct{}{}

	lockedViewInfos0, err := k[0].loadViewInfo(ids)
	require.NoError(t, err)

	ensurePrepared(t, preparedSub0, view3, lockedViewInfos0)

	sendPreparedMsg(t, k[2], clusterID3, view3, viewInfos2,
		vaults.GetVaults(0, 1, 0, 1, 2)...)
	sendPreparedMsg(t, k[2], clusterID3, view3, viewInfos2, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub2, view3, lockedViewInfos0, 1, clusterState2)

	tsHash3 := clusterState2.Tesseract().Hash()

	require.Equal(t, tsHash, tsHash3)
	require.NotEqual(t, tsHash2, tsHash3)

	ensureVote(t, voteSub2, view3, common.PREVOTE, tsHash3, false)
	ensureVote(t, voteSub0, view3, common.PREVOTE, tsHash3, false)
	ensureVote(t, voteSub1, view3, common.PREVOTE, tsHash3, false)

	sendVoteMsg(t, k[2], view3, clusterID3, common.PREVOTE,
		tsHash3, clusterState2, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[2], view3, clusterID3, common.PREVOTE,
		tsHash3, clusterState2, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub2, view3, common.PREVOTE, tsHash3, true)

	ensureVote(t, voteSub0, view3, common.PRECOMMIT, tsHash3, false)
	ensureVote(t, voteSub1, view3, common.PRECOMMIT, tsHash3, false)
	ensureVote(t, voteSub2, view3, common.PRECOMMIT, tsHash3, false)

	sendVoteMsg(t, k[2], view3, clusterID3, common.PRECOMMIT,
		tsHash3, clusterState2, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[2], view3, clusterID3, common.PRECOMMIT,
		tsHash3, clusterState2, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub2, view3, common.PRECOMMIT, tsHash3, true)

	ensureClusterCleanup(t, cleanupSub2, clusterID3)
	ensureClusterCleanup(t, cleanupSub1, clusterID3)
	ensureClusterCleanup(t, cleanupSub0, clusterID3)

	ensureEmptySlots(t, k[0])
	ensureEmptySlots(t, k[1])
	ensureEmptySlots(t, k[2])

	checkForTS(t, k[2], tsHash3)
	checkForTS(t, k[0], tsHash3)
	checkForTS(t, k[1], tsHash3)

	k[0].ctxCancel()
	k[1].ctxCancel()
	k[2].ctxCancel()

	wait(t, &wg)
}

// TestViewChangeInSyncLag verifies that in view V1, with N0 as operator, all participants commit B1 except N1.
// In view V2, with N0 still as operator, N1 learns about B1 from the proposal and sends a sync request.
func TestViewChangeInSyncLag(t *testing.T) {
	t.Parallel()

	wg := sync.WaitGroup{}
	ics, vaults := createICSCommittee(t, 2, 4, 5)

	// View-1 (operator-N0)
	// B0 E B0 B0  - B0 B0 B0 B0 // N1 - B1 // B1 E B1 B1 - B1 B1 B1 B1 //  B1 E B1 B1 - B1 B1 B1 B1

	operatorVault := vaults[0][0]

	viewID := uint64(1)

	k := createKramaInstances(t, vaults.GetVaults(0, 2))

	k[0].currentView.SetID(viewID)
	k[1].currentView.SetID(viewID)

	var (
		prepareSub0  = k[0].consensusMux.Subscribe(eventPrepare{})
		proposalSub0 = k[0].consensusMux.Subscribe(kbft.EventProposal{})
		voteSub0     = k[0].consensusMux.Subscribe(kbft.EventVote{})
		cleanupSub0  = k[0].consensusMux.Subscribe(eventCleanup{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k...)
	require.NoError(t, err)

	viewInfos, err := k[0].loadViewInfo(ids)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k[0], ids)
	require.NoError(t, err)

	clusterState0 := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k[0].selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k[0].pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot0, _ := k[0].slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot0 == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot0.UpdateClusterState(clusterState0)

	go startHandler(k[0], &wg)
	go startICSHandler(k[0], &wg, k[0].ctx, clusterID)
	slot0.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub0, viewID, ixns.Hashes())
	sendPreparedMsg(t, k[0], clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 2, 0, 1)...)
	sendPreparedMsg(t, k[0], clusterID, viewID, viewInfos, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub0, viewID, viewInfos, 6, clusterState0)

	tsHash := clusterState0.Tesseract().Hash()

	ensureVote(t, voteSub0, viewID, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(0, 2, 0, 1)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub0, viewID, common.PREVOTE, tsHash, true)

	ensureVote(t, voteSub0, viewID, common.PRECOMMIT, tsHash, false)

	sendVoteMsg(t, k[0], viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState0, vaults.GetVaults(0, 2, 0, 1)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState0, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState0, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub0, viewID, common.PRECOMMIT, tsHash, true)

	ensureClusterCleanup(t, cleanupSub0, clusterID)
	cleanupSub0.Unsubscribe()
	checkForTS(t, k[0], clusterState0.Tesseract().Hash())

	// View-2 (operator-N0)
	// B1 B0 B1 B1 - B1 B1 B1 B1 // N0 - B2 // B2 B2 B2 B2 - B2 B2 B2 B2

	view2 := uint64(2)
	operatorVault = vaults[0][0]

	k[0].currentView.SetID(view2)
	k[1].currentView.SetID(view2)

	k[0].registerForTest(k[1])
	k[1].registerForTest(k[0])

	preparedSub1 := k[1].consensusMux.Subscribe(eventPrepared{})
	syncRequestSub1 := k[1].mux.Subscribe(utils.SyncRequestEvent{})

	ics1 := createICSCommitteeFromVaults(vaults, []int{0, 1, 2})

	ix2 := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns2 := common.NewInteractionsWithLeaderCheck(false, ix2)

	ixnsHash2, err := ixns2.Hash()
	require.NoError(t, err)

	// useful in creating cluster state for validator or validating locks received
	k[1].storeICSCommitteeForTest(ixnsHash2, ics1)
	storeIxns(k[1].pool, &ixns)

	clusterID1, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps1, err := getParticipantsInfo(k[0], ids)
	require.NoError(t, err)

	viewInfos2, err := k[0].loadViewInfo(ids)
	require.NoError(t, err)

	clusterState1 := types.NewICS(
		ixns2,
		clusterID1,
		operatorVault.KramaID(),
		time.Now(),
		k[0].selfID,
		createICSCommitteeFromVaults(vaults, []int{0, 1, 2}),
		nil,
		ps1,
		viewInfos2,
		types.NewView(view2, viewTime(0, view2, k[0].pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot1, _ := k[0].slots.CreateSlotAndLockAccounts(clusterID1, types.OperatorSlot, ixns2.Locks())
	if slot1 == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot1.UpdateClusterState(clusterState1)

	go startHandler(k[1], &wg)
	go startICSHandler(k[0], &wg, k[0].ctx, clusterID1)

	slot1.NewICSChan <- clusterID1

	ensurePrepare(t, prepareSub0, view2, ixns2.Hashes())
	k[1].prepareTimeout <- struct{}{}

	ensurePrepared(t, preparedSub1, view2, viewInfos)

	sendPreparedMsg(t, k[0], clusterID1, view2, viewInfos2,
		vaults.GetVaults(0, 1, 0, 1)...)
	sendPreparedMsg(t, k[0], clusterID1, view2, viewInfos2, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub0, view2, viewInfos2, 5, clusterState1)
	ensureSyncEvent(t, syncRequestSub1, ids, 1)
	ensureSyncEvent(t, syncRequestSub1, ids, 1)

	slot0 = k[0].slots.GetSlot(clusterID1)

	exitICSHandler(slot0)

	k[0].ctxCancel()
	k[1].ctxCancel()

	ensureEmptySlots(t, k[0])
	ensureEmptySlots(t, k[1])

	wait(t, &wg)
}

// TestLock_PartialLocks_AbortOnly verifies that in view V1, with N0 as operator, N0 locks B1(L) on P1 and P2.
// In view V2, with N8 as operator, N8 locks B11(L) on P3 and P4.
// In view V3, with N1 as operator, N1 learns about both B1 and B11 while processing an ixn with
// participants P1, P3, and P5, and must abort due to conflicting locks.
func TestLock_PartialLocks_AbortOnly(t *testing.T) {
	t.Parallel()

	wg := sync.WaitGroup{}
	_, vaults := createICSCommittee(t, 5, 4, 5)

	// P1 : N0 - N3
	// P2 : N4 - N7
	// P3 : N8 - N11
	// P4 : N12 - N15
	// P5 : N16 - N19
	// R : N20 - N24

	operators := make([]*crypto.KramaVault, 0, 3)
	operators = append(operators, vaults.GetVaults(0, 1)...)
	operators = append(operators, vaults.GetVaults(2, 1)...)
	operators = append(operators, vaults.GetVaults(0, 1, 0)...)
	operators = append(operators, vaults.GetVaults(4, 1)...)

	k := createKramaInstances(t, operators)

	// View-1 (operator-N0) (Ix1) (P1,P2)
	// B0 B0 B0 B0 - B0 B0 B0 B0 // N0 - B1 // B1 B1 B1 B1  - B1 B1 B1 B1 // B1 NIL NIL NIL - NIL NIL NIL NIL

	operatorVault := vaults.GetVaults(0, 1)[0]

	viewID := uint64(1)

	k[0].currentView.SetID(viewID)
	k[1].currentView.SetID(viewID)

	var (
		prepareSub0  = k[0].consensusMux.Subscribe(eventPrepare{})
		proposalSub0 = k[0].consensusMux.Subscribe(kbft.EventProposal{})
		voteSub0     = k[0].consensusMux.Subscribe(kbft.EventVote{})
		cleanupSub0  = k[0].consensusMux.Subscribe(eventCleanup{})
	)

	ids := tests.GetIdentifiers(t, 5)
	ix := tests.CreateIXWithParticipants(t, ids[:2], 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)
	participants := ids[:2]

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k...)
	require.NoError(t, err)

	viewInfos, err := k[0].loadViewInfo(participants)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k[0], participants)
	require.NoError(t, err)

	clusterState0 := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k[0].selfID,
		createICSCommitteeFromVaults(vaults, []int{0, 1, 5}),
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k[0].pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot0, _ := k[0].slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot0 == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot0.UpdateClusterState(clusterState0)

	go startHandler(k[0], &wg)

	ctx, ctxCancel := context.WithCancel(context.Background())

	go startICSHandler(k[0], &wg, ctx, clusterID)
	slot0.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub0, viewID, ixns.Hashes())
	sendPreparedMsg(t, k[0], clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 2, 0)...)
	sendPreparedMsg(t, k[0], clusterID, viewID, viewInfos, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub0, viewID, viewInfos, 6, clusterState0)

	tsHash := clusterState0.Tesseract().Hash()

	ensureVote(t, voteSub0, viewID, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(0, 2, 0)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(5, 4)...)

	ensureVote(t, voteSub0, viewID, common.PREVOTE, tsHash, true)

	ensureVote(t, voteSub0, viewID, common.PRECOMMIT, tsHash, false)

	ctxCancel()

	ensureClusterCleanup(t, cleanupSub0, clusterID)
	cleanupSub0.Unsubscribe()

	// View-2 (operator-N8) (IX2) (P3,P4)
	// B0 B0 B0 B0 - B0 B0 B0 B0  // N8 - B11 // B11 B11 B11 B11 - B11 B11 B11 B11 // B11 NIL NIL NIL - NIL NIL NIL NIL

	view2 := uint64(2)
	operatorVault = vaults.GetVaults(2, 1)[0]

	k[1].currentView.SetID(view2)

	var (
		prepareSub1  = k[1].consensusMux.Subscribe(eventPrepare{})
		proposalSub1 = k[1].consensusMux.Subscribe(kbft.EventProposal{})
		voteSub1     = k[1].consensusMux.Subscribe(kbft.EventVote{})
		cleanupSub1  = k[1].consensusMux.Subscribe(eventCleanup{})
	)

	ix = tests.CreateIXWithParticipants(t, ids[2:4], 0, nil)
	ixns = common.NewInteractionsWithLeaderCheck(false, ix)
	participants = ids[2:4]

	clusterID1, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps1, err := getParticipantsInfo(k[1], participants)
	require.NoError(t, err)

	viewInfos2, err := k[1].loadViewInfo(participants)
	require.NoError(t, err)

	clusterState1 := types.NewICS(
		ixns,
		clusterID1,
		operatorVault.KramaID(),
		time.Now(),
		k[1].selfID,
		createICSCommitteeFromVaults(vaults, []int{2, 3, 5}),
		nil,
		ps1,
		viewInfos2,
		types.NewView(view2, viewTime(0, view2, k[0].pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot1, _ := k[1].slots.CreateSlotAndLockAccounts(clusterID1, types.OperatorSlot, ixns.Locks())
	if slot1 == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot1.UpdateClusterState(clusterState1)

	go startHandler(k[1], &wg)

	ctx, ctxCancel = context.WithCancel(context.Background())

	go startICSHandler(k[1], &wg, ctx, clusterID1)
	slot1.NewICSChan <- clusterID1

	ensurePrepare(t, prepareSub1, view2, ixns.Hashes())

	sendPreparedMsg(t, k[1], clusterID1, view2, viewInfos2,
		vaults.GetVaults(2, 2, 0)...)
	sendPreparedMsg(t, k[1], clusterID1, view2, viewInfos2, vaults.GetVaults(3, 3)...)

	ensureProposal(t, proposalSub1, view2, viewInfos2, 6, clusterState1)

	tsHash2 := clusterState1.Tesseract().Hash()

	require.NotEqual(t, tsHash, tsHash2)

	ensureVote(t, voteSub1, view2, common.PREVOTE, tsHash2, false)

	sendVoteMsg(t, k[1], view2, clusterID1, common.PREVOTE,
		tsHash2, clusterState1, vaults.GetVaults(2, 2, 0)...)
	sendVoteMsg(t, k[1], view2, clusterID1, common.PREVOTE,
		tsHash2, clusterState1, vaults.GetVaults(3, 3)...)
	sendVoteMsg(t, k[1], view2, clusterID1, common.PREVOTE,
		tsHash2, clusterState1, vaults.GetVaults(5, 4)...)

	ensureVote(t, voteSub1, view2, common.PREVOTE, tsHash2, true)

	ensureVote(t, voteSub1, view2, common.PRECOMMIT, tsHash2, false)

	ctxCancel()
	ensureClusterCleanup(t, cleanupSub1, clusterID1)
	cleanupSub1.Unsubscribe()

	// View-3 (operator-N1) (P1,P3,P5)
	// B1 B0 B0 B0 - B11 B0 B0 B0 - B0 B0 B0 B0 // N2 - Abort

	view3 := uint64(3)

	k[0].currentView.SetID(view3)
	k[1].currentView.SetID(view3)
	k[2].currentView.SetID(view3)

	k[0].registerForTest(k[1])
	k[0].registerForTest(k[2])
	k[1].registerForTest(k[0])
	k[1].registerForTest(k[2])
	k[2].registerForTest(k[0])
	k[2].registerForTest(k[1])

	operatorVault = vaults.GetVaults(0, 1, 0)[0]

	var (
		preparedSub0 = k[0].consensusMux.Subscribe(eventPrepared{})
		preparedSub1 = k[1].consensusMux.Subscribe(eventPrepared{})
		prepareSub2  = k[2].consensusMux.Subscribe(eventPrepare{})
		voteSub2     = k[2].consensusMux.Subscribe(kbft.EventVote{})
	)

	participants = []identifiers.Identifier{ids[0], ids[2], ids[4]}
	ix = tests.CreateIXWithParticipants(t, participants, 0, nil)
	ixns = common.NewInteractionsWithLeaderCheck(false, ix)

	ixnsHash, err := ixns.Hash()
	require.NoError(t, err)

	ics0 := createICSCommitteeFromVaults(vaults, []int{0, 2, 4, 5})
	ics1 := createICSCommitteeFromVaults(vaults, []int{0, 2, 4, 5})

	k[0].storeICSCommitteeForTest(ixnsHash, ics0)
	k[1].storeICSCommitteeForTest(ixnsHash, ics1)

	// useful in creating cluster state for validator or validating locks received
	for i := 0; i < 3; i++ {
		k[i].storeICSCommitteeForTest(clusterState0.IxnsHash(), createICSCommitteeFromVaults(vaults, []int{0, 1, 5}))
		k[i].storeICSCommitteeForTest(clusterState1.IxnsHash(), createICSCommitteeFromVaults(vaults, []int{2, 3, 5}))
	}

	clusterID3, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps3, err := getParticipantsInfo(k[2], participants)
	require.NoError(t, err)

	viewInfos3, err := k[2].loadViewInfo(participants)
	require.NoError(t, err)

	genesisViewInfos, err := k[3].loadViewInfo(participants)
	require.NoError(t, err)

	clusterState2 := types.NewICS(
		ixns,
		clusterID3,
		operatorVault.KramaID(),
		time.Now(),
		k[2].selfID,
		createICSCommitteeFromVaults(vaults, []int{0, 2, 4, 5}),
		nil,
		ps3,
		viewInfos3,
		types.NewView(view3, viewTime(0, view3, k[0].pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot2, _ := k[2].slots.CreateSlotAndLockAccounts(clusterID3, types.OperatorSlot, ixns.Locks())
	if slot2 == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot2.UpdateClusterState(clusterState2)

	storeIxns(k[0].pool, &ixns)
	storeIxns(k[1].pool, &ixns)

	go startHandler(k[2], &wg)
	go startICSHandler(k[2], &wg, k[2].ctx, clusterID3)

	k[0].startWorkers()
	k[1].startWorkers()
	k[2].startWorkers()

	slot2.NewICSChan <- clusterID3

	ensurePrepare(t, prepareSub2, view3, ixns.Hashes())

	k[0].prepareTimeout <- struct{}{}
	k[1].prepareTimeout <- struct{}{}

	lockedViewInfos0, err := k[0].loadViewInfo(participants)
	require.NoError(t, err)

	lockedViewInfos1, err := k[1].loadViewInfo(participants)
	require.NoError(t, err)

	ensurePrepared(t, preparedSub0, view3, lockedViewInfos0)
	ensurePrepared(t, preparedSub1, view3, lockedViewInfos1)

	sendPreparedMsg(t, k[2], clusterID3, view3, genesisViewInfos,
		vaults.GetVaults(0, 1, 0, 1)...)
	sendPreparedMsg(t, k[2], clusterID3, view3, genesisViewInfos, vaults.GetVaults(2, 2, 0)...)
	sendPreparedMsg(t, k[2], clusterID3, view3, genesisViewInfos, vaults.GetVaults(4, 3)...)

	ensureNoEventReceived(t, voteSub2)
	ensureNoEventReceived(t, voteSub0)
	ensureNoEventReceived(t, voteSub1)

	exitICSHandler(slot2)

	ensureEmptySlots(t, k[0])
	ensureEmptySlots(t, k[1])
	ensureEmptySlots(t, k[2])

	k[0].ctxCancel()
	k[1].ctxCancel()
	k[2].ctxCancel()

	wait(t, &wg)
}

// TestFullRound_CommittedTS_VoteOnLock verifies that in view V1, with N1 as operator, only N1 commits B1.
// In view V2, when other nodes re-propose B1(L),
// N1 should still prevote for B1 even though it has already committed it.
func TestFullRound_CommittedTS_VoteOnLock(t *testing.T) {
	t.Parallel()

	wg := sync.WaitGroup{}
	ics, vaults := createICSCommittee(t, 2, 4, 5)

	// View-1 (operator-N1)
	// B0 B0 B0 B0 - B0 B0 B0 B0 // N1 - B1 // B1 B1 B1 B1 - B1 B1 B1 B1 // B1 B1 B1 B1 - B1 B1 B1 B1
	// B1 NIL NIL NIL - NIL NIL NIL NIL

	operatorVault := vaults[0][0]
	viewID := uint64(1)
	k := createKramaInstances(t, vaults.GetVaults(0, 2))

	k[0].currentView.SetID(viewID)
	k[1].currentView.SetID(viewID)

	k[0].registerForTest(k[1], packet{msgType: message.VOTEMSG, voteType: common.PRECOMMIT})
	k[1].registerForTest(k[0])

	var (
		prepareSub0  = k[0].consensusMux.Subscribe(eventPrepare{})
		proposalSub0 = k[0].consensusMux.Subscribe(kbft.EventProposal{})
		voteSub0     = k[0].consensusMux.Subscribe(kbft.EventVote{})
	)

	ids := tests.GetIdentifiers(t, 2)
	ix := tests.CreateIXWithParticipants(t, ids, 0, nil)
	ixns := common.NewInteractionsWithLeaderCheck(false, ix)

	ts := createGenesisTS(t, ids)

	err := storeGenesisData(ts, k...)
	require.NoError(t, err)

	viewInfos, err := k[0].loadViewInfo(ids)
	require.NoError(t, err)

	clusterID, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps, err := getParticipantsInfo(k[0], ids)
	require.NoError(t, err)

	clusterState0 := types.NewICS(
		ixns,
		clusterID,
		operatorVault.KramaID(),
		time.Now(),
		k[0].selfID,
		ics,
		nil,
		ps,
		viewInfos,
		types.NewView(viewID, viewTime(0, viewID, k[0].pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	slot0, _ := k[0].slots.CreateSlotAndLockAccounts(clusterID, types.OperatorSlot, ixns.Locks())
	if slot0 == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot0.UpdateClusterState(clusterState0)

	ics1 := createICSCommitteeFromVaults(vaults, []int{0, 1, 2})

	ixnsHash, err := ixns.Hash()
	require.NoError(t, err)

	k[1].storeICSCommitteeForTest(ixnsHash, ics1)
	storeIxns(k[1].pool, &ixns)

	go startHandler(k[0], &wg)
	go startHandler(k[1], &wg)

	go startICSHandler(k[0], &wg, k[0].ctx, clusterID)

	slot0.NewICSChan <- clusterID

	ensurePrepare(t, prepareSub0, viewID, ixns.Hashes())
	sendPreparedMsg(t, k[0], clusterID, viewID, viewInfos,
		vaults.GetVaults(0, 1, 0, 1)...)
	sendPreparedMsg(t, k[0], clusterID, viewID, viewInfos, vaults.GetVaults(1, 3)...)

	k[1].prepareTimeout <- struct{}{}

	ensureProposal(t, proposalSub0, viewID, viewInfos, 6, clusterState0)

	tsHash := clusterState0.Tesseract().Hash()

	ensureVote(t, voteSub0, viewID, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(0, 1, 0, 1)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PREVOTE,
		tsHash, clusterState0, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub0, viewID, common.PREVOTE, tsHash, true)

	ensureVote(t, voteSub0, viewID, common.PRECOMMIT, tsHash, false)

	sendVoteMsg(t, k[0], viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState0, vaults.GetVaults(0, 1, 0, 1)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState0, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[0], viewID, clusterID, common.PRECOMMIT,
		tsHash, clusterState0, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub0, viewID, common.PRECOMMIT, tsHash, true)

	k[1].ctxCancel()

	ensureEmptySlots(t, k[0])
	ensureEmptySlots(t, k[1])
	checkForTS(t, k[0], clusterState0.Tesseract().Hash())

	// View-2 (operator-N2)
	// B1(L) B0 B0 B0 - B0 B0 B0 B0 // N2 - B1 // B1 B1 B1 B1 - B1 B1 B1 B1 //  B1 B1 B1 B1 - B1 B1 B1 B1
	// B1 B1 B1 B1 - B1 B1 B1 B1

	view2 := uint64(2)
	operatorVault = vaults[0][1]

	k[1].resetContextForTest()

	k[1].currentView.SetID(view2)
	k[1].currentView.SetID(view2)

	k[0].registerForTest(k[1])
	k[1].registerForTest(k[0], packet{
		msgType: message.PREPARE,
	})

	var (
		prepareSub1  = k[1].consensusMux.Subscribe(eventPrepare{})
		proposalSub1 = k[1].consensusMux.Subscribe(kbft.EventProposal{})
		voteSub1     = k[1].consensusMux.Subscribe(kbft.EventVote{})
	)

	ics0 := createICSCommitteeFromVaults(vaults, []int{0, 1, 2})

	// useful in creating cluster state for validator or validating locks received
	k[0].storeICSCommitteeForTest(ixnsHash, ics0)

	clusterID1, err := types.GenerateClusterID()
	require.NoError(t, err)

	ps1, err := getParticipantsInfo(k[1], ids)
	require.NoError(t, err)

	viewInfos1, err := k[1].loadViewInfo(ids)
	require.NoError(t, err)

	clusterState1 := types.NewICS(
		ixns,
		clusterID1,
		operatorVault.KramaID(),
		time.Now(),
		k[1].selfID,
		createICSCommitteeFromVaults(vaults, []int{0, 1, 2}),
		nil,
		ps1,
		viewInfos1,
		types.NewView(view2, viewTime(0, view2, k[0].pool.ViewTimeOut()), time.Now().Add(2*time.Minute)),
		false,
	)

	// useful while validating tesseract fetched through krama worker
	k[1].storeICSCommitteeForTest(ixnsHash, clusterState1.Committee())

	slot1, _ := k[1].slots.CreateSlotAndLockAccounts(clusterID1, types.OperatorSlot, ixns.Locks())
	if slot1 == nil {
		require.FailNow(t, "slots are full")

		return
	}

	slot1.UpdateClusterState(clusterState1)

	go startHandler(k[1], &wg)
	go startICSHandler(k[1], &wg, k[1].ctx, clusterID1)

	k[1].startWorkers()

	slot1.NewICSChan <- clusterID1

	ensurePrepare(t, prepareSub1, view2, ixns.Hashes())

	sendPreparedMsg(t, k[1], clusterID1, view2, viewInfos1,
		vaults.GetVaults(0, 2, 0, 1)...)
	sendPreparedMsg(t, k[1], clusterID1, view2, viewInfos1, vaults.GetVaults(1, 3)...)

	ensureProposal(t, proposalSub1, view2, viewInfos1, 6, clusterState1)

	ensureVote(t, voteSub1, view2, common.PREVOTE, tsHash, false)
	ensureVote(t, voteSub0, view2, common.PREVOTE, tsHash, false)

	sendVoteMsg(t, k[1], view2, clusterID1, common.PREVOTE,
		tsHash, clusterState1, vaults.GetVaults(0, 1, 0, 1)...)
	sendVoteMsg(t, k[1], view2, clusterID1, common.PREVOTE,
		tsHash, clusterState1, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[1], view2, clusterID1, common.PREVOTE,
		tsHash, clusterState1, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub1, view2, common.PREVOTE, tsHash, true)

	ensureVote(t, voteSub0, view2, common.PRECOMMIT, tsHash, false)
	ensureVote(t, voteSub1, view2, common.PRECOMMIT, tsHash, false)

	sendVoteMsg(t, k[1], view2, clusterID1, common.PRECOMMIT,
		tsHash, clusterState1, vaults.GetVaults(0, 1, 0, 1)...)
	sendVoteMsg(t, k[1], view2, clusterID1, common.PRECOMMIT,
		tsHash, clusterState1, vaults.GetVaults(1, 3)...)
	sendVoteMsg(t, k[1], view2, clusterID1, common.PRECOMMIT,
		tsHash, clusterState1, vaults.GetVaults(2, 4)...)

	ensureVote(t, voteSub1, view2, common.PRECOMMIT, tsHash, true)

	ensureEmptySlots(t, k[0])
	ensureEmptySlots(t, k[1])

	checkForTS(t, k[0], tsHash)
	checkForTS(t, k[1], tsHash)

	k[0].ctxCancel()
	k[1].ctxCancel()

	wait(t, &wg)
}

func TestProcessPrepareMsgs(t *testing.T) {
	ids := tests.GetIdentifiers(t, 8)
	nodes := tests.RandomKramaIDs(t, 4)
	_, vaults := createTestNodeSet(t, 1)

	var winner int

	if nodes[0].String() < nodes[1].String() {
		winner = 0
	} else {
		winner = 1
	}

	// This test assumes there is only one ixn for the prepare msg
	testcases := []struct {
		name                    string
		ixns                    []*common.Interaction // each ixn has at least two participants
		senders                 []identifiers.KramaID
		nodeContextParticipants []identifiers.Identifier // Participants that have this node as their context
		expectedNodesIndexes    []int
	}{
		{
			name: "Non conflicting ixn",
			ixns: []*common.Interaction{
				tests.CreateIXWithParticipants(t, ids[:2], 0, nil),
			},
			senders:                 nodes[:1],
			nodeContextParticipants: ids[:2],
			expectedNodesIndexes:    []int{0},
		},
		{
			name: "Non conflicting ixns",
			ixns: []*common.Interaction{
				tests.CreateIXWithParticipants(t, ids[:2], 0, nil),
				tests.CreateIXWithParticipants(t, ids[2:4], 0, nil),
			},
			senders:                 nodes[:2],
			nodeContextParticipants: ids[1:3],
			expectedNodesIndexes:    []int{0, 1},
		},
		{
			name: "conflicting ixns from different nodes",
			ixns: []*common.Interaction{
				tests.CreateIXWithParticipants(t, ids[:2], 0, nil),
				tests.CreateIXWithParticipants(t, ids[1:3], 0, nil),
			},
			senders:                 nodes[:2],
			nodeContextParticipants: ids[1:2],
			expectedNodesIndexes:    []int{winner},
		},
		{
			name: "conflicting ixns among multiple participants",
			ixns: []*common.Interaction{
				tests.CreateIXWithParticipants(t, ids[:3], 0, nil),
				tests.CreateIXWithParticipants(t, ids[1:4], 0, nil),
			},
			senders:                 nodes[:2],
			nodeContextParticipants: ids[1:3],
			expectedNodesIndexes:    []int{winner},
		},
		{
			name: "conflicting ixns and non-conflicting ixns",
			ixns: []*common.Interaction{
				// conflicting ixns with p-1
				tests.CreateIXWithParticipants(t, ids[:2], 0, nil),
				tests.CreateIXWithParticipants(t, ids[1:3], 0, nil),
				// non-conflicting
				tests.CreateIXWithParticipants(t, ids[4:6], 0, nil),
				tests.CreateIXWithParticipants(t, ids[6:8], 0, nil),
			},
			senders:                 nodes[:4],
			nodeContextParticipants: []identifiers.Identifier{ids[1], ids[4], ids[6]},
			expectedNodesIndexes:    []int{winner, 2, 3},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			k := newTestKramaEngine(vaults[0])

			// insert prepare messages into krama instance of this node
			for i, ixn := range testcase.ixns {
				prepareIxns := common.NewInteractionsWithLeaderCheck(false, ixn)
				msg := &metaPrepareMsg{
					msg: &types.Prepare{
						Ixns: []common.Hash{ixn.Hash()},
					},
					ixns:        &prepareIxns,
					sender:      testcase.senders[i],
					shouldReply: true,
				}

				ps := ixn.Participants()

				insertPrepareMsg(k, testcase.nodeContextParticipants, msg, ps)
			}

			k.processPrepareMsgs(k.participantToPrepareMsg)

			ensureResponseMatches(t, k, testcase.expectedNodesIndexes, testcase.senders, testcase.ixns)
		})
	}
}
