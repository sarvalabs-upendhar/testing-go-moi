package api

import (
	"github.com/sarvalabs/go-moi/common"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
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
func (p *PublicDebugAPI) DBGet(args *rpcargs.DebugArgs) (string, error) {
	encodedData := common.FromHex(args.Key)

	// Read the value of the encodedData from the database
	content, err := p.db.ReadEntry(encodedData)
	if err != nil {
		return "", err
	}

	decodedData := common.BytesToHex(content)

	return decodedData, nil
}

// GetAccounts returns a list of registered account addresses
func (p *PublicDebugAPI) GetAccounts() ([]common.Address, error) {
	return p.db.GetRegisteredAccounts()
}
