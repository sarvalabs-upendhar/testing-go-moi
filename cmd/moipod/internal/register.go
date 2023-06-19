package internal

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	cmdCommon "github.com/sarvalabs/moichain/cmd/common"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/jug/pisa"
	id "github.com/sarvalabs/moichain/mudra/kramaid"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"github.com/sarvalabs/moichain/common/tests"
	gtypes "github.com/sarvalabs/moichain/guna/types"
	"github.com/sarvalabs/moichain/moiclient"
	"github.com/sarvalabs/moichain/mudra"
	mudraCommon "github.com/sarvalabs/moichain/mudra/common"
	"github.com/sarvalabs/moichain/mudra/poi/moinode"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
	"github.com/spf13/cobra"
)

var (
	rpcURL        string
	keystorePath  string
	nodeDataDir   string
	nodeIndex     int32
	walletAddress string
	nodePassword  string
	accounts      []tests.AccountWithMnemonic
)

func GetRegisterCommand() *cobra.Command {
	registerCmd := &cobra.Command{
		Use:   "register",
		Short: "Register the guardian information with MOI protocol.",
		Run:   runRegisterCommand,
	}

	parseRegisterFlags(registerCmd)

	return registerCmd
}

func parseRegisterFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&rpcURL, "rpc-url", "http://localhost:1600/", "JSON RPC end point.")
	cmd.PersistentFlags().StringVar(&keystorePath, "keystore-path", "", "Path to keystore.")
	cmd.PersistentFlags().StringVar(&nodeDataDir, "data-dir", "", "Path to node data directory.")
	cmd.PersistentFlags().StringVar(&walletAddress, "wallet-address", "",
		"Incentive wallet address.")
	cmd.PersistentFlags().Int32Var(&nodeIndex, "node-index", 0, "Validator node index.")
	cmd.PersistentFlags().StringVar(&nodePassword, "node-password", "", "Passcode to encrypt the node keystore.")

	_ = cmd.MarkPersistentFlagRequired("keystore-path")
	_ = cmd.MarkPersistentFlagRequired("data-dir")
	_ = cmd.MarkPersistentFlagRequired("wallet-address")
	_ = cmd.MarkPersistentFlagRequired("node-index")
	_ = cmd.MarkPersistentFlagRequired("node-password")
}

func validateFlags() error {
	if keystorePath == "" {
		return errors.New("invalid key store path")
	}

	if walletAddress == "" {
		return errors.New("invalid incentive wallet address")
	}

	if nodeIndex == -1 {
		return errors.New("invalid node index")
	}

	return nil
}

func runRegisterCommand(cmd *cobra.Command, args []string) {
	if err := validateFlags(); err != nil {
		cmdCommon.Err(err)
	}

	file, err := os.ReadFile(keystorePath)
	if err != nil {
		cmdCommon.Err(err)
	}

	if err = json.Unmarshal(file, &accounts); err != nil {
		cmdCommon.Err(err)
	}

	vault, err := mudra.NewVault(&mudra.VaultConfig{
		DataDir:      nodeDataDir,
		NodeIndex:    uint32(nodeIndex),
		Mode:         mudra.UserMode,
		SeedPhrase:   accounts[0].Mnemonic,
		NodePassword: nodePassword,
		InMemory:     false,
	}, moinode.MoiFullNode, 1)
	if err != nil {
		cmdCommon.Err(err)
	}

	registerGuardian(vault)
}

