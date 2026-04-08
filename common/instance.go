package common

import (
	"encoding/json"
	"os"

	"github.com/pkg/errors"
)

type Instance struct {
	KramaID      string `json:"krama_id"`
	RPCUrl       string `json:"rpc_url"`
	ConsensusKey string `json:"consensus_key"`
}

func ReadInstancesFile(path string) ([]Instance, error) {
	instances := make([]Instance, 0)

	file, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("error reading instances file")
	}

	if err = json.Unmarshal(file, &instances); err != nil {
		return nil, errors.New("error reading instances file")
	}

	return instances, nil
}

func ReadKramaIDsFromInstancesFile(path string) ([]string, error) {
	instances, err := ReadInstancesFile(path)
	if err != nil {
		return nil, err
	}

	kramaIDs := make([]string, len(instances))
	for i, instance := range instances {
		kramaIDs[i] = instance.KramaID
	}

	return kramaIDs, nil
}
