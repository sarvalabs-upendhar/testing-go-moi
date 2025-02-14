package internal

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/peterh/liner"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	cmdCommon "github.com/sarvalabs/go-moi/cmd/common"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/corelogics/guardianregistry"
	"github.com/sarvalabs/go-moi/crypto"
	mudraCommon "github.com/sarvalabs/go-moi/crypto/common"
	"github.com/sarvalabs/go-moi/crypto/poi/moinode"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
	"github.com/sarvalabs/go-polo"
)

var (
	senderID                 string
	senderKeyID              int32
	networkRPC               string
	nodeDataDir              string
	nodeIndex                int32
	walletAddress            string
	nodePassword             string
	localRPC                 string
	mnemonicKeystorePath     string
	mnemonicKeystorePassword string
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
		&senderID,
		"sender-id",
		"",
		"sender id",
	)
	cmd.PersistentFlags().Int32Var(
		&senderKeyID,
		"sender-key-id",
		-1,
		"sender key id",
	)
	cmd.PersistentFlags().StringVar(
		&mnemonicKeystorePath,
		"mnemonic-keystore-path",
		"",
		"Path to mnemonic keystore.",
	)
	cmd.PersistentFlags().StringVar(
		&mnemonicKeystorePassword,
		"mnemonic-keystore-password",
		os.Getenv("MNEMONIC_KEYSTORE_PASSWORD"),
		"Password to decrypt mnemonic keystore.",
	)
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
		os.Getenv("NODE_PASSWORD"),
		"Passcode to encrypt the node keystore.",
	)

	_ = cmd.MarkPersistentFlagRequired("sender-id")
	_ = cmd.MarkPersistentFlagRequired("sender-key-id")
	_ = cmd.MarkPersistentFlagRequired("sender-public-key")
	_ = cmd.MarkPersistentFlagRequired("data-dir")
	_ = cmd.MarkPersistentFlagRequired("wallet-address")
	_ = cmd.MarkPersistentFlagRequired("node-index")
	_ = cmd.MarkPersistentFlagRequired("mnemonic-keystore-path")
}

func validateFlags() error {
	if mnemonicKeystorePath == "" {
		return errors.New("invalid mnemonic key store path")
	}

	if walletAddress == "" {
		return errors.New("invalid incentive wallet address")
	}

	if senderID == "" {
		return errors.New("invalid sender id")
	}

	if senderKeyID < 0 {
		return errors.New("invalid sender key id")
	}

	if nodeIndex == -1 {
		return errors.New("invalid node index")
	}

	if _, err := os.Stat(nodeDataDir); err != nil {
		if os.IsNotExist(err) {
			return errors.New("no data directory found at the given path")
		}

		return err
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

	if mnemonicKeystorePassword == "" {
		password, err := line.PasswordPrompt("Enter mnemonic key store password :")
		if err != nil {
			cmdCommon.Err(err)
		}

		mnemonicKeystorePassword = password
	}

	if nodePassword == "" {
		password, err := line.PasswordPrompt("Enter node password :")
		if err != nil {
			cmdCommon.Err(err)
		}

		nodePassword = password
	}

	vault, err := crypto.NewVault(&crypto.VaultConfig{
		DataDir:                  nodeDataDir,
		NodeIndex:                uint32(nodeIndex),
		Mode:                     crypto.UserMode,
		NodePassword:             nodePassword,
		InMemory:                 false,
		MnemonicKeystorePath:     mnemonicKeystorePath,
		MnemonicKeystorePassword: mnemonicKeystorePassword,
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

	sender, err := identifiers.NewParticipantIDFromHex(senderID)
	if err != nil {
		panic(err)
	}

	// Check if the guardian is already registered
	isRegistered, err := cmdCommon.IsGuardianRegistered(client, vault.KramaID())
	if err != nil {
		cmdCommon.Err(err)
	}

	if isRegistered {
		cmdCommon.Err(errors.New("guardian already registered"))
	}

	// Get the operator MOI ID from the vault
	moiID, err := vault.MoiID()
	if err != nil {
		cmdCommon.Err(errors.Wrap(err, "failed to generate moiID"))
	}

	fmt.Printf("Krama-ID %s \n", vault.KramaID())

	sequenceID, err := client.InteractionCount(context.Background(), &rpcargs.InteractionCountArgs{
		ID:    sender.AsIdentifier(),
		KeyID: uint64(senderKeyID),
		Options: rpcargs.TesseractNumberOrHash{
			TesseractNumber: &rpcargs.LatestTesseractHeight,
		},
	})
	if err != nil {
		cmdCommon.Err(errors.Wrap(err, "failed to fetch sequenceID"))
	}

	logicPayload := &common.LogicPayload{
		Logic:    common.GuardianLogicID,
		Callsite: "RegisterGuardian",
		Calldata: func() polo.Document {
			// Create a guardian object to register
			guardian := guardianregistry.Guardian{
				OperatorID: moiID,
				KramaID:    string(vault.KramaID()),
				PublicKey:  vault.GetConsensusPrivateKey().GetPublicKeyInBytes(),
				Incentive: guardianregistry.Incentive{
					Wallet: sender.AsIdentifier(),
				},
			}

			doc := make(polo.Document)
			// Set the guardian input data
			if err = doc.Set("guardian", guardian, polo.DocStructs()); err != nil {
				cmdCommon.Err(err)
			}

			return doc
		}().Bytes(),
	}

	rawPayload, err := logicPayload.Bytes()
	if err != nil {
		cmdCommon.Err(err)
	}

	ixArgs := common.IxData{
		Sender: common.Sender{
			ID:         sender.AsIdentifier(),
			SequenceID: sequenceID.ToUint64(),
			KeyID:      uint64(senderKeyID),
		},
		FuelPrice: big.NewInt(1),
		FuelLimit: 10000,
		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxLogicInvoke,
				Payload: rawPayload,
			},
		},
		Participants: []common.IxParticipant{
			{
				ID:       sender.AsIdentifier(),
				LockType: common.MutateLock,
			},
			{
				ID:       common.GuardianAccountID,
				LockType: common.MutateLock,
			},
		},
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
		IXArgs:     hex.EncodeToString(rawArgs),
		Signatures: hex.EncodeToString(signature),
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
		fmt.Println("Registration failed err", string(rpcReceipt.IxOps[0].Data))

		return
	}

	fmt.Println("Registration successful")
	fmt.Println("Registered guardian details")
}
