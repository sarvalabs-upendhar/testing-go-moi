package moinode

type MoiNodeType uint

// ByteString returns the constant byte associated with NODE Type
func (mnT MoiNodeType) ByteString() string {
	switch mnT {
	case MoiProvenanceNode:
		return "0x01"
	case MoiFullNode:
		return "0x07"
	default:
		return "0x00"
	}
}

const (
	MoiProvenanceNode MoiNodeType = iota
	MoiFullNode
)

type MoiNode struct {
	UserMoiID     string `json:"userMoiID"`
	NodeIndex     int    `json:"nodeIndex"`
	NodePublicKey string `json:"nodePubKey"`
	NodeType      string `json:"nodeType"`
	KramaID       string `json:"kramaID"`
	Lat           string `json:"latitude"`
	Long          string `json:"longitude"`
}
