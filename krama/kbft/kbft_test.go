package kbft

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common/tests"
	ktypes "github.com/sarvalabs/moichain/krama/types"
	"github.com/sarvalabs/moichain/types"
)

// common format for all tests
// 1. create vaults for all nodes which can be used to sign votes
// 2. create ics node set with those nodes
// 3. create cluster info from ics nodes, ixs, account meta info, tesseract grid with tesseracts of sender and receiver
// 4. create kbft  by using any one node from the above nodes for which consensus happens
// 5. listen on outbound channel to avoid blocking of algorithm
// 6. this node enters new round which is 0 and also starts handler
// 7. send votes and make sure to listen to them so that event mux won't get blocked
// 8. check if events are received for new round, new proposal, vote, timeouts,  polka

// 1. make sure this node prevoted
// 2. here all other nodes send exactly 2/3 rd prevotes including
// self vote on inbound channel and ensure they are received
// 3. make sure this node precommitted and received on outbound channel
// 4. all other nodes send exactly 2/3 rd precommit including self vote on inbound channel and ensure they are received
// 5. we have selected random set as 32 just to match practical ics set where random set is 2*context nodes
// 6. make sure tesseracts added to chain
func TestFullRound_WithMultipleNodes(t *testing.T) {
	t.Parallel()

	out := make(chan ktypes.ConsensusMessage)
	in := make(chan ktypes.ConsensusMessage)
	c := defaultChain()
	heights := []uint64{2, 3}
	round := int32(0)

	icsNodes, valSet := createICSNodes(t, 4, 4, 4,
		4, 32, 0)

	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs, false)
	ctx := context.Background()
	thisNode := valSet[ktypes.SenderBehaviourSet][0]

	ixHash, err := clusterInfo.Ixs.Hash()
	require.NoError(t, err)

	kbft := NewKBFTService(
		ctx,
		thisNode.KramaID(),
		createTestConsensusConfig(),
		out,
		in,
		thisNode,
		clusterInfo,
		c.finalizeTesseractGrid,
		withDefaultEventMux(),
		WithLogger(hclog.NewNullLogger()),
		WithWal(nullWal{}),
		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
	)

	newRoundSub := kbft.mux.Subscribe(eventNewRound{})
	proposalSub := kbft.mux.Subscribe(eventProposal{})
	voteSub := kbft.mux.Subscribe(eventVote{})
	polkaSub := kbft.mux.Subscribe(eventPolka{})

	kbftErr := make(chan error, 1)

	handleOutboundMsgChannel(kbft, ctx, out)

	go startTestRound(kbft, heights, round, kbftErr)

	ensureNewRound(t, newRoundSub, heights, round)
	ensureProposal(t, proposalSub, heights, round)

	gridID, err := kbft.ProposalGrid.GetTesseractGridID()
	require.NoError(t, err)

	ensurePrevote(t, voteSub, gridID, round, heights)
	validatePrevote(t, kbft, round, thisNode, gridID.Hash)

	// 6 nodes out of 8 sender nodes prevotes on grid
	// 6 nodes out of 8 receiver nodes prevotes on grid
	// 22 nodes out of 32 sender nodes prevotes on grid
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderBehaviourSet][1:3]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverBehaviourSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.RandomSet][:22]...)

	ensurePolka(t, polkaSub, heights, round)
	ensurePrecommit(t, voteSub, gridID, round, heights)
	validatePrecommit(t, kbft, 0, 0, thisNode, gridID.Hash, gridID.Hash)

	// 6 nodes out of 8 sender nodes precommits on grid
	// 6 nodes out of 8 receiver nodes precommits on grid
	// 22 nodes out of 32 sender nodes precommits on grid
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderBehaviourSet][1:3]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderRandomSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverBehaviourSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverRandomSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.RandomSet][:22]...)

	ensureNoError(t, kbftErr)
	require.Equal(t, c.tsCount, len(heights))
}

