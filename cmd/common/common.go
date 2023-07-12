package common

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
)

const (
	DefaultBehaviouralCount = 1
	DefaultRandomCount      = 0
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
			fmt.Printf("Failed to fetch receipt please try after some time IxHash %s \n", ixHash)

			return nil, ctx.Err()
		default:
			rpcReceipt, err := client.InteractionReceipt(&args.ReceiptArgs{
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

	if err := ioutil.WriteFile(path, file, os.ModePerm); err != nil {
		return err
	}

	log.Println("Genesis file created or updated")

	return nil
}

func GetThisNodeIP() (string, error) {
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
