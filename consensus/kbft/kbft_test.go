package kbft

//
// import (
//	"context"
//	"testing"
//	"time"
//
//	"github.com/hashicorp/go-hclog"
//	identifiers "github.com/sarvalabs/go-moi-identifiers"
//	"github.com/sarvalabs/go-moi/common"
//	"github.com/stretchr/testify/require"
//
//	"github.com/sarvalabs/go-moi/common/tests"
//	ktypes "github.com/sarvalabs/go-moi/consensus/types"
//)
//
//// common format for all tests
//// 1. create vaults for all nodes which can be used to sign votes
//// 2. create ics node set with those nodes
//// 3. create cluster info from ics nodes, ixs, account meta info, tesseract with participants of sender and receiver
//// 4. create kbft  by using any one node from the above nodes for which consensus happens
//// 5. listen on outbound channel to avoid blocking of algorithm
//// 6. this node enters new view which is 0 and also starts handler
//// 7. send votes and make sure to listen to them so that event mux won't get blocked
//// 8. check if events are received for new view, new proposal, vote, timeouts,  polka
//
//// 1. make sure this node prevoted
//// 2. here all other nodes send exactly 2/3 rd prevotes including
//// self vote on inbound channel and ensure they are received
//// 3. make sure this node precommitted and received on outbound channel
//// 4. all other nodes send exactly 2/3 rd precommit including self vote
// on inbound channel and ensure they are received
//// 5. we have selected random set as 32 just to match practical ics set where random set is 2*context nodes
//// 6. make sure tesseracts added to chain
// func TestFullRound_WithMultipleNodes(t *testing.T) {
//	t.Parallel()
//
//	out := make(chan ktypes.ConsensusMessage)
//	in := make(chan ktypes.ConsensusMessage)
//	c := defaultChain()
//	round := int32(0)
//
//	icsNodes, valSet := createICSNodes(t, 2, 4, 32, 0)
//
//	// create ixn with two participants
//	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
//
//	heights := map[identifiers.Address]uint64{
//		ixs[0].Sender():                2,
//		ixs[0].Transaction(0).Target(): 3,
//	}
//
//	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs)
//	voteset := ktypes.NewHeightVoteSet(make([]string, 0), clusterInfo.NewHeights(), clusterInfo, hclog.NewNullLogger())
//	ctx := context.Background()
//	thisNode := valSet[0][0]
//
//	ixHash, err := clusterInfo.Ixns().Hash()
//	require.NoError(t, err)
//
//	kbft := NewKBFTService(
//		ctx,
//		TestBFTimeout,
//		thisNode.KramaID(),
//		createTestConsensusConfig(),
//		out,
//		in,
//		thisNode,
//		clusterInfo,
//		voteset,
//		c.finalizeTesseractGrid,
//		withDefaultEventMux(),
//		WithLogger(hclog.NewNullLogger()),
//		WithWal(NullWal{}),
//		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
//	)
//
//	newRoundSub := kbft.mux.Subscribe(eventNewView{})
//	proposalSub := kbft.mux.Subscribe(eventProposal{})
//	voteSub := kbft.mux.Subscribe(eventVote{})
//	polkaSub := kbft.mux.Subscribe(eventPolka{})
//
//	kbftErr := make(chan error, 1)
//
//	handleOutboundMsgChannel(kbft, ctx, out)
//
//	go startTestRound(kbft, heights, round, kbftErr)
//
//	ensureNewRound(t, newRoundSub, heights, round)
//	ensureProposal(t, proposalSub, heights, round)
//
//	require.NotNil(t, kbft.ProposalTS)
//	tsHash := kbft.ProposalTS.Hash()
//
//	ensurePrevote(t, voteSub, tsHash, heights, round)
//	validatePrevote(t, kbft, round, thisNode, tsHash)
//
//	// participant-0: 6 out of 8 nodes prevote on tsHash, excluding the current node
//	// participant-1:  6 out of 8 nodes prevote on tsHash
//	// random set: 22 out of 32 nodes prevote on tsHash
//	sendAndEnsurePreVote(
//		t,
//		kbft,
//		round,
//		tsHash,
//		heights,
//		voteSub,
//		round,
//		valSet.GetVaults(0, 6, thisNode.KramaID())...,
//	)
//	sendAndEnsurePreVote(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(2, 6)...)
//	sendAndEnsurePreVote(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(4, 22)...)
//
//	ensurePolka(t, polkaSub, heights, round)
//	ensurePrecommit(t, voteSub, tsHash, heights, round)
//	validatePrecommit(t, kbft, 0, 0, thisNode, tsHash, tsHash)
//
//	// participant-0: 6 out of 8 nodes precommit on tsHash, excluding the current node
//	// participant-1:  6 out of 8 nodes precommit on tsHash
//	// random set: 22 out of 32 nodes precommit on tsHash
//	sendAndEnsurePrecommit(t,
//		kbft,
//		round,
//		tsHash,
//		heights,
//		voteSub,
//		round,
//		valSet.GetVaults(0, 6, thisNode.KramaID())...,
//	)
//	sendAndEnsurePrecommit(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(2, 6)...)
//	sendAndEnsurePrecommit(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(4, 22)...)
//
//	ensureNoError(t, kbftErr)
//	require.Equal(t, c.tsCount, len(heights))
//}
//
//// 1. only 2/3 -1 nodes send prevotes
//// 2. make sure this node shouldn't send precommit
//// 3. kbft timeout occurs
// func TestFullRound_WithLessThan23rdPrevotes(t *testing.T) {
//	t.Parallel()
//
//	out := make(chan ktypes.ConsensusMessage)
//	in := make(chan ktypes.ConsensusMessage)
//	c := defaultChain()
//	round := int32(0)
//
//	icsNodes, valSet := createICSNodes(t, 2, 4, 32, 0)
//
//	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
//	heights := map[identifiers.Address]uint64{
//		ixs[0].Sender():                2,
//		ixs[0].Transaction(0).Target(): 3,
//	}
//	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs)
//	voteset := ktypes.NewHeightVoteSet(make([]string, 0), clusterInfo.NewHeights(), clusterInfo, hclog.NewNullLogger())
//	ctx := context.Background()
//	thisNode := valSet[0][0]
//
//	ixHash, err := clusterInfo.Ixns().Hash()
//	require.NoError(t, err)
//
//	kbft := NewKBFTService(
//		ctx,
//		TestBFTimeout,
//		thisNode.KramaID(),
//		createTestConsensusConfig(),
//		out,
//		in,
//		thisNode,
//		clusterInfo,
//		voteset,
//		c.finalizeTesseractGrid,
//		withDefaultEventMux(),
//		WithLogger(hclog.NewNullLogger()),
//		WithWal(NullWal{}),
//		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
//	)
//
//	newRoundSub := kbft.mux.Subscribe(eventNewView{})
//	proposalSub := kbft.mux.Subscribe(eventProposal{})
//	voteSub := kbft.mux.Subscribe(eventVote{})
//
//	kbftErr := make(chan error, 1)
//
//	handleOutboundMsgChannel(kbft, ctx, out)
//
//	go startTestRound(kbft, heights, round, kbftErr)
//
//	ensureNewRound(t, newRoundSub, heights, round)
//	ensureProposal(t, proposalSub, heights, round)
//
//	require.NotNil(t, kbft.ProposalTS)
//	tsHash := kbft.ProposalTS.Hash()
//
//	ensurePrevote(t, voteSub, tsHash, heights, round)
//	validatePrevote(t, kbft, round, thisNode, tsHash)
//
//	// participant-0: 4 out of 8 nodes prevote on tsHash, excluding the current node
//	// participant-1:  6 out of 8 nodes prevote on tsHash
//	// random set: 22 out of 32 nodes prevote on tsHash
//	sendAndEnsurePreVote(
//		t,
//		kbft,
//		round,
//		tsHash,
//		heights,
//		voteSub,
//		round,
//		valSet.GetVaults(0, 4, thisNode.KramaID())...,
//	)
//	sendAndEnsurePreVote(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(2, 6)...)
//	sendAndEnsurePreVote(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(4, 22)...)
//
//	ensureNoPrecommitReceived(t, voteSub)
//
//	// kbft context gets timed out
//	ensureError(t, kbftErr, "context deadline exceeded")
//	require.Equal(t, c.tsCount, -1)
//}
//
//// 1. only 2/3 -1 nodes send precommit
//// 2. make sure this node shouldn't add tesseract to chains
//// 3. kbft timeout occurs
// func TestFullRound_WithLessThan23rdPrecommit(t *testing.T) {
//	t.Parallel()
//
//	out := make(chan ktypes.ConsensusMessage)
//	in := make(chan ktypes.ConsensusMessage)
//	c := defaultChain()
//	round := int32(0)
//
//	icsNodes, valSet := createICSNodes(t, 2, 4, 32, 0)
//
//	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
//	heights := map[identifiers.Address]uint64{
//		ixs[0].Sender():                2,
//		ixs[0].Transaction(0).Target(): 3,
//	}
//	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs)
//	voteset := ktypes.NewHeightVoteSet(make([]string, 0), clusterInfo.NewHeights(), clusterInfo, hclog.NewNullLogger())
//	ctx := context.Background()
//	thisNode := valSet[0][0]
//
//	ixHash, err := clusterInfo.Ixns().Hash()
//	require.NoError(t, err)
//
//	kbft := NewKBFTService(
//		ctx,
//		TestBFTimeout,
//		thisNode.KramaID(),
//		createTestConsensusConfig(),
//		out,
//		in,
//		thisNode,
//		clusterInfo,
//		voteset,
//		c.finalizeTesseractGrid,
//		withDefaultEventMux(),
//		WithLogger(hclog.NewNullLogger()),
//		WithWal(NullWal{}),
//		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
//	)
//
//	newRoundSub := kbft.mux.Subscribe(eventNewView{})
//	proposalSub := kbft.mux.Subscribe(eventProposal{})
//	voteSub := kbft.mux.Subscribe(eventVote{})
//
//	kbftErr := make(chan error, 1)
//
//	handleOutboundMsgChannel(kbft, ctx, out)
//
//	go startTestRound(kbft, heights, round, kbftErr)
//
//	ensureNewRound(t, newRoundSub, heights, round)
//	ensureProposal(t, proposalSub, heights, round)
//
//	require.NotNil(t, kbft.ProposalTS)
//	tsHash := kbft.ProposalTS.Hash()
//
//	ensurePrevote(t, voteSub, tsHash, heights, round)
//	validatePrevote(t, kbft, round, thisNode, tsHash)
//
//	// participant-0: 6 out of 8 nodes prevote on tsHash, excluding the current node
//	// participant-1:  6 out of 8 nodes prevote on tsHash
//	// random set: 22 out of 32 nodes prevote on tsHash
//	sendAndEnsurePreVote(
//		t,
//		kbft,
//		round,
//		tsHash,
//		heights,
//		voteSub,
//		round,
//		valSet.GetVaults(0, 6, thisNode.KramaID())...,
//	)
//	sendAndEnsurePreVote(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(2, 6)...)
//	sendAndEnsurePreVote(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(4, 22)...)
//
//	ensurePrecommit(t, voteSub, tsHash, heights, round)
//	validatePrecommit(t, kbft, 0, 0, thisNode, tsHash, tsHash)
//
//	// participant-0: 4 out of 8 nodes precommit on tsHash, excluding the current node
//	// participant-1:  6 out of 8 nodes precommit on tsHash
//	// random set: 22 out of 32 nodes precommit on tsHash
//	sendAndEnsurePrecommit(t,
//		kbft,
//		round,
//		tsHash,
//		heights,
//		voteSub,
//		round,
//		valSet.GetVaults(0, 4, thisNode.KramaID())...,
//	)
//	sendAndEnsurePrecommit(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(2, 6)...)
//	sendAndEnsurePrecommit(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(4, 22)...)
//
//	// kbft context gets timed out
//	ensureError(t, kbftErr, "context deadline exceeded")
//	require.Equal(t, c.tsCount, -1)
//}
//
//// 1. if any 2/3 rd nodes prevote then should enter prevote wait
//// 2. this node should precommit nil
// func TestFullRound_WithAny23rdPrevote_Any23rdPrecommit(t *testing.T) {
//	t.Parallel()
//
//	out := make(chan ktypes.ConsensusMessage)
//	in := make(chan ktypes.ConsensusMessage)
//	c := defaultChain()
//	round := int32(0)
//
//	icsNodes, valSet := createICSNodes(t, 2, 4, 32, 0)
//
//	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
//	heights := map[identifiers.Address]uint64{
//		ixs[0].Sender():                2,
//		ixs[0].Transaction(0).Target(): 3,
//	}
//	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs)
//	voteset := ktypes.NewHeightVoteSet(make([]string, 0), clusterInfo.NewHeights(), clusterInfo, hclog.NewNullLogger())
//	ctx := context.Background()
//	thisNode := valSet[0][0]
//
//	ixHash, err := clusterInfo.Ixns().Hash()
//	require.NoError(t, err)
//
//	kbft := NewKBFTService(
//		ctx,
//		TestBFTimeout,
//		thisNode.KramaID(),
//		createTestConsensusConfig(),
//		out,
//		in,
//		thisNode,
//		clusterInfo,
//		voteset,
//		c.finalizeTesseractGrid,
//		withDefaultEventMux(),
//		WithLogger(hclog.NewNullLogger()),
//		WithWal(NullWal{}),
//		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
//	)
//
//	newRoundSub := kbft.mux.Subscribe(eventNewView{})
//	proposalSub := kbft.mux.Subscribe(eventProposal{})
//	voteSub := kbft.mux.Subscribe(eventVote{})
//	prevoteTimeoutSub := kbft.mux.Subscribe(eventTimeoutPrevote{})
//	precommitTimeoutSub := kbft.mux.Subscribe(eventTimeoutPrecommit{})
//
//	kbftErr := make(chan error, 1)
//
//	handleOutboundMsgChannel(kbft, ctx, out)
//
//	go startTestRound(kbft, heights, round, kbftErr)
//
//	ensureNewRound(t, newRoundSub, heights, round)
//	ensureProposal(t, proposalSub, heights, round)
//
//	require.NotNil(t, kbft.ProposalTS)
//	tsHash := kbft.ProposalTS.Hash()
//
//	ensurePrevote(t, voteSub, tsHash, heights, round)
//	validatePrevote(t, kbft, round, thisNode, tsHash)
//
//	randomHash := tests.RandomHash(t)
//	// participant-0: 5 out of 8 nodes prevote on randomHash, excluding the current node
//	// participant-1:  6 out of 8 nodes prevote on nilHash
//	// random set: 22 out of 32 nodes prevote on nilHash
//	sendAndEnsurePreVote(
//		t,
//		kbft,
//		round,
//		randomHash,
//		heights,
//		voteSub,
//		round,
//		valSet.GetVaults(0, 5, thisNode.KramaID())...,
//	)
//	sendAndEnsurePreVote(t, kbft, round, common.NilHash, heights, voteSub, round, valSet.GetVaults(2, 6)...)
//	sendAndEnsurePreVote(t, kbft, round, common.NilHash, heights, voteSub, round, valSet.GetVaults(4, 22)...)
//
//	ensurePrevoteTimeout(t, prevoteTimeoutSub, heights, round, kbft.config.PrevoteWaitDuration(0).Nanoseconds())
//	ensurePrecommit(t, voteSub, common.NilHash, heights, round)
//	validatePrecommit(t, kbft, 0, 0, thisNode, common.NilHash, common.NilHash)
//
//	// participant-0: 5 out of 8 nodes precommit on randomHash, excluding the current node
//	// participant-1:  6 out of 8 nodes precommit on nilHash
//	// random set: 22 out of 32 nodes precommit on nilHash
//	sendAndEnsurePrecommit(t,
//		kbft,
//		round,
//		randomHash,
//		heights,
//		voteSub,
//		round,
//		valSet.GetVaults(0, 5, thisNode.KramaID())...,
//	)
//	sendAndEnsurePrecommit(t, kbft, round, common.NilHash, heights, voteSub, round, valSet.GetVaults(2, 6)...)
//	sendAndEnsurePrecommit(t, kbft, round, common.NilHash, heights, voteSub, round, valSet.GetVaults(4, 22)...)
//	ensurePrecommitTimeout(t, precommitTimeoutSub, heights, round,
//		kbft.config.PrecommitWaitDuration(0).Nanoseconds())
//
//	ensureError(t, kbftErr, "context deadline exceeded")
//	require.Equal(t, c.tsCount, -1)
//}
//
//// 1. if 2/3 rd nodes precommit nil then tesseract shouldn't get added
//// 2. should enter precommit wait timeout
// func TestFullRound_With23rdNilPrecommit(t *testing.T) {
//	t.Parallel()
//
//	out := make(chan ktypes.ConsensusMessage)
//	in := make(chan ktypes.ConsensusMessage)
//	c := defaultChain()
//	round := int32(0)
//
//	icsNodes, valSet := createICSNodes(t, 2, 4, 32, 0)
//
//	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
//	heights := map[identifiers.Address]uint64{
//		ixs[0].Sender():                2,
//		ixs[0].Transaction(0).Target(): 3,
//	}
//	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs)
//	voteset := ktypes.NewHeightVoteSet(make([]string, 0), clusterInfo.NewHeights(), clusterInfo, hclog.NewNullLogger())
//	ctx := context.Background()
//	thisNode := valSet[0][0]
//
//	ixHash, err := clusterInfo.Ixns().Hash()
//	require.NoError(t, err)
//
//	kbft := NewKBFTService(
//		ctx,
//		TestBFTimeout,
//		thisNode.KramaID(),
//		createTestConsensusConfig(),
//		out,
//		in,
//		thisNode,
//		clusterInfo,
//		voteset,
//		c.finalizeTesseractGrid,
//		withDefaultEventMux(),
//		WithLogger(hclog.NewNullLogger()),
//		WithWal(NullWal{}),
//		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
//	)
//
//	newRoundSub := kbft.mux.Subscribe(eventNewView{})
//	proposalSub := kbft.mux.Subscribe(eventProposal{})
//	voteSub := kbft.mux.Subscribe(eventVote{})
//	precommitTimeoutSub := kbft.mux.Subscribe(eventTimeoutPrecommit{})
//
//	kbftErr := make(chan error, 1)
//
//	handleOutboundMsgChannel(kbft, ctx, out)
//
//	go startTestRound(kbft, heights, round, kbftErr)
//
//	ensureNewRound(t, newRoundSub, heights, round)
//	ensureProposal(t, proposalSub, heights, round)
//
//	require.NotNil(t, kbft.ProposalTS)
//	tsHash := kbft.ProposalTS.Hash()
//
//	ensurePrevote(t, voteSub, tsHash, heights, round)
//	validatePrevote(t, kbft, round, thisNode, tsHash)
//
//	// participant-0: 5 out of 8 nodes prevote on tsHash, excluding the current node
//	// participant-1:  6 out of 8 nodes prevote on tsHash
//	// random set: 22 out of 32 nodes prevote on tsHash
//	sendAndEnsurePreVote(
//		t,
//		kbft,
//		round,
//		tsHash,
//		heights,
//		voteSub,
//		round,
//		valSet.GetVaults(0, 5, thisNode.KramaID())...,
//	)
//	sendAndEnsurePreVote(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(2, 6)...)
//	sendAndEnsurePreVote(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(4, 22)...)
//
//	ensurePrecommit(t, voteSub, tsHash, heights, round)
//	validatePrecommit(t, kbft, 0, 0, thisNode, tsHash, tsHash)
//
//	// participant-0: 5 out of 8 nodes precommit on nilHash, excluding the current node
//	// participant-1:  6 out of 8 nodes precommit on nilHash
//	// random set: 22 out of 32 nodes precommit on nilHash
//	sendAndEnsurePrecommit(t,
//		kbft,
//		round,
//		common.NilHash,
//		heights,
//		voteSub,
//		round,
//		valSet.GetVaults(0, 5, thisNode.KramaID())...,
//	)
//	sendAndEnsurePrecommit(t, kbft, round, common.NilHash, heights, voteSub, round, valSet.GetVaults(2, 6)...)
//	sendAndEnsurePrecommit(t, kbft, round, common.NilHash, heights, voteSub, round, valSet.GetVaults(4, 22)...)
//
//	ensurePrecommitTimeout(
//		t,
//		precommitTimeoutSub,
//		heights,
//		round,
//		kbft.config.PrecommitWaitDuration(0).Nanoseconds(),
//	)
//
//	ensureError(t, kbftErr, "context deadline exceeded")
//	require.Equal(t, c.tsCount, -1)
//}
//
//// sign random tsHash two times by same validator
//// check if signatures match
// func TestSignVote(t *testing.T) {
//	t.Parallel()
//
//	out := make(chan ktypes.ConsensusMessage)
//	in := make(chan ktypes.ConsensusMessage)
//	c := defaultChain()
//
//	icsNodes, valSet := createICSNodes(t, 1, 1,
//		0, 0)
//
//	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
//	heights := map[identifiers.Address]uint64{
//		ixs[0].Sender():                2,
//		ixs[0].Transaction(0).Target(): 3,
//	}
//
//	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs)
//	voteset := ktypes.NewHeightVoteSet(make([]string, 0), clusterInfo.NewHeights(), clusterInfo, hclog.NewNullLogger())
//	ctx := context.Background()
//	thisNode := valSet[0][0]
//
//	ixHash, err := clusterInfo.Ixns().Hash()
//	require.NoError(t, err)
//
//	kbft := NewKBFTService(
//		ctx,
//		TestBFTimeout,
//		thisNode.KramaID(),
//		createTestConsensusConfig(),
//		out,
//		in,
//		thisNode,
//		clusterInfo,
//		voteset,
//		c.finalizeTesseractGrid,
//		withDefaultEventMux(),
//		WithLogger(hclog.NewNullLogger()),
//		WithWal(NullWal{}),
//		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
//	)
//
//	randomHash := tests.RandomHash(t)
//	firstVote, err := kbft.signVote(common.PRECOMMIT, randomHash)
//	require.NoError(t, err)
//
//	secondVote, err := kbft.signVote(common.PRECOMMIT, randomHash)
//	require.NoError(t, err)
//
//	require.Equal(t, firstVote.Signature, secondVote.Signature)
//}
//
//// 1. validator in sender behaviour set prevotes on both tesseract hash
//// and nil tesseract hash which is conflict of vote for that view
//// 2. check if evidence added
// func TestConflictPrevote(t *testing.T) {
//	t.Parallel()
//
//	out := make(chan ktypes.ConsensusMessage)
//	in := make(chan ktypes.ConsensusMessage)
//	c := defaultChain()
//	round := int32(0)
//
//	icsNodes, valSet := createICSNodes(t, 1, 2,
//		0, 0)
//
//	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
//	heights := map[identifiers.Address]uint64{
//		ixs[0].Sender():                2,
//		ixs[0].Transaction(0).Target(): 3,
//	}
//
//	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs)
//	voteset := ktypes.NewHeightVoteSet(make([]string, 0), clusterInfo.NewHeights(), clusterInfo, hclog.NewNullLogger())
//	ctx := context.Background()
//	thisNode := valSet[0][0]
//
//	ixHash, err := clusterInfo.Ixns().Hash()
//	require.NoError(t, err)
//
//	kbft := NewKBFTService(
//		ctx,
//		TestBFTimeout,
//		thisNode.KramaID(),
//		createTestConsensusConfig(),
//		out,
//		in,
//		thisNode,
//		clusterInfo,
//		voteset,
//		c.finalizeTesseractGrid,
//		withDefaultEventMux(),
//		WithLogger(hclog.NewNullLogger()),
//		WithWal(NullWal{}),
//		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
//	)
//
//	newRoundSub := kbft.mux.Subscribe(eventNewView{})
//	proposalSub := kbft.mux.Subscribe(eventProposal{})
//	voteSub := kbft.mux.Subscribe(eventVote{})
//
//	kbftErr := make(chan error, 1)
//
//	handleOutboundMsgChannel(kbft, ctx, out)
//
//	go startTestRound(kbft, heights, round, kbftErr)
//
//	ensureNewRound(t, newRoundSub, heights, round)
//	ensureProposal(t, proposalSub, heights, round)
//
//	require.NotNil(t, kbft.ProposalTS)
//	tsHash := kbft.ProposalTS.Hash()
//
//	ensurePrevote(t, voteSub, tsHash, heights, round)
//	validatePrevote(t, kbft, round, thisNode, tsHash)
//
//	sendAndEnsurePreVote(t, kbft, round, tsHash, heights, voteSub, round, valSet[0][1])
//	signAddVotesSynchronously(t, kbft, round, common.PREVOTE, common.NilHash, heights,
//		valSet[0][1]) // conflict vote
//
//	time.Sleep(5 * time.Millisecond) // wait for 5ms so that conflict prevote gets processed
//
//	expectedConflictVote := signVote(t, kbft, round, common.PREVOTE, common.NilHash, heights,
//		valSet[0][1])
//
//	require.Equal(t, expectedConflictVote, kbft.evidence.Votes[len(kbft.evidence.Votes)-1])
//}
//
//// 1. validator in sender behaviour set precommits on both ts hash and nil ts hash
//// which is conflict of vote for that view
//// 2. check if evidence added
// func TestConflictPrecommit(t *testing.T) {
//	t.Parallel()
//
//	out := make(chan ktypes.ConsensusMessage)
//	in := make(chan ktypes.ConsensusMessage)
//	c := defaultChain()
//	round := int32(0)
//
//	icsNodes, valSet := createICSNodes(t, 2,
//		4, 32, 0)
//
//	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
//	heights := map[identifiers.Address]uint64{
//		ixs[0].Sender():                2,
//		ixs[0].Transaction(0).Target(): 3,
//	}
//
//	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs)
//	voteset := ktypes.NewHeightVoteSet(make([]string, 0), clusterInfo.NewHeights(), clusterInfo, hclog.NewNullLogger())
//	ctx := context.Background()
//	thisNode := valSet[0][0]
//
//	ixHash, err := clusterInfo.Ixns().Hash()
//	require.NoError(t, err)
//
//	kbft := NewKBFTService(
//		ctx,
//		TestBFTimeout,
//		thisNode.KramaID(),
//		createTestConsensusConfig(),
//		out,
//		in,
//		thisNode,
//		clusterInfo,
//		voteset,
//		c.finalizeTesseractGrid,
//		withDefaultEventMux(),
//		WithLogger(hclog.NewNullLogger()),
//		WithWal(NullWal{}),
//		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
//	)
//
//	newRoundSub := kbft.mux.Subscribe(eventNewView{})
//	proposalSub := kbft.mux.Subscribe(eventProposal{})
//	voteSub := kbft.mux.Subscribe(eventVote{})
//
//	kbftErr := make(chan error, 1)
//
//	handleOutboundMsgChannel(kbft, ctx, out)
//
//	go startTestRound(kbft, heights, round, kbftErr)
//
//	ensureNewRound(t, newRoundSub, heights, round)
//	ensureProposal(t, proposalSub, heights, round)
//
//	require.NotNil(t, kbft.ProposalTS)
//	tsHash := kbft.ProposalTS.Hash()
//
//	ensurePrevote(t, voteSub, tsHash, heights, round)
//	validatePrevote(t, kbft, round, thisNode, tsHash)
//	// participant-0: 5 out of 8 nodes prevote on tsHash, excluding the current node
//	// participant-1: 6 out of 8 nodes prevote on tsHash
//	// random set: 22 out of 32 nodes prevote on tsHash
//	sendAndEnsurePreVote(
//		t,
//		kbft,
//		round,
//		tsHash,
//		heights,
//		voteSub,
//		round,
//		valSet.GetVaults(0, 5, thisNode.KramaID())...,
//	)
//	sendAndEnsurePreVote(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(2, 6)...)
//	sendAndEnsurePreVote(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(4, 22)...)
//
//	ensurePrecommit(t, voteSub, tsHash, heights, round)
//	validatePrecommit(t, kbft, 0, 0, thisNode, tsHash, tsHash)
//
//	sendAndEnsurePrecommit(t, kbft, round, tsHash, heights, voteSub, round, valSet[0][1])
//	signAddVotesSynchronously(t, kbft, round, common.PRECOMMIT, common.NilHash, heights,
//		valSet[0][1]) // conflict vote
//
//	time.Sleep(5 * time.Millisecond) // wait for 5ms so that conflict precommit gets processed
//
//	expectedConflictVote := signVote(t,
//		kbft,
//		round,
//		common.PRECOMMIT,
//		common.NilHash,
//		heights,
//		valSet[0][1],
//	)
//
//	require.Equal(t, expectedConflictVote, kbft.evidence.Votes[len(kbft.evidence.Votes)-1])
//}
//
//// we make sure only 2/3 -1 prevotes received
//// then prevote from random validator shouldn't trigger precommit
// func TestRandomValidatorPreVote(t *testing.T) {
//	t.Parallel()
//
//	out := make(chan ktypes.ConsensusMessage)
//	in := make(chan ktypes.ConsensusMessage)
//	c := defaultChain()
//	round := int32(0)
//
//	icsNodes, valSet := createICSNodes(t, 2,
//		4, 32, 0)
//
//	_, randomVal := createTestNodeSet(t, 1)
//
//	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
//	heights := map[identifiers.Address]uint64{
//		ixs[0].Sender():                2,
//		ixs[0].Transaction(0).Target(): 3,
//	}
//	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs)
//	voteset := ktypes.NewHeightVoteSet(make([]string, 0), clusterInfo.NewHeights(), clusterInfo, hclog.NewNullLogger())
//	ctx := context.Background()
//	thisNode := valSet[0][0]
//
//	ixHash, err := clusterInfo.Ixns().Hash()
//	require.NoError(t, err)
//
//	kbft := NewKBFTService(
//		ctx,
//		TestBFTimeout,
//		thisNode.KramaID(),
//		createTestConsensusConfig(),
//		out,
//		in,
//		thisNode,
//		clusterInfo,
//		voteset,
//		c.finalizeTesseractGrid,
//		withDefaultEventMux(),
//		WithLogger(hclog.NewNullLogger()),
//		WithWal(NullWal{}),
//		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
//	)
//
//	newRoundSub := kbft.mux.Subscribe(eventNewView{})
//	proposalSub := kbft.mux.Subscribe(eventProposal{})
//	voteSub := kbft.mux.Subscribe(eventVote{})
//
//	kbftErr := make(chan error, 1)
//
//	handleOutboundMsgChannel(kbft, ctx, out)
//
//	go startTestRound(kbft, heights, round, kbftErr)
//
//	ensureNewRound(t, newRoundSub, heights, round)
//	ensureProposal(t, proposalSub, heights, round)
//
//	require.NotNil(t, kbft.ProposalTS)
//	tsHash := kbft.ProposalTS.Hash()
//
//	ensurePrevote(t, voteSub, tsHash, heights, round)
//	validatePrevote(t, kbft, round, thisNode, tsHash)
//
//	// participant-0: 5 out of 8 nodes prevote on tsHash, excluding the current node
//	// participant-1: 6 out of 8 nodes prevote on tsHash
//	// random set: 21 out of 32 nodes prevote on tsHash
//	sendAndEnsurePreVote(
//		t,
//		kbft,
//		round,
//		tsHash,
//		heights,
//		voteSub,
//		round,
//		valSet.GetVaults(0, 5, thisNode.KramaID())...,
//	)
//	sendAndEnsurePreVote(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(2, 6)...)
//	sendAndEnsurePreVote(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(4, 21)...)
//
//	signAddVotes(t, kbft, round, common.PREVOTE, tsHash, heights, randomVal[0])
//
//	ensureNoPrecommitReceived(t, voteSub)
//}
//
//// we make sure only 2/3 -1 precommit received
//// then precommit from random validator shouldn't add tesseract to chain
// func TestRandomValidatorPrecommit(t *testing.T) {
//	t.Parallel()
//
//	out := make(chan ktypes.ConsensusMessage)
//	in := make(chan ktypes.ConsensusMessage)
//	c := defaultChain()
//	round := int32(0)
//
//	icsNodes, valSet := createICSNodes(t, 2, 4, 32, 0)
//
//	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
//	heights := map[identifiers.Address]uint64{
//		ixs[0].Sender():                2,
//		ixs[0].Transaction(0).Target(): 3,
//	}
//	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs)
//	voteset := ktypes.NewHeightVoteSet(make([]string, 0), clusterInfo.NewHeights(), clusterInfo, hclog.NewNullLogger())
//	ctx := context.Background()
//	thisNode := valSet[0][0]
//
//	ixHash, err := clusterInfo.Ixns().Hash()
//	require.NoError(t, err)
//
//	kbft := NewKBFTService(
//		ctx,
//		TestBFTimeout,
//		thisNode.KramaID(),
//		createTestConsensusConfig(),
//		out,
//		in,
//		thisNode,
//		clusterInfo,
//		voteset,
//		c.finalizeTesseractGrid,
//		withDefaultEventMux(),
//		WithLogger(hclog.NewNullLogger()),
//		WithWal(NullWal{}),
//		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
//	)
//
//	_, randomVal := createTestNodeSet(t, 1)
//
//	newRoundSub := kbft.mux.Subscribe(eventNewView{})
//	proposalSub := kbft.mux.Subscribe(eventProposal{})
//	voteSub := kbft.mux.Subscribe(eventVote{})
//
//	kbftErr := make(chan error, 1)
//
//	handleOutboundMsgChannel(kbft, ctx, out)
//
//	go startTestRound(kbft, heights, round, kbftErr)
//
//	ensureNewRound(t, newRoundSub, heights, round)
//	ensureProposal(t, proposalSub, heights, round)
//
//	require.NotNil(t, kbft.ProposalTS)
//	tsHash := kbft.ProposalTS.Hash()
//
//	ensurePrevote(t, voteSub, tsHash, heights, round)
//	validatePrevote(t, kbft, round, thisNode, tsHash)
//
//	// participant-0: 6 out of 8 nodes prevote on tsHash, excluding the current node
//	// participant-1: 6 out of 8 nodes prevote on tsHash
//	// random set: 22 out of 32 nodes prevote on tsHash
//	sendAndEnsurePreVote(
//		t,
//		kbft,
//		round,
//		tsHash,
//		heights,
//		voteSub,
//		round,
//		valSet.GetVaults(0, 6, thisNode.KramaID())...,
//	)
//	sendAndEnsurePreVote(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(2, 6)...)
//	sendAndEnsurePreVote(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(4, 22)...)
//
//	ensurePrecommit(t, voteSub, tsHash, heights, round)
//	validatePrecommit(t, kbft, 0, 0, thisNode, tsHash, tsHash)
//
//	// participant-0: 6 out of 8 nodes precommit on tsHash, excluding the current node
//	// participant-1: 6 out of 8 nodes precommit on tsHash
//	// random set: 21 out of 32 nodes precommit on tsHash
//	sendAndEnsurePrecommit(t,
//		kbft,
//		round,
//		tsHash,
//		heights,
//		voteSub,
//		round,
//		valSet.GetVaults(0, 6, thisNode.KramaID())...,
//	)
//	sendAndEnsurePrecommit(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(2, 6)...)
//	sendAndEnsurePrecommit(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(4, 21)...)
//
//	signAddVotes(t, kbft, round, common.PRECOMMIT, tsHash, heights, randomVal[0])
//
//	// kbft context gets timed out
//	ensureError(t, kbftErr, "context deadline exceeded")
//	require.Equal(t, c.tsCount, -1)
//}
//
//// 1. enter prevoteWait after any 2/3 rd maj
//// 2. receives 2/3 rd prevotes during wait
//// 3. This node should vote precommit
// func TestReceivePrevoteDuringPrevoteWait(t *testing.T) {
//	t.Parallel()
//
//	out := make(chan ktypes.ConsensusMessage)
//	in := make(chan ktypes.ConsensusMessage)
//	c := defaultChain()
//	round := int32(0)
//
//	icsNodes, valSet := createICSNodes(t, 2,
//		4, 32, 0)
//
//	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
//	heights := map[identifiers.Address]uint64{
//		ixs[0].Sender():                2,
//		ixs[0].Transaction(0).Target(): 3,
//	}
//	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs)
//	voteset := ktypes.NewHeightVoteSet(make([]string, 0), clusterInfo.NewHeights(), clusterInfo, hclog.NewNullLogger())
//	ctx := context.Background()
//	thisNode := valSet[0][0]
//
//	ixHash, err := clusterInfo.Ixns().Hash()
//	require.NoError(t, err)
//
//	kbft := NewKBFTService(
//		ctx,
//		TestBFTimeout,
//		thisNode.KramaID(),
//		createTestConsensusConfig(),
//		out,
//		in,
//		thisNode,
//		clusterInfo,
//		voteset,
//		c.finalizeTesseractGrid,
//		withDefaultEventMux(),
//		WithLogger(hclog.NewNullLogger()),
//		WithWal(NullWal{}),
//		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
//	)
//
//	newRoundSub := kbft.mux.Subscribe(eventNewView{})
//	proposalSub := kbft.mux.Subscribe(eventProposal{})
//	voteSub := kbft.mux.Subscribe(eventVote{})
//
//	kbftErr := make(chan error, 1)
//
//	handleOutboundMsgChannel(kbft, ctx, out)
//
//	go startTestRound(kbft, heights, round, kbftErr)
//
//	ensureNewRound(t, newRoundSub, heights, round)
//	ensureProposal(t, proposalSub, heights, round)
//
//	require.NotNil(t, kbft.ProposalTS)
//	tsHash := kbft.ProposalTS.Hash()
//	// participant-0: 6 out of 8 nodes prevote on tsHash, excluding the current node
//	// participant-1: 6 out of 8 nodes prevote on tsHash
//	// random set: 21 out of 32 nodes prevote on tsHash
//	ensurePrevote(t, voteSub, tsHash, heights, round)
//	validatePrevote(t, kbft, round, thisNode, tsHash)
//	sendAndEnsurePreVote(
//		t,
//		kbft,
//		round,
//		tsHash,
//		heights,
//		voteSub,
//		round,
//		valSet.GetVaults(0, 6, thisNode.KramaID())...,
//	)
//	sendAndEnsurePreVote(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(2, 6)...)
//	sendAndEnsurePreVote(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(4, 21)...)
//
//	// 2/3 rd any majority is reached with the following vote
//	sendAndEnsurePreVote(t, kbft, round, common.NilHash, heights, voteSub, round,
//	valSet[icsNodes.StochasticSetPosition()][21])
//	time.Sleep(3 * time.Millisecond) // wait for 3 ms as prevote timeout is 10 ms
//
//	// send the 2/3 rd prevote
//	sendAndEnsurePreVote(t, kbft, round, tsHash, heights, voteSub, round, valSet[icsNodes.StochasticSetPosition()][22])
//
//	ensurePrecommit(t, voteSub, tsHash, heights, round)
//	validatePrecommit(t, kbft, 0, 0, thisNode, tsHash, tsHash)
//}
//
//// 1. enter precommitWait after any 2/3 rd maj
//// 2. receives 2/3 rd precommits during wait
//// 3. This node should add tesseract to chain
// func TestReceivePrecommitDuringPrecommmitWait(t *testing.T) {
//	t.Parallel()
//
//	out := make(chan ktypes.ConsensusMessage)
//	in := make(chan ktypes.ConsensusMessage)
//	c := defaultChain()
//	round := int32(0)
//
//	icsNodes, valSet := createICSNodes(t, 2,
//		4, 32, 0)
//
//	ixs := createIxs(t, tests.RandomAddress(t), tests.RandomAddress(t))
//	heights := map[identifiers.Address]uint64{
//		ixs[0].Sender():                2,
//		ixs[0].Transaction(0).Target(): 3,
//	}
//	clusterInfo := createTestClusterInfo(t, icsNodes, heights, ixs)
//	voteset := ktypes.NewHeightVoteSet(make([]string, 0), clusterInfo.NewHeights(), clusterInfo, hclog.NewNullLogger())
//	ctx := context.Background()
//	thisNode := valSet[0][0]
//
//	ixHash, err := clusterInfo.Ixns().Hash()
//	require.NoError(t, err)
//
//	kbft := NewKBFTService(
//		ctx,
//		TestBFTimeout,
//		thisNode.KramaID(),
//		createTestConsensusConfig(),
//		out,
//		in,
//		thisNode,
//		clusterInfo,
//		voteset,
//		c.finalizeTesseractGrid,
//		withDefaultEventMux(),
//		WithLogger(hclog.NewNullLogger()),
//		WithWal(NullWal{}),
//		WithEvidence(NewEvidence(ixHash, clusterInfo.Operator, clusterInfo.Size())),
//	)
//
//	newRoundSub := kbft.mux.Subscribe(eventNewView{})
//	proposalSub := kbft.mux.Subscribe(eventProposal{})
//	voteSub := kbft.mux.Subscribe(eventVote{})
//
//	kbftErr := make(chan error, 1)
//
//	handleOutboundMsgChannel(kbft, ctx, out)
//
//	go startTestRound(kbft, heights, round, kbftErr)
//
//	ensureNewRound(t, newRoundSub, heights, round)
//	ensureProposal(t, proposalSub, heights, round)
//
//	require.NotNil(t, kbft.ProposalTS)
//	tsHash := kbft.ProposalTS.Hash()
//
//	ensurePrevote(t, voteSub, tsHash, heights, round)
//	validatePrevote(t, kbft, round, thisNode, tsHash)
//	// participant-0: 5 out of 8 nodes prevote on tsHash, excluding the current node
//	// participant-1:  6 out of 8 nodes prevote on tsHash
//	// random set: 22 out of 32 nodes prevote on tsHash
//	sendAndEnsurePreVote(
//		t,
//		kbft,
//		round,
//		tsHash,
//		heights,
//		voteSub,
//		round,
//		valSet.GetVaults(0, 5, thisNode.KramaID())...,
//	)
//	sendAndEnsurePreVote(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(2, 6)...)
//	sendAndEnsurePreVote(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(4, 22)...)
//
//	ensurePrecommit(t, voteSub, tsHash, heights, round)
//	validatePrecommit(t, kbft, 0, 0, thisNode, tsHash, tsHash)
//
//	// participant-0: 5 out of 8 nodes precommit on tsHash, excluding the current node
//	// participant-1:  6 out of 8 nodes precommit on tsHash
//	// random set: 21 out of 32 nodes precommit on tsHash
//	sendAndEnsurePrecommit(t,
//		kbft,
//		round,
//		tsHash,
//		heights,
//		voteSub,
//		round,
//		valSet.GetVaults(0, 5, thisNode.KramaID())...,
//	)
//	sendAndEnsurePrecommit(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(2, 6)...)
//	sendAndEnsurePrecommit(t, kbft, round, tsHash, heights, voteSub, round, valSet.GetVaults(4, 21)...)
//
//	// any 2/3 rd majority
//	sendAndEnsurePrecommit(
//		t,
//		kbft,
//		round,
//		common.NilHash,
//		heights,
//		voteSub,
//		round,
//		valSet[icsNodes.StochasticSetPosition()][21],
//	)
//
//	time.Sleep(5 * time.Millisecond) // wait for 5 ms as precommit timeout is 10 ms
//	// send the 2/3 rd precommit
//	sendAndEnsurePrecommit(t, kbft, round, tsHash, heights, voteSub, round, valSet[icsNodes.StochasticSetPosition()][22])
//
//	ensureNoError(t, kbftErr)
//	require.Equal(t, c.tsCount, len(heights))
//}