// This test is same as TestFullRound_WithMultipleNodes, except that receiver of interaction is non-registered
func TestFullRound_WithNonRegisteredReceiver(t *testing.T) {
	t.Parallel()

	out := make(chan ktypes.ConsensusMessage)
	in := make(chan ktypes.ConsensusMessage)
	c := new(chain)
	heights := []uint64{2, 0, 9}
	round := int32(0)

	// 32 random nodes are chosen to simulate reality 2 * context node(16)
	icsNodes, valSet := createICSNodes(t, 4, 4, 4,
		4, 32, 0)

	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs, true)
	ctx := context.Background()
	thisNode := valSet[ktypes.SenderBehaviourSet][0]

	ixHash, err := clusterInfo.Ixs.Hash()
	require.NoError(t, err)

	kbft := NewKBFTService(
		ctx,
		thisNode.KramaID(),
		createTestConsensusConfig(),
		out,
		in,
		thisNode,
		clusterInfo,
		c.finalizeTesseractGrid,
		withDefaultEventMux(),
		WithLogger(hclog.NewNullLogger()),
		WithWal(nullWal{}),
		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
	)

	newRoundSub := kbft.mux.Subscribe(eventNewRound{})
	proposalSub := kbft.mux.Subscribe(eventProposal{})
	voteSub := kbft.mux.Subscribe(eventVote{})
	polkaSub := kbft.mux.Subscribe(eventPolka{})

	kbftErr := make(chan error, 1)

	handleOutboundMsgChannel(kbft, ctx, out)

	go startTestRound(kbft, heights, round, kbftErr)

	ensureNewRound(t, newRoundSub, heights, round)
	ensureProposal(t, proposalSub, heights, round)

	gridID, err := kbft.ProposalGrid.GetTesseractGridID()
	require.NoError(t, err)

	ensurePrevote(t, voteSub, gridID, round, heights)
	validatePrevote(t, kbft, round, thisNode, gridID.Hash)

	// 6 nodes out of 8 sender nodes prevotes on grid
	// 6 nodes out of 8 receiver nodes prevotes on grid
	// 22 nodes out of 32 sender nodes prevotes on grid
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderBehaviourSet][1:3]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverBehaviourSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.RandomSet][:22]...)

	ensurePolka(t, polkaSub, heights, round)
	ensurePrecommit(t, voteSub, gridID, round, heights)
	validatePrecommit(t, kbft, 0, 0, thisNode, gridID.Hash, gridID.Hash)

	// 6 nodes out of 8 sender nodes precommits on gridkbft context gets timed out
	// 6 nodes out of 8 receiver nodes precommits on grid
	// 22 nodes out of 32 sender nodes precommits on grid
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderBehaviourSet][1:3]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderRandomSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverBehaviourSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverRandomSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.RandomSet][:22]...)

	ensureNoError(t, kbftErr)
	require.Equal(t, c.tsCount, len(heights))
}

// This test is same as TestFullRound_WithMultipleNodes, except that receiver of interaction is nil
func TestFullRound_WithNilReceiverAddress(t *testing.T) {
	t.Parallel()

	out := make(chan ktypes.ConsensusMessage)
	in := make(chan ktypes.ConsensusMessage)
	c := new(chain)
	heights := []uint64{2}
	round := int32(0)

	icsNodes, valSet := createICSNodes(t, 4, 4, 4,
		4, 32, 0)

	ixs := createIxs(t, tests.RandomAddress(t), types.NilAddress)
	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs, false)
	ctx := context.Background()
	thisNode := valSet[ktypes.SenderBehaviourSet][0]

	ixHash, err := clusterInfo.Ixs.Hash()
	require.NoError(t, err)

	kbft := NewKBFTService(
		ctx,
		thisNode.KramaID(),
		createTestConsensusConfig(),
		out,
		in,
		thisNode,
		clusterInfo,
		c.finalizeTesseractGrid,
		withDefaultEventMux(),
		WithLogger(hclog.NewNullLogger()),
		WithWal(nullWal{}),
		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
	)

	newRoundSub := kbft.mux.Subscribe(eventNewRound{})
	proposalSub := kbft.mux.Subscribe(eventProposal{})
	voteSub := kbft.mux.Subscribe(eventVote{})
	polkaSub := kbft.mux.Subscribe(eventPolka{})

	kbftErr := make(chan error, 1)

	handleOutboundMsgChannel(kbft, ctx, out)

	go startTestRound(kbft, heights, round, kbftErr)

	ensureNewRound(t, newRoundSub, heights, round)
	ensureProposal(t, proposalSub, heights, round)

	gridID, err := kbft.ProposalGrid.GetTesseractGridID()
	require.NoError(t, err)

	ensurePrevote(t, voteSub, gridID, round, heights)
	validatePrevote(t, kbft, round, thisNode, gridID.Hash)

	// 6 nodes out of 8 sender nodes prevotes on grid
	// 6 nodes out of 8 receiver nodes prevotes on grid
	// 22 nodes out of 32 sender nodes prevotes on grid
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderBehaviourSet][1:3]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverBehaviourSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.RandomSet][:22]...)

	ensurePolka(t, polkaSub, heights, round)
	ensurePrecommit(t, voteSub, gridID, round, heights)
	validatePrecommit(t, kbft, 0, 0, thisNode, gridID.Hash, gridID.Hash)

	// 6 nodes out of 8 sender nodes precommits on grid
	// 6 nodes out of 8 receiver nodes precommits on grid
	// 22 nodes out of 32 sender nodes precommits on grid
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderBehaviourSet][1:3]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderRandomSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverBehaviourSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverRandomSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.RandomSet][:22]...)

	ensureNoError(t, kbftErr)
	require.Equal(t, c.tsCount, len(heights))
}

