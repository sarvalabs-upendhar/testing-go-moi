package bgclient

import "github.com/sarvalabs/go-moi/common/tests"

type AccountsResponse struct {
	Accounts []tests.AccountWithMnemonic `json:"accounts"`
}

type JSONRpcUrlsResponse struct {
	Urls []string `json:"urls"`
}

type StatusResponse struct {
	Status string `json:"status"`
}
