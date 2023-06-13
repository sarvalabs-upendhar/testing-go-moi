package common

import (
	"context"
	"fmt"
	"os"

	"github.com/sarvalabs/moichain/moiclient"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
)

type Instance struct {
	KramaID      string `json:"krama_id"`
	RPCUrl       string `json:"rpc_url"`
	ConsensusKey string `json:"consensus_key"`
}

func Err(err error) {
	if err != nil {
		fmt.Println("MOIPod failed Error occurred:", err)
		os.Exit(1)
	}
}

func WaitForReceipts(ctx context.Context, client *moiclient.Client, ixHash types.Hash) (*ptypes.RPCReceipt, error) {
	for {
		select {
		case <-ctx.Done():
			fmt.Printf("Failed to fetch receipt please try after some time IxHash %s \n", ixHash)

			return nil, ctx.Err()
		default:
			rpcReceipt, err := client.InteractionReceipt(&ptypes.ReceiptArgs{
				Hash: ixHash,
			})
			if err != nil {
				continue
			}

			return rpcReceipt, err
		}
	}
}
