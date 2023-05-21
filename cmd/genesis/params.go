package genesis

import "github.com/sarvalabs/moichain/common/hexutil"

var (
	genesisFilePath       string
	instancesFilePath     string
	accAddresses          []string
	behaviouralNodesCount int
	randomNodesCount      int
	moiID                 string
	address               string
	accountType           int
	artifact              string
	allocations           []string
	assetInfo             string
)

type Artifact struct {
	Name     string        `json:"name"`
	Callsite string        `json:"callsite"`
	Calldata hexutil.Bytes `json:"calldata"`
	Manifest hexutil.Bytes `json:"manifest"`
}