// 1. only 2/3 -1 nodes send prevotes
// 2. make sure this node shouldn't send precommit
// 3. kbft timeout occurs
func TestFullRound_WithLessThan23rdPrevotes(t *testing.T) {
	t.Parallel()

	out := make(chan ktypes.ConsensusMessage)
	in := make(chan ktypes.ConsensusMessage)
	c := defaultChain()
	heights := []uint64{2, 3}
	round := int32(0)

	icsNodes, valSet := createICSNodes(t, 4, 4, 4,
		4, 32, 0)

	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs, false)
	ctx := context.Background()
	thisNode := valSet[ktypes.SenderBehaviourSet][0]

	ixHash, err := clusterInfo.Ixs.Hash()
	require.NoError(t, err)

	kbft := NewKBFTService(
		ctx,
		thisNode.KramaID(),
		createTestConsensusConfig(),
		out,
		in,
		thisNode,
		clusterInfo,
		c.finalizeTesseractGrid,
		withDefaultEventMux(),
		WithLogger(hclog.NewNullLogger()),
		WithWal(nullWal{}),
		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
	)

	newRoundSub := kbft.mux.Subscribe(eventNewRound{})
	proposalSub := kbft.mux.Subscribe(eventProposal{})
	voteSub := kbft.mux.Subscribe(eventVote{})

	kbftErr := make(chan error, 1)

	handleOutboundMsgChannel(kbft, ctx, out)

	go startTestRound(kbft, heights, round, kbftErr)

	ensureNewRound(t, newRoundSub, heights, round)
	ensureProposal(t, proposalSub, heights, round)

	gridID, err := kbft.ProposalGrid.GetTesseractGridID()
	require.NoError(t, err)

	ensurePrevote(t, voteSub, gridID, round, heights)
	validatePrevote(t, kbft, round, thisNode, gridID.Hash)

	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderBehaviourSet][1:2]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverBehaviourSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.RandomSet][:22]...)

	ensureNoPrecommitReceived(t, voteSub)

	// kbft context gets timed out
	ensureError(t, kbftErr, "context deadline exceeded")
	require.Equal(t, c.tsCount, -1)
}

// 1. only 2/3 -1 nodes send precommit
// 2. make sure this node shouldn't add grid to chains
// 3. kbft timeout occurs
func TestFullRound_WithLessThan23rdPrecommit(t *testing.T) {
	t.Parallel()

	out := make(chan ktypes.ConsensusMessage)
	in := make(chan ktypes.ConsensusMessage)
	c := defaultChain()
	heights := []uint64{2, 3}
	round := int32(0)

	icsNodes, valSet := createICSNodes(t, 4, 4, 4,
		4, 32, 0)

	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs, false)
	ctx := context.Background()
	thisNode := valSet[ktypes.SenderBehaviourSet][0]

	ixHash, err := clusterInfo.Ixs.Hash()
	require.NoError(t, err)

	kbft := NewKBFTService(
		ctx,
		thisNode.KramaID(),
		createTestConsensusConfig(),
		out,
		in,
		thisNode,
		clusterInfo,
		c.finalizeTesseractGrid,
		withDefaultEventMux(),
		WithLogger(hclog.NewNullLogger()),
		WithWal(nullWal{}),
		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
	)

	newRoundSub := kbft.mux.Subscribe(eventNewRound{})
	proposalSub := kbft.mux.Subscribe(eventProposal{})
	voteSub := kbft.mux.Subscribe(eventVote{})

	kbftErr := make(chan error, 1)

	handleOutboundMsgChannel(kbft, ctx, out)

	go startTestRound(kbft, heights, round, kbftErr)

	ensureNewRound(t, newRoundSub, heights, round)
	ensureProposal(t, proposalSub, heights, round)

	gridID, err := kbft.ProposalGrid.GetTesseractGridID()
	require.NoError(t, err)

	ensurePrevote(t, voteSub, gridID, round, heights)
	validatePrevote(t, kbft, round, thisNode, gridID.Hash)

	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderBehaviourSet][1:3]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverBehaviourSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.RandomSet][:22]...)

	ensurePrecommit(t, voteSub, gridID, round, heights)
	validatePrecommit(t, kbft, 0, 0, thisNode, gridID.Hash, gridID.Hash)

	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderBehaviourSet][1:2]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderRandomSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverBehaviourSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverRandomSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.RandomSet][:22]...)

	// kbft context gets timed out
	ensureError(t, kbftErr, "context deadline exceeded")
	require.Equal(t, c.tsCount, -1)
}

