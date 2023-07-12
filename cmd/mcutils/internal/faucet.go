package internal

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"time"

	mudraCommon "github.com/sarvalabs/moichain/crypto/common"
	rpcargs "github.com/sarvalabs/moichain/jsonrpc/args"

	"github.com/sarvalabs/moichain/cmd/common"
	common2 "github.com/sarvalabs/moichain/common"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/crypto"
	"github.com/sarvalabs/moichain/crypto/poi/moinode"
	"github.com/sarvalabs/moichain/moiclient"
)

var (
	rpcURL        string
	keystorePath  string
	walletAddress string
	amount        uint64
	accounts      []tests.AccountWithMnemonic
)

func GetFaucetCommand() *cobra.Command {
	serverCmd := &cobra.Command{
		Use:   "faucet",
		Short: "Faucet to get fee token.",
		Run:   runFaucetCommand,
	}

	parseFaucetFlags(serverCmd)

	return serverCmd
}

func parseFaucetFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().Uint64Var(&amount, "amount", 0, "Amount to MOI tokens reuired from faucet.")
	cmd.PersistentFlags().StringVar(&rpcURL, "rpc-url", "http://localhost:1600/", "JSON RPC end point.")
	cmd.PersistentFlags().StringVar(&keystorePath, "keystore-path", "", "Path to keystore file.")
	cmd.PersistentFlags().StringVar(&walletAddress, "wallet-address", "",
		"Wallet address to credit MOI tokens.")

	_ = cmd.MarkPersistentFlagRequired("amount")
	_ = cmd.MarkPersistentFlagRequired("keystore-path")
	_ = cmd.MarkPersistentFlagRequired("wallet-address")
}

func runFaucetCommand(cmd *cobra.Command, args []string) {
	file, err := os.ReadFile(keystorePath)
	if err != nil {
		common.Err(err)
	}

	if err = json.Unmarshal(file, &accounts); err != nil {
		common.Err(err)
	}

	client, _ := moiclient.NewClient(rpcURL)

	vault, err := crypto.NewVault(&crypto.VaultConfig{
		SeedPhrase: accounts[0].Mnemonic,
		Mode:       crypto.UserMode,
		InMemory:   true,
	}, moinode.MoiFullNode, 1)
	if err != nil {
		common.Err(err)
	}

	faucetWalletPublicKey, err := vault.GetPublicKeyAt(crypto.DefaultMOIWalletPath)
	if err != nil {
		common.Err(err)
	}

	nonce, err := client.InteractionCount(&rpcargs.InteractionCountArgs{
		Address: common2.BytesToAddress(faucetWalletPublicKey),
		Options: rpcargs.TesseractNumberOrHash{
			TesseractNumber: &rpcargs.LatestTesseractHeight,
		},
	})
	if err != nil {
		common.Err(errors.Wrap(err, "failed to fetch nonce"))
	}

	ixArgs := common2.SendIXArgs{
		Type:      common2.IxValueTransfer,
		Sender:    common2.BytesToAddress(faucetWalletPublicKey),
		Receiver:  common2.HexToAddress(walletAddress),
		Nonce:     nonce.ToUint64(),
		FuelPrice: big.NewInt(1),
		FuelLimit: big.NewInt(1000),
		TransferValues: map[common2.AssetID]*big.Int{
			common2.KMOITokenAssetID: new(big.Int).SetUint64(amount),
		},
	}

	rawArgs, err := ixArgs.Bytes()
	if err != nil {
		common.Err(err)
	}

	signature, err := vault.Sign(rawArgs, mudraCommon.EcdsaSecp256k1, crypto.UsingIgcPath(crypto.DefaultMOIWalletPath))
	if err != nil {
		common.Err(err)
	}

	ixHash, err := client.SendInteractions(&rpcargs.SendIX{
		IXArgs:    hex.EncodeToString(rawArgs),
		Signature: hex.EncodeToString(signature),
	})
	if err != nil {
		common.Err(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	rpcReceipt, err := common.WaitForReceipts(ctx, client, ixHash)
	if err != nil {
		common.Err(err)
	}

	if rpcReceipt.Status == common2.ReceiptOk {
		fmt.Printf(" %d Gas tokens credited to %s \n", amount, walletAddress)

		return
	}
}
