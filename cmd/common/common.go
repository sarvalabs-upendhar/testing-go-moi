package common

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/crypto/poi"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
)

func Err(err error) {
	if err != nil {
		fmt.Println("MOIPod failed Error occurred:", err)
		os.Exit(1)
	}
}

func WaitForReceipts(ctx context.Context, client *moiclient.Client, ixHash common.Hash) (*args.RPCReceipt, error) {
	for {
		select {
		case <-ctx.Done():
			fmt.Printf("failed to fetch receipt please try after some time IxHash %s \n", ixHash)

			return nil, ctx.Err()
		default:
			rpcReceipt, err := client.InteractionReceipt(ctx, &args.ReceiptArgs{
				Hash: ixHash,
			})
			if err != nil {
				continue
			}

			return rpcReceipt, err
		}
	}
}

// WriteToGenesisFile creates a new file if it doesn't exist, or replaces an existing one.
func WriteToGenesisFile(path string, genesis *common.GenesisFile) error {
	file, err := json.MarshalIndent(genesis, "", "\t")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, file, os.ModePerm); err != nil {
		return err
	}

	log.Println("Genesis file created or updated")

	return nil
}

func GetIP() (string, error) {
	// Retrieve the network interface addresses of the host machine
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		// Return the error
		return "", err
	}

	// Iterate over the network interface addresses
	for _, a := range addrs {
		// Check that the address is an IP address and not a loopback address
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			// Check if the IP address is an IPv4 address
			if ipnet.IP.To4() != nil {
				// Convert into a string IP address and store to variable
				return ipnet.IP.String(), nil
			}
		}
	}

	return "", errors.New("this node's ip not found")
}

func GetAccountsWithMnemonic(accountCount int) ([]tests.AccountWithMnemonic, error) {
	accounts := make([]tests.AccountWithMnemonic, 0, accountCount)

	for i := 0; i < accountCount; i++ {
		mnemonic := poi.GenerateRandMnemonic().String()

		_, publicKey, err := poi.GetPrivateKeyAtPath(mnemonic, config.DefaultMoiWalletPath)
		if err != nil {
			return nil, err
		}

		participantID, err := identifiers.GenerateParticipantIDv0(common.NewAccounIDFromBytes(publicKey), 0)
		if err != nil {
			return nil, err
		}

		accounts = append(accounts,
			tests.AccountWithMnemonic{
				ID:        participantID.AsIdentifier(),
				Mnemonic:  mnemonic,
				PublicKey: publicKey,
			})
	}

	return accounts, nil
}
