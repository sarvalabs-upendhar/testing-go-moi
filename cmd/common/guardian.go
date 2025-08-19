package common

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/sarvalabs/go-moi/common"

	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/crypto"
	mudraCommon "github.com/sarvalabs/go-moi/crypto/common"
	"github.com/sarvalabs/go-moi/moiclient"
)

func IsGuardianRegistered(client *moiclient.Client, kramaID identifiers.KramaID) (bool, error) {
	// Retrieve the value for the storage key from the
	_, err := client.Validators(context.Background(), &rpcargs.GetValidatorsArgs{
		KramaID: kramaID,
	})
	if err == nil {
		// If no error was returned, the key was found.
		// This means the guardian is registered
		return true, nil
	}

	// If the krama id is not found, the guardian is NOT registered
	if err.Error() == common.ErrKramaIDNotFound.Error() {
		return false, nil
	}

	return false, errors.Wrap(err, "failed to fetch guardian information from the network")
}

func RegisterWithWatchDog(rpcURL string, watchdogURL string, vault *crypto.KramaVault) error {
	if rpcURL == "" {
		ipAddr, err := GetIP()
		if err != nil {
			return err
		}

		rpcURL = fmt.Sprintf("%s%s:%d", "http://", ipAddr, config.DefaultJSONRPCPort)
	}

	parsedURL, err := url.Parse(rpcURL)
	if err != nil {
		return errors.Wrap(err, "invalid rpc url")
	}

	if watchdogURL == "" {
		return errors.New("invalid watch dog url")
	}

	reqParams := make(map[string]interface{})

	req := KramaIDReq{
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

	httpResponse, err := http.Post(watchdogURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return errors.Wrap(err, "failed to register with watchdog")
	}

	if httpResponse.StatusCode >= 200 && httpResponse.StatusCode < 300 {
		return nil
	}

	return errors.Wrap(err, "failed to register with watchdog")
}
