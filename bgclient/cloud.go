package bgclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/sarvalabs/battleground/network/infrastructure"
	"github.com/sarvalabs/battleground/types"

	"github.com/sarvalabs/go-moi/common/tests"
)

type NetworkType uint8

const (
	UNDEF NetworkType = iota
	LOCAL
	CLOUD
)

type CloudNetwork struct {
	cfg  *Config
	conn *http.Client
}

func NewCloudNetwork(cfg *Config, conn *http.Client) *CloudNetwork {
	return &CloudNetwork{
		cfg:  cfg,
		conn: conn,
	}
}

func (cloud *CloudNetwork) sendHTTPRequest(ctx context.Context, method, endpoint string, body any) ([]byte, error) {
	var reqBody io.Reader

	if body != nil {
		reqJSON, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}

		reqBody = bytes.NewReader(reqJSON)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("X-API-Key", os.Getenv("BG_TOKEN"))

	resp, err := cloud.conn.Do(req)
	if err != nil {
		return nil, err
	}

	defer func(Body io.ReadCloser) { _ = Body.Close() }(resp.Body)

	if resp.StatusCode != http.StatusOK {
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("failed to read response body: %w", readErr)
		}

		return nil, fmt.Errorf("unexpected status code: %d, response body: %s", resp.StatusCode, respBody)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return respBody, nil
}

func (cloud *CloudNetwork) ServerStatus(ctx context.Context) error {
	endpoint := cloud.cfg.EndPoint + "/status"
	_, err := cloud.sendHTTPRequest(ctx, "GET", endpoint, nil)

	return err
}

func (cloud *CloudNetwork) StartNetwork(ctx context.Context) ([]Moipod, error) {
	endpoint := cloud.cfg.EndPoint + "/deploy"
	// Marshal the CloudConfig into JSON
	rawBgConfig, err := json.Marshal(cloud.cfg.CloudCfg)
	if err != nil {
		return nil, err
	}

	requestBody := types.DeploymentParams{
		EnableTracing:      false,
		OperatorSlots:      2,
		ValidatorSlots:     3,
		BattlegroundConfig: rawBgConfig,
	}

	_, err = cloud.sendHTTPRequest(ctx, "POST", endpoint, requestBody)
	if err != nil {
		return nil, err
	}

	err = cloud.waitForDeployment(ctx, 25*time.Minute)
	if err != nil {
		return nil, err
	}

	urls, err := cloud.JSONRpcUrls(ctx)
	if err != nil {
		return nil, err
	}

	moipods := make([]Moipod, len(urls))

	for i := 0; i < len(moipods); i++ {
		moipods[i] = Moipod{
			URL: urls[i],
		}
	}

	return moipods, nil
}

func (cloud *CloudNetwork) NetworkStatus(ctx context.Context) (infrastructure.Status, error) {
	endpoint := cloud.cfg.EndPoint + "/battleground/status"

	resp, err := cloud.sendHTTPRequest(ctx, "GET", endpoint, types.StatusParams{})
	if err != nil {
		return 0, err
	}

	var statusResponse StatusResponse
	if err := json.Unmarshal(resp, &statusResponse); err != nil {
		return 0, err
	}

	switch statusResponse.Status {
	case "Active":
		return infrastructure.Active, nil
	case "Pending":
		return infrastructure.Pending, nil
	case "Failed":
		return infrastructure.Failed, nil
	default:
		return infrastructure.Inactive, nil
	}
}

func (cloud *CloudNetwork) waitForDeployment(ctx context.Context, timeout time.Duration) (err error) {
	var status infrastructure.Status

	retryFunc := func() error {
		status, err = cloud.NetworkStatus(ctx)
		if err != nil {
			return err
		}

		if status == 1 {
			return errors.New("deployment in progress")
		}

		return nil
	}

	startTime := time.Now().UTC()
	// Retry loop with timeout
	for {
		if err := retryFunc(); err == nil {
			break // Success, break out of the loop
		}

		// Check if the timeout has been reached
		elapsed := time.Since(startTime)
		if elapsed >= timeout {
			return errors.Wrap(err, "timeout reached while retrying")
		}

		// Wait for a short duration before retrying
		time.Sleep(1 * time.Second)
	}

	return nil
}

func (cloud *CloudNetwork) waitForDestruction(ctx context.Context, timeout time.Duration) (err error) {
	var status infrastructure.Status

	retryFunc := func() error {
		status, err = cloud.NetworkStatus(ctx)
		if err != nil {
			return err
		}

		if status != 0 {
			return errors.New("destruction in progress")
		}

		return nil
	}

	startTime := time.Now().UTC()
	// Retry loop with timeout
	for {
		if err := retryFunc(); err == nil {
			break // Success, break out of the loop
		}

		// Check if the timeout has been reached
		elapsed := time.Since(startTime)
		if elapsed >= timeout {
			return errors.Wrap(err, "timeout reached while retrying")
		}

		// Wait for a short duration before retrying
		time.Sleep(1 * time.Second)
	}

	return nil
}

func (cloud *CloudNetwork) UpdateNetwork(ctx context.Context, commitHash string, cleanDB bool, logLevel string) error {
	endpoint := cloud.cfg.EndPoint + "/update"
	requestBody := types.UpdateParams{
		MoichainSourceRef: commitHash,
		CleanDB:           cleanDB,
		LogLevel:          logLevel,
	}

	_, err := cloud.sendHTTPRequest(ctx, "POST", endpoint, requestBody)
	if err != nil {
		return err
	}

	return nil
}

func (cloud *CloudNetwork) Accounts(ctx context.Context) ([]tests.AccountWithMnemonic, error) {
	var accountsResponse AccountsResponse

	endpoint := cloud.cfg.EndPoint + "/genesis/accounts"

	resp, err := cloud.sendHTTPRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(resp, &accountsResponse); err != nil {
		return nil, err
	}

	return accountsResponse.Accounts, err
}

func (cloud *CloudNetwork) JSONRpcUrls(ctx context.Context) ([]string, error) {
	endpoint := cloud.cfg.EndPoint + "/rpcurls"

	resp, err := cloud.sendHTTPRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var response JSONRpcUrlsResponse
	if err := json.Unmarshal(resp, &response); err != nil {
		return nil, err
	}

	return response.Urls, nil
}

func (cloud *CloudNetwork) DestroyNetwork(ctx context.Context, removeFolders bool) error {
	endpoint := cloud.cfg.EndPoint + "/destroy"

	_, err := cloud.sendHTTPRequest(ctx, "POST", endpoint, types.DestroyParams{})
	if err != nil {
		return err
	}

	err = cloud.waitForDestruction(ctx, 10*time.Minute)

	return err
}

func (cloud *CloudNetwork) StartNode(ctx context.Context, rpcAddr string, cleanDB bool) error {
	endpoint := cloud.cfg.EndPoint + "/start/node"

	_, err := cloud.sendHTTPRequest(ctx, "POST", endpoint, types.MoipodParams{RpcAddr: rpcAddr, CleanDB: cleanDB})
	if err != nil {
		return err
	}

	return err
}

func (cloud *CloudNetwork) StopNode(ctx context.Context, rpcAddr string) error {
	endpoint := cloud.cfg.EndPoint + "/stop/node"

	_, err := cloud.sendHTTPRequest(ctx, "POST", endpoint, types.MoipodParams{RpcAddr: rpcAddr})
	if err != nil {
		return err
	}

	return err
}
