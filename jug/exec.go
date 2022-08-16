package jug

import (
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/guna"
	"log"
)

type Exec struct {
	executorInstances map[ktypes.ClusterID]*Executor
	state             *guna.StateManager
}

func NewExec(state *guna.StateManager) *Exec {
	e := &Exec{
		executorInstances: make(map[ktypes.ClusterID]*Executor),
		state:             state,
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

	e.executorInstances[clusterID] = executor

	return executor.Receipts(), nil
}

func (e *Exec) Revert(clusterID ktypes.ClusterID) error {
	executor, ok := e.executorInstances[clusterID]
	if ok {
		return executor.Revert()
	}

	return nil
}

func (e *Exec) CleanupExecutorInstances(id ktypes.ClusterID) {
	delete(e.executorInstances, id)
}

/*
func CommitObjects(senderObject, receiverObject *guna.StateObject) (senderHash, receiverHash []byte, err error) {
	if senderObject != nil {
		if senderHash, err = senderObject.Commit(); err != nil {
			return
		}
	}
	if receiverObject != nil {
		if receiverHash, err = receiverObject.Commit(); err != nil {
			return
		}
	}
	return
}
func UpdateContext(senderObject, receiverObject *guna.StateObject,
contextInfo map[ktypes.Address]*ktypes.ContextDelta) error {
	for k, v := range contextInfo {
		switch k {
		case senderObject.Address:
			//bsize, err := strconv.Atoi(v[0])
			//if err != nil {
			//	return err
			//}

			//if _, err := senderObject.AddBContextNodes(v.BehaviouralNodes); err != nil {
			//	return errors.Wrap(ktypes.ErrUpdatingContext, err.Error())
			//}
			//if _, err := senderObject.AddRContextNodes(v.RandomNodes); err != nil {
			//	return errors.Wrap(ktypes.ErrUpdatingContext, err.Error())
			//}

			if _, err := senderObject.UpdateContext(v.BehaviouralNodes, v.RandomNodes); err != nil {
				return errors.Wrap(ktypes.ErrUpdatingContext, err.Error())
			}
		case receiverObject.Address:

			//if _, err := receiverObject.AddBContextNodes(v.BehaviouralNodes); err != nil {
			//	return errors.Wrap(ktypes.ErrUpdatingContext, err.Error())
			//}
			//
			//if _, err := receiverObject.AddRContextNodes(v.RandomNodes); err != nil {
			//	return errors.Wrap(ktypes.ErrUpdatingContext, err.Error())
			//}

			if _, err := receiverObject.UpdateContext(v.BehaviouralNodes, v.RandomNodes); err != nil {
				return errors.Wrap(ktypes.ErrUpdatingContext, err.Error())
			}

		}
	}

	return nil
}

func GenerateReceipt(ixType int, gasConsumed uint64, ixHash ktypes.Hash, senderHash,
senderContextHash, receiverHash, receiverContextHash []byte, data interface{}) (*ktypes.Receipt, error) {

	r := new(ktypes.Receipt)
	r.IxType = ixType
	r.GasUsed = gasConsumed
	r.IxHash = ixHash
	r.SenderStateHash = senderHash
	r.ReceiverStateHash = receiverHash
	r.SenderContextHash = senderContextHash
	r.ReceiverContextHash = receiverContextHash

	rawData, err := json.Marshal(data)
	if err != nil {
		log.Panicln(err)
	}
	r.ExtraData = rawData
	return r, nil
}

func (e *Exec) CreateAsset(creator *guna.StateObject, assetDetails *ktypes.AssetDataInput) (string, error) {
	assetID, err := creator.CreateAsset(uint8(assetDetails.Dimension), assetDetails.IsFungible,
assetDetails.IsMintable, assetDetails.Symbol, int64(assetDetails.TotalSupply), assetDetails.Code)
	if err != nil {
		return "", err
	}
	return string(*assetID), nil
}
func (e *Exec) ValueTransfer(sender, receiver *guna.StateObject
, assetId ktypes.AssetID, value *big.Int) (uint64, error) {

	bal, err := sender.BalanceOf(assetId)
	if err != nil {
		return 0, err
	}
	if value.Sign() <= 0 {
		return 0, errors.New("invalid transfer amount")
	}
	if bal.Cmp(value) == -1 {

		return 0, errors.New("low balance")
	}

	sender.SubBalance(assetId, value)

	receiver.AddBalance(assetId, value)

	return 1, nil

}
*/