// 1. if any 2/3 rd nodes prevote then should enter prevote wait
// 2. this node should precommit nil
func TestFullRound_WithAny23rdPrevote_Any23rdPrecommit(t *testing.T) {
	t.Parallel()

	out := make(chan ktypes.ConsensusMessage)
	in := make(chan ktypes.ConsensusMessage)
	c := defaultChain()
	heights := []uint64{2, 3}
	round := int32(0)

	icsNodes, valSet := createICSNodes(t, 4, 4, 4,
		4, 32, 0)

	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs, false)
	ctx := context.Background()
	thisNode := valSet[ktypes.SenderBehaviourSet][0]

	ixHash, err := clusterInfo.Ixs.Hash()
	require.NoError(t, err)

	kbft := NewKBFTService(
		ctx,
		thisNode.KramaID(),
		createTestConsensusConfig(),
		out,
		in,
		thisNode,
		clusterInfo,
		c.finalizeTesseractGrid,
		withDefaultEventMux(),
		WithLogger(hclog.NewNullLogger()),
		WithWal(nullWal{}),
		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
	)

	newRoundSub := kbft.mux.Subscribe(eventNewRound{})
	proposalSub := kbft.mux.Subscribe(eventProposal{})
	voteSub := kbft.mux.Subscribe(eventVote{})
	prevoteTimeoutSub := kbft.mux.Subscribe(eventTimeoutPrevote{})
	precommitTimeoutSub := kbft.mux.Subscribe(eventTimeoutPrecommit{})

	kbftErr := make(chan error, 1)

	handleOutboundMsgChannel(kbft, ctx, out)

	go startTestRound(kbft, heights, round, kbftErr)

	ensureNewRound(t, newRoundSub, heights, round)
	ensureProposal(t, proposalSub, heights, round)

	gridID, err := kbft.ProposalGrid.GetTesseractGridID()
	require.NoError(t, err)

	ensurePrevote(t, voteSub, gridID, round, heights)
	validatePrevote(t, kbft, round, thisNode, gridID.Hash)

	randomGridID := createGridWithHeights(t, heights, tests.RandomHash(t))
	sendAndEnsurePreVote(t, kbft, round, randomGridID, voteSub, round, valSet[ktypes.SenderBehaviourSet][1:3]...)
	sendAndEnsurePreVote(t, kbft, round, randomGridID, voteSub, round, valSet[ktypes.SenderRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, nil, voteSub, round, valSet[ktypes.ReceiverBehaviourSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, nil, voteSub, round, valSet[ktypes.ReceiverRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, nil, voteSub, round, valSet[ktypes.RandomSet][:22]...)

	ensurePrevoteTimeout(t, prevoteTimeoutSub, heights, round, kbft.config.PrevoteWaitDuration(0).Nanoseconds())
	ensurePrecommit(t, voteSub, nil, round, heights)
	validatePrecommit(t, kbft, 0, 0, thisNode, types.NilHash, types.NilHash)

	sendAndEnsurePrecommit(t, kbft, round, randomGridID, voteSub, round, valSet[ktypes.SenderBehaviourSet][1:3]...)
	sendAndEnsurePrecommit(t, kbft, round, randomGridID, voteSub, round, valSet[ktypes.SenderRandomSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, nil, voteSub, round, valSet[ktypes.ReceiverBehaviourSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, nil, voteSub, round, valSet[ktypes.ReceiverRandomSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, nil, voteSub, round, valSet[ktypes.RandomSet][:22]...)

	ensurePrecommitTimeout(t, precommitTimeoutSub, heights, round,
		kbft.config.PrecommitWaitDuration(0).Nanoseconds())

	ensureError(t, kbftErr, "context deadline exceeded")
	require.Equal(t, c.tsCount, -1)
}

// 1. if 2/3 rd nodes precommit nil then tesseract shouldn't get added
// 2. should enter precommit wait timeout
func TestFullRound_With23rdNilPrecommit(t *testing.T) {
	t.Parallel()

	out := make(chan ktypes.ConsensusMessage)
	in := make(chan ktypes.ConsensusMessage)
	c := defaultChain()
	heights := []uint64{2, 3}
	round := int32(0)

	icsNodes, valSet := createICSNodes(t, 4, 4, 4,
		4, 32, 0)

	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs, false)
	ctx := context.Background()
	thisNode := valSet[ktypes.SenderBehaviourSet][0]

	ixHash, err := clusterInfo.Ixs.Hash()
	require.NoError(t, err)

	kbft := NewKBFTService(
		ctx,
		thisNode.KramaID(),
		createTestConsensusConfig(),
		out,
		in,
		thisNode,
		clusterInfo,
		c.finalizeTesseractGrid,
		withDefaultEventMux(),
		WithLogger(hclog.NewNullLogger()),
		WithWal(nullWal{}),
		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
	)

	newRoundSub := kbft.mux.Subscribe(eventNewRound{})
	proposalSub := kbft.mux.Subscribe(eventProposal{})
	voteSub := kbft.mux.Subscribe(eventVote{})
	precommitTimeoutSub := kbft.mux.Subscribe(eventTimeoutPrecommit{})

	kbftErr := make(chan error, 1)

	handleOutboundMsgChannel(kbft, ctx, out)

	go startTestRound(kbft, heights, round, kbftErr)

	ensureNewRound(t, newRoundSub, heights, round)
	ensureProposal(t, proposalSub, heights, round)

	gridID, err := kbft.ProposalGrid.GetTesseractGridID()
	require.NoError(t, err)

	ensurePrevote(t, voteSub, gridID, round, heights)
	validatePrevote(t, kbft, round, thisNode, gridID.Hash)

	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderBehaviourSet][1:3]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverBehaviourSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.RandomSet][:22]...)

	ensurePrecommit(t, voteSub, gridID, round, heights)
	validatePrecommit(t, kbft, 0, 0, thisNode, gridID.Hash, gridID.Hash)

	sendAndEnsurePrecommit(t, kbft, round, nil, voteSub, round, valSet[ktypes.SenderBehaviourSet][1:3]...)
	sendAndEnsurePrecommit(t, kbft, round, nil, voteSub, round, valSet[ktypes.SenderRandomSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, nil, voteSub, round, valSet[ktypes.ReceiverBehaviourSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, nil, voteSub, round, valSet[ktypes.ReceiverRandomSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, nil, voteSub, round, valSet[ktypes.RandomSet][:22]...)

	ensurePrecommitTimeout(
		t,
		precommitTimeoutSub,
		heights,
		round,
		kbft.config.PrecommitWaitDuration(0).Nanoseconds(),
	)

	ensureError(t, kbftErr, "context deadline exceeded")
	require.Equal(t, c.tsCount, -1)
}

// sign random grid two times by same validator
// check if signatures match
func TestSignVote(t *testing.T) {
	t.Parallel()

	out := make(chan ktypes.ConsensusMessage)
	in := make(chan ktypes.ConsensusMessage)
	c := defaultChain()
	heights := []uint64{2, 3}

	icsNodes, valSet := createICSNodes(t, 1, 0, 0,
		0, 0, 0)

	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs, false)
	ctx := context.Background()
	thisNode := valSet[ktypes.SenderBehaviourSet][0]

	ixHash, err := clusterInfo.Ixs.Hash()
	require.NoError(t, err)

	kbft := NewKBFTService(
		ctx,
		thisNode.KramaID(),
		createTestConsensusConfig(),
		out,
		in,
		thisNode,
		clusterInfo,
		c.finalizeTesseractGrid,
		withDefaultEventMux(),
		WithLogger(hclog.NewNullLogger()),
		WithWal(nullWal{}),
		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
	)

	grid := createGridWithHeights(t, heights, tests.RandomHash(t))
	firstVote, err := kbft.signVote(ktypes.PRECOMMIT, grid)
	require.NoError(t, err)

	secondVote, err := kbft.signVote(ktypes.PRECOMMIT, grid)
	require.NoError(t, err)

	require.Equal(t, firstVote.Signature, secondVote.Signature)
}

// 1. validator in sender behaviour set prevotes on both gridID and nil grid ID which is conflict of vote for that round
// 2. check if evidence added
func TestConflictPrevote(t *testing.T) {
	t.Parallel()

	out := make(chan ktypes.ConsensusMessage)
	in := make(chan ktypes.ConsensusMessage)
	c := defaultChain()
	heights := []uint64{2, 3}
	round := int32(0)

	icsNodes, valSet := createICSNodes(t, 2, 0,
		0, 0, 0, 0)

	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs, false)
	ctx := context.Background()
	thisNode := valSet[ktypes.SenderBehaviourSet][0]

	ixHash, err := clusterInfo.Ixs.Hash()
	require.NoError(t, err)

	kbft := NewKBFTService(
		ctx,
		thisNode.KramaID(),
		createTestConsensusConfig(),
		out,
		in,
		thisNode,
		clusterInfo,
		c.finalizeTesseractGrid,
		withDefaultEventMux(),
		WithLogger(hclog.NewNullLogger()),
		WithWal(nullWal{}),
		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
	)

	newRoundSub := kbft.mux.Subscribe(eventNewRound{})
	proposalSub := kbft.mux.Subscribe(eventProposal{})
	voteSub := kbft.mux.Subscribe(eventVote{})

	kbftErr := make(chan error, 1)

	handleOutboundMsgChannel(kbft, ctx, out)

	go startTestRound(kbft, heights, round, kbftErr)

	ensureNewRound(t, newRoundSub, heights, round)
	ensureProposal(t, proposalSub, heights, round)

	gridID, err := kbft.ProposalGrid.GetTesseractGridID()
	require.NoError(t, err)

	ensurePrevote(t, voteSub, gridID, round, heights)
	validatePrevote(t, kbft, round, thisNode, gridID.Hash)

	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderBehaviourSet][1])
	signAddVotesSynchronously(t, kbft, round, ktypes.PREVOTE, nil, valSet[ktypes.SenderBehaviourSet][1]) // conflict vote

	time.Sleep(5 * time.Millisecond) // wait for 5ms so that conflict prevote gets processed

	expectedConflictVote := signVote(t, kbft, round, ktypes.PREVOTE, nil, valSet[ktypes.SenderBehaviourSet][1])

	require.Equal(t, expectedConflictVote, kbft.evidence.Votes[len(kbft.evidence.Votes)-1])
}

