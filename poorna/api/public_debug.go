package api

import (
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
)

// PublicDebugAPI is the collection of APIs exposed over the public debugging endpoint
type PublicDebugAPI struct {
	db DB
}

func NewPublicDebugAPI(db DB) *PublicDebugAPI {
	// Create the public Debug API wrapper and return it
	return &PublicDebugAPI{db}
}

// DBGet returns the raw value of the key that is stored in the database
func (p *PublicDebugAPI) DBGet(args *ptypes.DebugArgs) (string, error) {
	encodedData := types.FromHex(args.Key)

	// Read the value of the encodedData from the database
	content, err := p.db.ReadEntry(encodedData)
	if err != nil {
		return "", err
	}

	decodedData := types.BytesToHex(content)

	return decodedData, nil
}
