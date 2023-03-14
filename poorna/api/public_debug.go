package api

import (
	"context"

	ptypes "github.com/sarvalabs/moichain/poorna/types"

	"github.com/sarvalabs/moichain/dhruva"
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

// GetAccounts returns a list of registered account addresses
func (p *PublicDebugAPI) GetAccounts() ([]types.Address, error) {
	addrsList := make([]types.Address, 0)

	for i := int64(0); i <= 1024; i++ {
		prefix := dhruva.IDToBytes(i)

		entries, err := p.db.GetEntriesWithPrefix(context.Background(), prefix)
		if err != nil {
			return nil, err
		}

		for entry := range entries {
			addr := entry.Key[10:]
			addrsList = append(addrsList, types.BytesToAddress(addr))
		}
	}

	return addrsList, nil
}
