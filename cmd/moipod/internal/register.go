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

	"github.com/peterh/liner"

	id "github.com/sarvalabs/go-moi/common/kramaid"
	mudraCommon "github.com/sarvalabs/go-moi/crypto/common"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	gtypes "github.com/sarvalabs/go-moi/state"
	"github.com/sarvalabs/go-polo"

	cmdCommon "github.com/sarvalabs/go-moi/cmd/common"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/sarvalabs/go-moi/compute/pisa"

	"github.com/sarvalabs/go-moi/crypto"
	"github.com/sarvalabs/go-moi/crypto/poi/moinode"
	"github.com/sarvalabs/go-moi/moiclient"
)

var (
	networkRPC           string
	nodeDataDir          string
	nodeIndex            int32
	walletAddress        string
	nodePassword         string
	localRPC             string
	watchDogURL          string
	mnemonicKeystorePath string
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
	cmd.PersistentFlags().StringVar(
		&mnemonicKeystorePath,
		"mnemonic-keystore-path",
		"",
		"Path to mnemonic keystore.",
	)
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
	_ = cmd.MarkPersistentFlagRequired("data-dir")
	_ = cmd.MarkPersistentFlagRequired("wallet-address")
	_ = cmd.MarkPersistentFlagRequired("node-index")
	_ = cmd.MarkPersistentFlagRequired("node-password")
	_ = cmd.MarkPersistentFlagRequired("mnemonic-keystore-path")
}

func validateFlags() error {
	if mnemonicKeystorePath == "" {
		return errors.New("invalid mnemonic key store path")
	}

	if walletAddress == "" {
		return errors.New("invalid incentive wallet address")
	}

	if nodeIndex == -1 {
		return errors.New("invalid node index")
	}

	if _, err := os.Stat(mnemonicKeystorePath); err != nil {
		if os.IsNotExist(err) {
			return mudraCommon.ErrNoMnemonicKeystore
		}

		return err
	}

	return nil
}

func runRegisterCommand(cmd *cobra.Command, args []string) {
	if err := validateFlags(); err != nil {
		cmdCommon.Err(err)
	}

	line := liner.NewLiner()

	masterPassword, err := line.PasswordPrompt("Enter mnemonic key store password :")
	if err != nil {
		cmdCommon.Err(err)
	}

	vault, err := crypto.NewVault(&crypto.VaultConfig{
		DataDir:                  nodeDataDir,
		NodeIndex:                uint32(nodeIndex),
		Mode:                     crypto.UserMode,
		NodePassword:             nodePassword,
		InMemory:                 false,
		MnemonicKeystorePath:     mnemonicKeystorePath,
		MnemonicKeystorePassword: masterPassword,
	}, moinode.MoiFullNode, 1)
	if err != nil {
		cmdCommon.Err(err)
	}

	registerGuardian(vault)
}

func registerGuardian(vault *crypto.KramaVault) {
	client, err := moiclient.NewClient(networkRPC)
	if err != nil {
		cmdCommon.Err(errors.Wrap(err, "failed to create moi-client"))
	}

	if isGuardianRegistered(client, vault.KramaID()) {
		fmt.Println("Guardian already registered, updating details")

		err = registerWithWatchDog(localRPC, vault)
		if err != nil {
			cmdCommon.Err(err)
		}

		return
	}

	moiID, err := vault.MoiID()
	if err != nil {
		cmdCommon.Err(errors.Wrap(err, "failed to generate moiID"))
	}

	g := gtypes.Guardian{
		GuardianOperator: moiID,
		KramaID:          string(vault.KramaID()),
		PublicKey:        vault.GetConsensusPrivateKey().GetPublicKeyInBytes(),
		IncentiveWallet:  common.HexToAddress(walletAddress),
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

	moiIDpublicKey, err := vault.GetPublicKeyAt(config.DefaultMOIIDPath)
	if err != nil {
		cmdCommon.Err(err)
	}

	fmt.Printf("Krama-ID %s \n", vault.KramaID())

	nonce, err := client.InteractionCount(context.Background(), &rpcargs.InteractionCountArgs{
		Address: common.BytesToAddress(moiIDpublicKey),
		Options: rpcargs.TesseractNumberOrHash{
			TesseractNumber: &rpcargs.LatestTesseractHeight,
		},
	})
	if err != nil {
		cmdCommon.Err(errors.Wrap(err, "failed to fetch nonce"))
	}

	logicPayload := &common.LogicPayload{
		Logic:    common.GuardianLogicID,
		Callsite: "Register!",
		Calldata: dc.Bytes(),
	}

	rawPayload, err := logicPayload.Bytes()
	if err != nil {
		cmdCommon.Err(err)
	}

	ixArgs := common.SendIXArgs{
		Type:      common.IxLogicInvoke,
		Sender:    common.BytesToAddress(moiIDpublicKey),
		Nonce:     nonce.ToUint64(),
		FuelPrice: big.NewInt(1),
		FuelLimit: big.NewInt(10000),
		Payload:   rawPayload,
	}

	rawArgs, err := ixArgs.Bytes()
	if err != nil {
		cmdCommon.Err(err)
	}

	signature, err := vault.Sign(rawArgs, mudraCommon.EcdsaSecp256k1, crypto.UsingIgcPath(crypto.DefaultMOIIDPath))
	if err != nil {
		cmdCommon.Err(err)
	}

	ixHash, err := client.SendInteractions(context.Background(), &rpcargs.SendIX{
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

	if rpcReceipt.Status != common.ReceiptOk {
		fmt.Println("Registration failed err", string(rpcReceipt.ExtraData))

		return
	}

	if err = registerWithWatchDog(localRPC, vault); err != nil {
		cmdCommon.Err(err)

		return
	}

	fmt.Println("Registration successful")
	fmt.Println("Registered guardian details")
}

func isGuardianRegistered(client *moiclient.Client, kramaID id.KramaID) bool {
	storageData, err := client.Storage(context.Background(), &rpcargs.GetLogicStorageArgs{
		LogicID:    common.GuardianLogicID,
		StorageKey: pisa.SlotHash(gtypes.GuardianSLot),
		Options: rpcargs.TesseractNumberOrHash{
			TesseractNumber: &rpcargs.LatestTesseractHeight,
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

func registerWithWatchDog(rpcURL string, vault *crypto.KramaVault) error {
	if rpcURL == "" {
		ipAddr, err := cmdCommon.GetThisNodeIP()
		if err != nil {
			return err
		}

		rpcURL = fmt.Sprintf("%s%s:%d", "http://", ipAddr, config.DefaultJSONRPCPort)
	}

	parsedURL, err := url.Parse(rpcURL)
	if err != nil {
		return errors.Wrap(err, "invalid rpc url")
	}

	if watchDogURL == "" {
		return errors.New("invalid watch dog url")
	}

	reqParams := make(map[string]interface{})

	req := cmdCommon.KramaIDReq{
		KramaID: string(vault.KramaID()),
		RPCUrl:  parsedURL.String(),
	}

	rawData, err := req.Bytes()
	if err != nil {
		return nil
	}

	signature, err := vault.Sign(rawData, mudraCommon.EcdsaSecp256k1, crypto.UsingNetworkKey())
	if err != nil {
		return err
	}

	reqParams["krama_id"] = vault.KramaID()
	reqParams["rpc_url"] = parsedURL.String()
	reqParams["signature"] = hex.EncodeToString(signature)

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
