package tests

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sarvalabs/go-moi/common/identifiers"
)

type AccountWithMnemonic struct {
	ID        identifiers.Identifier `json:"id"`
	Mnemonic  string                 `json:"mnemonic"`
	PublicKey []byte                 `json:"public_key"`
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

func GetAccountsFromFile(filePath string) ([]AccountWithMnemonic, error) {
	accounts := make([]AccountWithMnemonic, 0)

	file, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(file, &accounts); err != nil {
		return nil, err
	}

	return accounts, nil
}
