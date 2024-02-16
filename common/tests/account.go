package tests

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sarvalabs/go-moi-identifiers"
)

type AccountWithMnemonic struct {
	Addr     identifiers.Address `json:"address"`
	Mnemonic string              `json:"mnemonic"`
}

func WriteToAccountsFile(filePath string, accounts []AccountWithMnemonic) error {
	file, err := json.MarshalIndent(accounts, "", "\t")
	if err != nil {
		return err
	}

	if err := os.WriteFile(filePath, file, os.ModePerm); err != nil {
		return err
	}

	fmt.Println("Accounts file created")

	return nil
}

func GetAddressFromAccountsFile(filePath string) ([]string, error) {
	accounts := make([]AccountWithMnemonic, 0)
	addresses := make([]string, 0)

	file, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(file, &accounts); err != nil {
		return nil, err
	}

	for index := range accounts {
		addresses = append(addresses, accounts[index].Addr.String())
	}

	return addresses, nil
}
