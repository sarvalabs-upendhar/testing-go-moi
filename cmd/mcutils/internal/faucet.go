package internal

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"time"

	cryptocommon "github.com/sarvalabs/go-moi/crypto/common"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"

	cmdcommon "github.com/sarvalabs/go-moi/cmd/common"
	"github.com/sarvalabs/go-moi/common"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/crypto"
	"github.com/sarvalabs/go-moi/crypto/poi/moinode"
	"github.com/sarvalabs/go-moi/moiclient"
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
		cmdcommon.Err(err)
	}

	if err = json.Unmarshal(file, &accounts); err != nil {
		cmdcommon.Err(err)
	}

	client, _ := moiclient.NewClient(rpcURL)

	vault, err := crypto.NewVault(&crypto.VaultConfig{
		SeedPhrase: accounts[0].Mnemonic,
		Mode:       crypto.UserMode,
		InMemory:   true,
	}, moinode.MoiFullNode, 1)
	if err != nil {
		cmdcommon.Err(err)
	}

	faucetWalletPublicKey, err := vault.GetPublicKeyAt(crypto.DefaultMOIWalletPath)
	if err != nil {
		cmdcommon.Err(err)
	}

	nonce, err := client.InteractionCount(&rpcargs.InteractionCountArgs{
		Address: common.BytesToAddress(faucetWalletPublicKey),
		Options: rpcargs.TesseractNumberOrHash{
			TesseractNumber: &rpcargs.LatestTesseractHeight,
		},
	})
	if err != nil {
		cmdcommon.Err(errors.Wrap(err, "failed to fetch nonce"))
	}

	ixArgs := common.SendIXArgs{
		Type:      common.IxValueTransfer,
		Sender:    common.BytesToAddress(faucetWalletPublicKey),
		Receiver:  common.HexToAddress(walletAddress),
		Nonce:     nonce.ToUint64(),
		FuelPrice: big.NewInt(1),
		FuelLimit: big.NewInt(1000),
		TransferValues: map[common.AssetID]*big.Int{
			common.KMOITokenAssetID: new(big.Int).SetUint64(amount),
		},
	}

	rawArgs, err := ixArgs.Bytes()
	if err != nil {
		cmdcommon.Err(err)
	}

	signature, err := vault.Sign(rawArgs, cryptocommon.EcdsaSecp256k1, crypto.UsingIgcPath(crypto.DefaultMOIWalletPath))
	if err != nil {
		cmdcommon.Err(err)
	}

	ixHash, err := client.SendInteractions(&rpcargs.SendIX{
		IXArgs:    hex.EncodeToString(rawArgs),
		Signature: hex.EncodeToString(signature),
	})
	if err != nil {
		cmdcommon.Err(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	rpcReceipt, err := cmdcommon.WaitForReceipts(ctx, client, ixHash)
	if err != nil {
		cmdcommon.Err(err)
	}

	if rpcReceipt.Status == common.ReceiptOk {
		fmt.Printf(" %d Gas tokens credited to %s \n", amount, walletAddress)

		return
	}
}
