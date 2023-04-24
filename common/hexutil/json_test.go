package hexutil

import (
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJSONSerializeForBig(t *testing.T) {
	x := (*Big)(big.NewInt(78))

	value, err := json.Marshal(x)
	if err != nil {
		fmt.Println(err)
	}

	y := new(Big)
	if err = json.Unmarshal(value, y); err != nil {
		fmt.Println(err)
	}

	require.Equal(t, x.ToInt(), y.ToInt())
}

func TestJSONSerializeForUint64(t *testing.T) {
	x := Uint64(56)

	value, err := json.Marshal(x)
	if err != nil {
		fmt.Println(err)
	}

	y := new(Uint64)
	if err = json.Unmarshal(value, y); err != nil {
		fmt.Println(err)
	}

	require.Equal(t, x.ToInt(), y.ToInt())
}

func TestJSONSerializeForUint8(t *testing.T) {
	x := Uint8(45)

	value, err := json.Marshal(x)
	if err != nil {
		fmt.Println(err)
	}

	y := new(Uint8)
	if err = json.Unmarshal(value, y); err != nil {
		fmt.Println(err)
	}

	require.Equal(t, x.ToInt(), y.ToInt())
}

func TestJSONSerializeForUint(t *testing.T) {
	x := Uint(100)

	value, err := json.Marshal(x)
	if err != nil {
		fmt.Println(err)
	}

	y := new(Uint)
	if err = json.Unmarshal(value, y); err != nil {
		fmt.Println(err)
	}

	require.Equal(t, x.ToInt(), y.ToInt())
}

func TestJSONSerializeForBytes(t *testing.T) {
	x := Bytes([]byte{0x00, 0x10})

	value, err := json.Marshal(x)
	if err != nil {
		fmt.Println(err)
	}

	y := new(Bytes)
	if err = json.Unmarshal(value, y); err != nil {
		fmt.Println(err)
	}

	require.Equal(t, x.Bytes(), y.Bytes())
}