func registerGuardian(vault *mudra.KramaVault) {
	client, err := moiclient.NewClient(rpcURL)
	if err != nil {
		cmdCommon.Err(errors.Wrap(err, "failed to create moi-client"))
	}

	if isGuardianRegistered(vault.KramaID(), client) {
		cmdCommon.Err(errors.New("Guardian already registered"))
	}

	moiID, err := vault.MOiID()
	if err != nil {
		cmdCommon.Err(errors.Wrap(err, "failed to generate moiID"))
	}

	g := gtypes.Guardian{
		GuardianOperator: moiID,
		KramaID:          string(vault.KramaID()),
		PublicKey:        vault.GetConsensusPrivateKey().GetPublicKeyInBytes(),
		IncentiveWallet:  types.HexToAddress(walletAddress),
	}

	dc := make(polo.Document)
	if err = dc.Set("operator", vault.KramaID()); err != nil {
		cmdCommon.Err(err)
	}

	guardianDoc, err := polo.DocumentEncode(g)
	if err != nil {
		cmdCommon.Err(err)
	}

	if err = dc.Set("guardianDetails", guardianDoc); err != nil {
		cmdCommon.Err(err)
	}

	moiIDpublicKey, err := vault.GetPublicKeyAt(common.DefaultMOIIDPath)
	if err != nil {
		cmdCommon.Err(err)
	}

	// FIXME: kramaID.moiID is not returning the corr
	log.Println(types.HexToAddress(moiID), types.BytesToAddress(moiIDpublicKey).Hex())

	nonce, err := client.InteractionCount(&ptypes.InteractionCountArgs{
		Address: types.BytesToAddress(moiIDpublicKey),
		Options: ptypes.TesseractNumberOrHash{
			TesseractNumber: &ptypes.LatestTesseractHeight,
		},
	})
	if err != nil {
		cmdCommon.Err(errors.Wrap(err, "failed to fetch nonce"))
	}

	logicPayload := &types.LogicPayload{
		Logic:    types.GuardianLogicID,
		Callsite: "Register!",
		Calldata: dc.Bytes(),
	}

	rawPayload, err := logicPayload.Bytes()
	if err != nil {
		cmdCommon.Err(err)
	}

	ixArgs := types.SendIXArgs{
		Type:      types.IxLogicInvoke,
		Sender:    types.BytesToAddress(moiIDpublicKey),
		Nonce:     nonce.ToUint64(),
		FuelPrice: big.NewInt(1),
		FuelLimit: big.NewInt(1000),
		Payload:   rawPayload,
	}

	rawArgs, err := ixArgs.Bytes()
	if err != nil {
		cmdCommon.Err(err)
	}

	signature, err := vault.Sign(rawArgs, mudraCommon.EcdsaSecp256k1, mudra.UsingIgcPath(mudra.DefaultMOIIDPath))
	if err != nil {
		cmdCommon.Err(err)
	}

	ixHash, err := client.SendInteractions(&ptypes.SendIX{
		IXArgs:    hex.EncodeToString(rawArgs),
		Signature: hex.EncodeToString(signature),
	})
	if err != nil {
		cmdCommon.Err(err)
	}

	fmt.Println("Sit back and relax, registration in progress")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	rpcReceipt, err := cmdCommon.WaitForReceipts(ctx, client, ixHash)
	if err != nil {
		cmdCommon.Err(err)
	}

	if rpcReceipt.Status == types.ReceiptOk {
		fmt.Println("Registration successful")
		fmt.Printf("Registered guardian details %+v", g)
	}

	fmt.Println("Registration failed", rpcReceipt.ExtraData)
}

func isGuardianRegistered(kramaID id.KramaID, client *moiclient.Client) bool {
	storageData, err := client.Storage(&ptypes.GetStorageArgs{
		LogicID:    types.GuardianLogicID,
		StorageKey: pisa.SlotHash(gtypes.GuardianSLot),
		Options: ptypes.TesseractNumberOrHash{
			TesseractNumber: &ptypes.LatestTesseractHeight,
		},
	})
	if err != nil {
		cmdCommon.Err(errors.Wrap(err, "failed to fetch guardian information from the network"))
	}

	guardians := make(gtypes.Guardians)

	err = polo.Depolorize(&guardians, storageData.Bytes())
	if err != nil {
		cmdCommon.Err(errors.Wrap(err, "failed to fetch guardian information from the network"))
	}

	_, ok := guardians[string(kramaID)]

	return ok
}
