package guna

// EPOCH is a basic state variable
type EPOCH struct {
	EpochNumber uint `json:"epoch_num"`
}

// PeerID is the string version of the libp2p peer id
type PeerID string

// HashString represents any hash in string format
type HashString string

// DIDMethod is a hypothetical DID of a user, to be replaced with proper structure (TODO)
type DIDMethod string

// Asset is a type differentiator for transactions carrying various commodity payloads
type Asset string