// 1. validator in sender behaviour set precommits on both gridID and nil grid ID
// which is conflict of vote for that round
// 2. check if evidence added
func TestConflictPrecommit(t *testing.T) {
	t.Parallel()

	out := make(chan ktypes.ConsensusMessage)
	in := make(chan ktypes.ConsensusMessage)
	c := defaultChain()
	heights := []uint64{2, 3}
	round := int32(0)

	icsNodes, valSet := createICSNodes(t, 4, 4, 4,
		4, 32, 0)

	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs, false)
	ctx := context.Background()
	thisNode := valSet[ktypes.SenderBehaviourSet][0]

	ixHash, err := clusterInfo.Ixs.Hash()
	require.NoError(t, err)

	kbft := NewKBFTService(
		ctx,
		thisNode.KramaID(),
		createTestConsensusConfig(),
		out,
		in,
		thisNode,
		clusterInfo,
		c.finalizeTesseractGrid,
		withDefaultEventMux(),
		WithLogger(hclog.NewNullLogger()),
		WithWal(nullWal{}),
		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
	)

	newRoundSub := kbft.mux.Subscribe(eventNewRound{})
	proposalSub := kbft.mux.Subscribe(eventProposal{})
	voteSub := kbft.mux.Subscribe(eventVote{})

	kbftErr := make(chan error, 1)

	handleOutboundMsgChannel(kbft, ctx, out)

	go startTestRound(kbft, heights, round, kbftErr)

	ensureNewRound(t, newRoundSub, heights, round)
	ensureProposal(t, proposalSub, heights, round)

	gridID, err := kbft.ProposalGrid.GetTesseractGridID()
	require.NoError(t, err)

	ensurePrevote(t, voteSub, gridID, round, heights)
	validatePrevote(t, kbft, round, thisNode, gridID.Hash)

	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderBehaviourSet][1:3]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverBehaviourSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.RandomSet][:22]...)

	ensurePrecommit(t, voteSub, gridID, round, heights)
	validatePrecommit(t, kbft, 0, 0, thisNode, gridID.Hash, gridID.Hash)

	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderBehaviourSet][1])
	signAddVotesSynchronously(t, kbft, round, ktypes.PRECOMMIT, nil, valSet[ktypes.SenderBehaviourSet][1]) // conflict vote

	time.Sleep(5 * time.Millisecond) // wait for 5ms so that conflict precommit gets processed

	expectedConflictVote := signVote(t,
		kbft,
		round,
		ktypes.PRECOMMIT,
		nil,
		valSet[ktypes.SenderBehaviourSet][1],
	)

	require.Equal(t, expectedConflictVote, kbft.evidence.Votes[len(kbft.evidence.Votes)-1])
}

