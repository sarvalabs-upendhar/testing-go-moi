package internal

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
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
	networkRPC    string
	keystorePath  string
	nodeDataDir   string
	nodeIndex     int32
	walletAddress string
	nodePassword  string
	localRPC      string
	watchDogURL   string
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
	cmd.PersistentFlags().StringVar(&keystorePath, "keystore-path", "", "Path to keystore.")
	cmd.PersistentFlags().StringVar(&watchDogURL, "watchdog-url", "", "WatchDog service url")
	cmd.PersistentFlags().StringVar(&nodeDataDir, "data-dir", "", "Path to node data directory.")
	cmd.PersistentFlags().StringVar(
		&networkRPC,
		"network-rpc-url",
		"http://localhost:1600/",
		"Network JSON RPC end point.",
	)
	cmd.PersistentFlags().StringVar(
		&localRPC,
		"local-rpc-url",
		"",
		"Local JSON RPC end point.",
	)
	cmd.PersistentFlags().StringVar(
		&walletAddress,
		"wallet-address",
		"",
		"Incentive wallet address.",
	)
	cmd.PersistentFlags().Int32Var(
		&nodeIndex,
		"node-index",
		0,
		"Validator node index.",
	)
	cmd.PersistentFlags().StringVar(
		&nodePassword,
		"node-password",
		"",
		"Passcode to encrypt the node keystore.",
	)

	_ = cmd.MarkPersistentFlagRequired("watchdog-url")
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
		cmdCommon.Err(errors.Wrap(err, "failed to read keystore file"))
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
	client, err := moiclient.NewClient(networkRPC)
	if err != nil {
		cmdCommon.Err(errors.Wrap(err, "failed to create moi-client"))
	}

	if isGuardianRegistered(vault.KramaID(), client) {
		cmdCommon.Err(errors.New("Guardian already registered"))
	}

	moiID, err := vault.MoiID()
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
	if err = dc.Set("operator", moiID); err != nil {
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

	fmt.Printf("Krama-ID %s", vault.KramaID())

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
		FuelLimit: big.NewInt(10000),
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

	if rpcReceipt.Status != types.ReceiptOk {
		fmt.Println("Registration failed err", string(rpcReceipt.ExtraData))

		return
	}

	if err = registerWithWatchDog(vault.KramaID(), localRPC); err != nil {
		cmdCommon.Err(err)

		return
	}

	fmt.Println("Registration successful")
	fmt.Println("Registered guardian details")
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

func registerWithWatchDog(kramaID id.KramaID, rpcURL string) error {
	if rpcURL == "" {
		ipAddr, err := cmdCommon.GetThisNodeIP()
		if err != nil {
			return err
		}

		rpcURL = fmt.Sprintf("%s%s:%d", "http://", ipAddr, common.DefaultJSONRPCPort)
	}

	parsedURL, err := url.Parse(rpcURL)
	if err != nil {
		return errors.Wrap(err, "invalid rpc url")
	}

	if watchDogURL == "" {
		return errors.New("invalid watch dog url")
	}

	reqParams := make(map[string]interface{})

	reqParams["krama_id"] = kramaID
	reqParams["rpc_url"] = parsedURL.String()

	jsonData, err := json.Marshal(reqParams)
	if err != nil {
		return errors.New("failed to marshal request params")
	}

	httpResponse, err := http.Post(watchDogURL, "application/json", bytes.NewBuffer(jsonData)) //nolint
	if err != nil {
		return errors.Wrap(err, "failed to register with watchdog")
	}

	if httpResponse.StatusCode >= 200 && httpResponse.StatusCode < 300 {
		return nil
	}

	return errors.Wrap(err, "failed to register with watchdog")
}
