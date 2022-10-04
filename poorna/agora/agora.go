package agora

import (
	"context"
	"github.com/hashicorp/go-hclog"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/moichain/poorna"
	"gitlab.com/sarvalabs/moichain/poorna/agora/db"
	"gitlab.com/sarvalabs/moichain/poorna/agora/decision"
	"gitlab.com/sarvalabs/moichain/poorna/agora/network"
	"gitlab.com/sarvalabs/moichain/poorna/agora/session"
	"gitlab.com/sarvalabs/moichain/poorna/agora/types"
)

const (
	DefaultRequestWorkerCount  = 15
	DefaultResponseWorkerCount = 15
	DefaultLedgerWorkerCount   = 2
)

type Agora struct {
	ctx      context.Context
	engine   *decision.Engine
	ledger   *decision.Ledger
	db       *db.DataStore
	network  *network.AgoraNetwork
	im       *session.InterestManager
	sm       *session.SessionManager
	notifier types.PubSub
}

func NewAgora(
	ctx context.Context,
	logger hclog.Logger,
	store db.PersistenceManager,
	server *poorna.Server,
	metrics *Metrics,
) (*Agora, error) {
	interestManager := session.NewInterestManager()

	dataStore := db.NewDataStore(ctx, logger, store)

	ledger, err := decision.NewLedger(ctx, logger, DefaultLedgerWorkerCount, dataStore)
	if err != nil {
		return nil, err
	}

	notifier := types.NewNotifier()

	agoraNetwork := network.NewAgoraNetwork(ctx, logger, server, metrics.Network)

	engine := decision.NewEngine(
		ctx,
		logger.Named("Agora"),
		DefaultRequestWorkerCount,
		DefaultResponseWorkerCount,
		dataStore,
		ledger,
		agoraNetwork,
		metrics.Engine,
	)

	sessionManager := session.NewSessionManager(logger, interestManager, notifier, engine)

	ag := &Agora{
		ctx:      ctx,
		engine:   engine,
		ledger:   ledger,
		db:       dataStore,
		network:  agoraNetwork,
		im:       interestManager,
		sm:       sessionManager,
		notifier: notifier,
	}

	return ag, nil
}

func (ag *Agora) NewSession(
	ctx context.Context,
	contextPeers []id.KramaID,
	address ktypes.Address,
	stateHash ktypes.Hash,
) (*session.Session, error) {
	return ag.sm.NewSession(ctx, address, stateHash, ag.network, contextPeers)
}

func (ag *Agora) Start() {
	go ag.db.Start()
	go ag.engine.Start()
	go ag.ledger.Start()
	go ag.network.Start(ag.sm)
}