// we make sure only 2/3 -1 prevotes received
// then prevote from random validator shouldn't trigger precommit
func TestRandomValidatorPreVote(t *testing.T) {
	t.Parallel()

	out := make(chan ktypes.ConsensusMessage)
	in := make(chan ktypes.ConsensusMessage)
	c := defaultChain()
	heights := []uint64{2, 3}
	round := int32(0)

	icsNodes, valSet := createICSNodes(t, 4, 4, 4,
		4, 32, 0)

	_, randomVal := createTestNodeSet(t, 1)

	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs, false)
	ctx := context.Background()
	thisNode := valSet[ktypes.SenderBehaviourSet][0]

	ixHash, err := clusterInfo.Ixs.Hash()
	require.NoError(t, err)

	kbft := NewKBFTService(
		ctx,
		thisNode.KramaID(),
		createTestConsensusConfig(),
		out,
		in,
		thisNode,
		clusterInfo,
		c.finalizeTesseractGrid,
		withDefaultEventMux(),
		WithLogger(hclog.NewNullLogger()),
		WithWal(nullWal{}),
		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
	)

	newRoundSub := kbft.mux.Subscribe(eventNewRound{})
	proposalSub := kbft.mux.Subscribe(eventProposal{})
	voteSub := kbft.mux.Subscribe(eventVote{})

	kbftErr := make(chan error, 1)

	handleOutboundMsgChannel(kbft, ctx, out)

	go startTestRound(kbft, heights, round, kbftErr)

	ensureNewRound(t, newRoundSub, heights, round)
	ensureProposal(t, proposalSub, heights, round)

	gridID, err := kbft.ProposalGrid.GetTesseractGridID()
	require.NoError(t, err)

	ensurePrevote(t, voteSub, gridID, round, heights)
	validatePrevote(t, kbft, round, thisNode, gridID.Hash)

	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderBehaviourSet][1:3]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverBehaviourSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.RandomSet][:21]...)

	signAddVotes(t, kbft, round, ktypes.PREVOTE, gridID, randomVal[0])

	ensureNoPrecommitReceived(t, voteSub)
}

