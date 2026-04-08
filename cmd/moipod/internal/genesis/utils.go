package genesis

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/sarvalabs/go-moi/common"

	"github.com/pkg/errors"
	cmdcommon "github.com/sarvalabs/go-moi/cmd/common"
)

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

func getContextNodes(instancesFile string, consensusNodesCount int) []string {
	kramaIDs, err := common.ReadKramaIDsFromInstancesFile(instancesFile)
	if err != nil {
		cmdcommon.Err(err)
	}

	if consensusNodesCount > len(kramaIDs) {
		cmdcommon.Err(errors.New("insufficient krama IDs"))
	}

	return kramaIDs[0:consensusNodesCount]
}
