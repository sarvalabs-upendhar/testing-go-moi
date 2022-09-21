package jug

import (
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/guna"
	"log"
	"sync"
)

type Exec struct {
	executorInstances sync.Map
	state             *guna.StateManager
}

func NewExec(state *guna.StateManager) *Exec {
	e := &Exec{
		state: state,
	}

	return e
}

func (e *Exec) ExecuteInteractions(
	clusterID ktypes.ClusterID,
	ixs []*ktypes.Interaction,
	contextDelta ktypes.ContextDelta,
) (ktypes.Receipts, error) {
	executor := NewExecutor(ixs, 1000, contextDelta, e.state)
	if err := executor.Execute(); err != nil {
		if err := executor.Revert(); err != nil {
			log.Fatal(err) // This should not happen
		}

		return nil, err
	}

	e.executorInstances.Store(clusterID, executor)

	return executor.Receipts(), nil
}

func (e *Exec) Revert(clusterID ktypes.ClusterID) error {
	rawExecutor, ok := e.executorInstances.Load(clusterID)
	if !ok {
		return nil
	}

	executor, ok := rawExecutor.(*Executor)
	if !ok {
		return ktypes.ErrInterfaceConversion
	}

	return executor.Revert()
}

func (e *Exec) CleanupExecutorInstances(id ktypes.ClusterID) {
	e.executorInstances.Delete(id)
}