// we make sure only 2/3 -1 precommit received
// then precommit from random validator shouldn't add grid to chain
func TestRandomValidatorPrecommit(t *testing.T) {
	t.Parallel()

	out := make(chan ktypes.ConsensusMessage)
	in := make(chan ktypes.ConsensusMessage)
	c := defaultChain()
	heights := []uint64{2, 3}
	round := int32(0)

	icsNodes, valSet := createICSNodes(t, 4, 4, 4,
		4, 32, 0)

	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs, false)
	ctx := context.Background()
	thisNode := valSet[ktypes.SenderBehaviourSet][0]

	ixHash, err := clusterInfo.Ixs.Hash()
	require.NoError(t, err)

	kbft := NewKBFTService(
		ctx,
		thisNode.KramaID(),
		createTestConsensusConfig(),
		out,
		in,
		thisNode,
		clusterInfo,
		c.finalizeTesseractGrid,
		withDefaultEventMux(),
		WithLogger(hclog.NewNullLogger()),
		WithWal(nullWal{}),
		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
	)

	_, randomVal := createTestNodeSet(t, 1)

	newRoundSub := kbft.mux.Subscribe(eventNewRound{})
	proposalSub := kbft.mux.Subscribe(eventProposal{})
	voteSub := kbft.mux.Subscribe(eventVote{})

	kbftErr := make(chan error, 1)

	handleOutboundMsgChannel(kbft, ctx, out)

	go startTestRound(kbft, heights, round, kbftErr)

	ensureNewRound(t, newRoundSub, heights, round)
	ensureProposal(t, proposalSub, heights, round)

	gridID, err := kbft.ProposalGrid.GetTesseractGridID()
	require.NoError(t, err)

	ensurePrevote(t, voteSub, gridID, round, heights)
	validatePrevote(t, kbft, round, thisNode, gridID.Hash)

	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderBehaviourSet][1:3]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverBehaviourSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.RandomSet][:22]...)

	ensurePrecommit(t, voteSub, gridID, round, heights)
	validatePrecommit(t, kbft, 0, 0, thisNode, gridID.Hash, gridID.Hash)

	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderBehaviourSet][1:2]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderRandomSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverBehaviourSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverRandomSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.RandomSet][:21]...)

	signAddVotes(t, kbft, round, ktypes.PRECOMMIT, gridID, randomVal[0])

	// kbft context gets timed out
	ensureError(t, kbftErr, "context deadline exceeded")
	require.Equal(t, c.tsCount, -1)
}

