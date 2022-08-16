package moinode

import (
	"fmt"
	"testing"

	"gitlab.com/sarvalabs/moichain/mudra/kramaid"
)

func TestMoiNodeRegistry_GetNodePublicKey(t *testing.T) {
	mnRgstry := Init("https://devapi.moinet.io")

	/* Initializing the MoiNode Registry */
	kid := kramaid.KramaID("a69etUh4e5P6uTVU6FUmryMmuJYjsEqJv6KHnG1kEZLZhZN2zj." +
		"16Uiu2HAmVN1PQfL7nNbvo8igGGo4YKwZqynPkjYrAXWevK613bd7")

	fmt.Println(mnRgstry.GetNodePublicKey(kid))
}
