package internal

import (
	"fmt"
	"testing"

	"github.com/sarvalabs/go-moi/moiclient"
	"github.com/stretchr/testify/assert"
)

func TestIsGuardianRegistered(t *testing.T) {
	client, err := moiclient.NewClient("https://app-voyage.moibit.io/babylon/")
	if err != nil {
		assert.NoError(t, err)
	}

	ok := isGuardianRegistered(
		"3WwV4BpYNzfAQatstXKYV3EfxM32Kmj9wapqSp8ERRgnP1ugN3Ls.16Uiu2HAmFqM8E9v3AAeaMP5cQVLnsD7Ej4PtG7ybfgGBFubj6td7",
		client,
	)
	fmt.Print(ok)
}
