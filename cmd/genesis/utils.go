package genesis

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"strings"

	"github.com/sarvalabs/moichain/types"

	"github.com/pkg/errors"
	"github.com/sarvalabs/moichain/cmd/common"
	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa"
)

func readInstancesFile(path string) ([]common.Instance, error) {
	instances := make([]common.Instance, 0)

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
	instances, err := readInstancesFile(path)
	if err != nil {
		return nil, err
	}

	kramaIDs := make([]string, len(instances))
	for i, instance := range instances {
		kramaIDs[i] = instance.KramaID
	}

	return kramaIDs, nil
}

func readGenesisFile() (*types.GenesisFile, error) {
	genesis := new(types.GenesisFile)

	file, err := os.ReadFile(genesisFilePath)
	if err != nil {
		return nil, errors.New("error reading genesis file")
	}

	if err = json.Unmarshal(file, genesis); err != nil {
		return nil, errors.New("error reading genesis file")
	}

	return genesis, nil
}

// WriteToGenesisFile creates a new file if it doesn't exist, or replaces an existing one.
func WriteToGenesisFile(path string, genesis *types.GenesisFile) error {
	file, err := json.MarshalIndent(genesis, "", "\t")
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(path, file, os.ModePerm); err != nil {
		return err
	}

	fmt.Println("Genesis file created or updated")

	return nil
}

// parseUint256orHex returns big int from string, if string has 0x prefix it is treated as hex
// else it will be treated as decimal number
func parseUint256orHex(val *string) (*big.Int, error) {
	if val == nil {
		return nil, nil
	}

	str := *val
	base := 10

	if strings.HasPrefix(str, "0x") {
		str = str[2:]
		base = 16
	}

	b, ok := new(big.Int).SetString(str, base)
	if !ok {
		return nil, fmt.Errorf("could not parse")
	}

	return b, nil
}

func getContextNodes(instancesFile string, behaviourCount, randomCount int) ([]string, []string) {
	kramaIDs, err := ReadKramaIDsFromInstancesFile(instancesFile)
	if err != nil {
		common.Err(err)
	}

	if behaviourCount+randomCount > len(kramaIDs) {
		common.Err(errors.New("insufficient krama IDs"))
	}

	return kramaIDs[0:behaviourCount],
		kramaIDs[behaviourCount : behaviourCount+randomCount]
}

// ReadManifest Reads the manifest file at the given
// filepath and returns it as POLO encoded hex string
func ReadManifest(filePath string) ([]byte, error) {
	// Register the PISA element registry with the EngineIO package
	engineio.RegisterEngineRuntime(pisa.NewRuntime())

	// Decode the manifest into a Manifest object
	manifest, err := engineio.ReadManifestFile(filePath)
	if err != nil {
		return nil, err
	}

	// Encode the Manifest into POLO data
	encoded, err := manifest.Encode(engineio.POLO)
	if err != nil {
		return nil, err
	}

	return encoded, nil
}

func readArtifactFile(path string) (*Artifact, error) {
	ar := new(Artifact)

	file, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("error reading artifact file")
	}

	if err = json.Unmarshal(file, ar); err != nil {
		return nil, errors.New("error unmarshalling into artifact")
	}

	return ar, nil
}
