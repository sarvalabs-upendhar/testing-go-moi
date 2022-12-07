package engine

// AssetAction represents some asset action trigger
type AssetAction int

const (
	AssetInspect AssetAction = iota
	AssetMint
	AssetBurn
	AssetSend
	AssetReceive
	AssetApprove
	AssetRevoke
)

// AssetEngine describes an interface around the ExecutionEngine for supporting
// asset based logics with strict method definitions for asset logic triggers.
// TODO: needs to be properly defined and documented
type AssetEngine interface {
	ExecutionEngine
	RunAssetAction(AssetAction, *LogicObject) error
}