// 1. enter prevoteWait after any 2/3 rd maj
// 2. receives 2/3 rd prevotes during wait
// 3. This node should vote precommit
func TestReceivePrevoteDuringPrevoteWait(t *testing.T) {
	t.Parallel()

	out := make(chan ktypes.ConsensusMessage)
	in := make(chan ktypes.ConsensusMessage)
	c := defaultChain()
	heights := []uint64{2, 3}
	round := int32(0)

	icsNodes, valSet := createICSNodes(t, 4, 4, 4,
		4, 32, 0)

	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs, false)
	ctx := context.Background()
	thisNode := valSet[ktypes.SenderBehaviourSet][0]

	ixHash, err := clusterInfo.Ixs.Hash()
	require.NoError(t, err)

	kbft := NewKBFTService(
		ctx,
		thisNode.KramaID(),
		createTestConsensusConfig(),
		out,
		in,
		thisNode,
		clusterInfo,
		c.finalizeTesseractGrid,
		withDefaultEventMux(),
		WithLogger(hclog.NewNullLogger()),
		WithWal(nullWal{}),
		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
	)

	newRoundSub := kbft.mux.Subscribe(eventNewRound{})
	proposalSub := kbft.mux.Subscribe(eventProposal{})
	voteSub := kbft.mux.Subscribe(eventVote{})

	kbftErr := make(chan error, 1)

	handleOutboundMsgChannel(kbft, ctx, out)

	go startTestRound(kbft, heights, round, kbftErr)

	ensureNewRound(t, newRoundSub, heights, round)
	ensureProposal(t, proposalSub, heights, round)

	gridID, err := kbft.ProposalGrid.GetTesseractGridID()
	require.NoError(t, err)

	ensurePrevote(t, voteSub, gridID, round, heights)
	validatePrevote(t, kbft, round, thisNode, gridID.Hash)

	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderBehaviourSet][1:3]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverBehaviourSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.RandomSet][:21]...)

	// 2/3 rd any majority is reached with the following vote
	sendAndEnsurePreVote(t, kbft, round, nil, voteSub, round, valSet[ktypes.RandomSet][21])
	time.Sleep(5 * time.Millisecond) // wait for 5 ms as prevote timeout is 10 ms
	// send the 2/3 rd prevote
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.RandomSet][22])

	ensurePrecommit(t, voteSub, gridID, round, heights)
	validatePrecommit(t, kbft, 0, 0, thisNode, gridID.Hash, gridID.Hash)
}

// 1. enter precommitWait after any 2/3 rd maj
// 2. receives 2/3 rd precommits during wait
// 3. This node should add tesseract grid to chain
func TestReceivePrecommitDuringPrecommmitWait(t *testing.T) {
	t.Parallel()

	out := make(chan ktypes.ConsensusMessage)
	in := make(chan ktypes.ConsensusMessage)
	c := defaultChain()
	heights := []uint64{2, 3}
	round := int32(0)

	icsNodes, valSet := createICSNodes(t, 4, 4, 4,
		4, 32, 0)

	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs, false)
	ctx := context.Background()
	thisNode := valSet[ktypes.SenderBehaviourSet][0]

	ixHash, err := clusterInfo.Ixs.Hash()
	require.NoError(t, err)

	kbft := NewKBFTService(
		ctx,
		thisNode.KramaID(),
		createTestConsensusConfig(),
		out,
		in,
		thisNode,
		clusterInfo,
		c.finalizeTesseractGrid,
		withDefaultEventMux(),
		WithLogger(hclog.NewNullLogger()),
		WithWal(nullWal{}),
		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
	)

	newRoundSub := kbft.mux.Subscribe(eventNewRound{})
	proposalSub := kbft.mux.Subscribe(eventProposal{})
	voteSub := kbft.mux.Subscribe(eventVote{})

	kbftErr := make(chan error, 1)

	handleOutboundMsgChannel(kbft, ctx, out)

	go startTestRound(kbft, heights, round, kbftErr)

	ensureNewRound(t, newRoundSub, heights, round)
	ensureProposal(t, proposalSub, heights, round)

	gridID, err := kbft.ProposalGrid.GetTesseractGridID()
	require.NoError(t, err)

	ensurePrevote(t, voteSub, gridID, round, heights)
	validatePrevote(t, kbft, round, thisNode, gridID.Hash)

	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderBehaviourSet][1:3]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverBehaviourSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverRandomSet][1:4]...)
	sendAndEnsurePreVote(t, kbft, round, gridID, voteSub, round, valSet[ktypes.RandomSet][:22]...)

	ensurePrecommit(t, voteSub, gridID, round, heights)
	validatePrecommit(t, kbft, 0, 0, thisNode, gridID.Hash, gridID.Hash)

	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderBehaviourSet][1:3]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.SenderRandomSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverBehaviourSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.ReceiverRandomSet][1:4]...)
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.RandomSet][:21]...)

	// any 2/3 rd majority
	sendAndEnsurePrecommit(t, kbft, round, nil, voteSub, round, valSet[ktypes.RandomSet][21])
	time.Sleep(5 * time.Millisecond) // wait for 5 ms as precommit timeout is 10 ms
	// send the 2/3 rd precommit
	sendAndEnsurePrecommit(t, kbft, round, gridID, voteSub, round, valSet[ktypes.RandomSet][22])

	ensureNoError(t, kbftErr)
	require.Equal(t, c.tsCount, len(heights))
}
