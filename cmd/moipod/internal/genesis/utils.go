package genesis

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"

	cmdcommon "github.com/sarvalabs/go-moi/cmd/common"
	"github.com/sarvalabs/go-moi/common"

	"github.com/pkg/errors"
)

func readGenesisFile() (*common.GenesisFile, error) {
	if _, err := os.Stat(genesisFilePath); os.IsNotExist(err) {
		return &common.GenesisFile{}, nil
	}

	genesis := new(common.GenesisFile)

	file, err := os.ReadFile(genesisFilePath)
	if err != nil {
		return nil, errors.New("error reading genesis file")
	}

	if err = json.Unmarshal(file, genesis); err != nil {
		return nil, errors.New("error reading genesis file")
	}

	return genesis, nil
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
	kramaIDs, err := cmdcommon.ReadKramaIDsFromInstancesFile(instancesFile)
	if err != nil {
		cmdcommon.Err(err)
	}

	if behaviourCount+randomCount > len(kramaIDs) {
		cmdcommon.Err(errors.New("insufficient krama IDs"))
	}

	return kramaIDs[0:behaviourCount],
		kramaIDs[behaviourCount : behaviourCount+randomCount]
}
