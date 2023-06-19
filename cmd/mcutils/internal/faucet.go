package internal

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/sarvalabs/moichain/cmd/common"

	mudraCommon "github.com/sarvalabs/moichain/mudra/common"

	"github.com/pkg/errors"
	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/moiclient"
	"github.com/sarvalabs/moichain/mudra"
	"github.com/sarvalabs/moichain/mudra/poi/moinode"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
	"github.com/spf13/cobra"
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

	vault, err := mudra.NewVault(&mudra.VaultConfig{
		SeedPhrase: accounts[0].Mnemonic,
		Mode:       mudra.UserMode,
		InMemory:   true,
	}, moinode.MoiFullNode, 1)
	if err != nil {
		common.Err(err)
	}

	faucetWalletPublicKey, err := vault.GetPublicKeyAt(mudra.DefaultMOIWalletPath)
	if err != nil {
		common.Err(err)
	}

	nonce, err := client.InteractionCount(&ptypes.InteractionCountArgs{
		Address: types.BytesToAddress(faucetWalletPublicKey),
		Options: ptypes.TesseractNumberOrHash{
			TesseractNumber: &ptypes.LatestTesseractHeight,
		},
	})
	if err != nil {
		common.Err(errors.Wrap(err, "failed to fetch nonce"))
	}

	ixArgs := types.SendIXArgs{
		Type:      types.IxValueTransfer,
		Sender:    types.BytesToAddress(faucetWalletPublicKey),
		Receiver:  types.HexToAddress(walletAddress),
		Nonce:     nonce.ToUint64(),
		FuelPrice: big.NewInt(1),
		FuelLimit: big.NewInt(1000),
		TransferValues: map[types.AssetID]*big.Int{
			types.MOITokenAssetID: new(big.Int).SetUint64(amount),
		},
	}

	rawArgs, err := ixArgs.Bytes()
	if err != nil {
		common.Err(err)
	}

	signature, err := vault.Sign(rawArgs, mudraCommon.EcdsaSecp256k1, mudra.UsingIgcPath(mudra.DefaultMOIWalletPath))
	if err != nil {
		common.Err(err)
	}

	ixHash, err := client.SendInteractions(&ptypes.SendIX{
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

	if rpcReceipt.Status == types.ReceiptOk {
		fmt.Printf(" %d Gas tokens credited to %s \n", amount, walletAddress)

		return
	}
}
