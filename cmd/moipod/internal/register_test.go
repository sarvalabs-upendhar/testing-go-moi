package internal

import (
	"fmt"
	"testing"

	"github.com/sarvalabs/go-moi/moiclient"
	"github.com/stretchr/testify/assert"
)

func TestIsGuardianRegistered(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	client, err := moiclient.NewClient("http://localhost:1600/")
	if err != nil {
		assert.NoError(t, err)
	}

	ok := isGuardianRegistered(
		client,
		"3WxrHRPGwn5HykA6J78BYtkMVMwURiYeGjZmSmtSZXpfdtTAEExX.16Uiu2HAmVfbjVwvwGTJvbZAjdEeuz5fQQ2CpJ3dFbYteXEvYVjBh",
	)
	fmt.Print(ok)
}
