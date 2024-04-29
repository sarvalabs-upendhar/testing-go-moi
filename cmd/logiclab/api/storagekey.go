//nolint:nlreturn
package api

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/sarvalabs/go-moi/compute/pisa"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/cmd/logiclab/core"
)

type AccessorKind int

const (
	InvalidAccessor AccessorKind = iota
	ArrayIndexAccessor
	MapKeyAccessor
	ClassFieldAccessor
)

var accessorKindToString = map[AccessorKind]string{
	ArrayIndexAccessor: "arridx",
	MapKeyAccessor:     "mapkey",
	ClassFieldAccessor: "clsfld",
}

var accessorKindFromString = map[string]AccessorKind{
	"arridx": ArrayIndexAccessor,
	"mapkey": MapKeyAccessor,
	"clsfld": ClassFieldAccessor,
}

func (kind AccessorKind) String() string {
	str, ok := accessorKindToString[kind]
	if !ok {
		panic("invalid accessor kind")
	}

	return str
}

func newAccessorKindFromString(str string) AccessorKind {
	kind, ok := accessorKindFromString[str]
	if !ok {
		return InvalidAccessor
	}

	return kind
}

func (kind AccessorKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(kind.String())
}

func (kind *AccessorKind) UnmarshalJSON(data []byte) (err error) {
	str := new(string)
	if err = json.Unmarshal(data, str); err != nil {
		return err
	}

	*kind = newAccessorKindFromString(*str)
	if *kind == InvalidAccessor {
		return errors.New(fmt.Sprintf("invalid accessor kind: %s", *str))
	}

	return nil
}

type Accessor struct {
	Kind  AccessorKind `json:"kind"`
	Value string       `json:"value"`
}

type StorageKeyRequest struct {
	Slot      uint8      `json:"slot"`
	Accessors []Accessor `json:"accessors"`
}

type StorageKeyResponse struct {
	Hash string `json:"hash"`
}

func (api *API) generateStorageKey(c *gin.Context) {
	// Decode the request
	request := new(StorageKeyRequest)
	if err := c.ShouldBindJSON(request); err != nil {
		c.JSON(http.StatusBadRequest, Error(err))
		return
	}

	// Check slot value bounds // todo: this check might not be necessary
	if request.Slot == math.MaxUint8 {
		c.JSON(http.StatusBadRequest, Error(errors.New("slot number is too large")))
		return
	}

	storageKeys := make([]pisa.Accessor, 0)

	accessors := request.Accessors
	for i := 0; i < len(accessors); i++ {
		kind := accessors[i].Kind

		value, err := hex.DecodeString(accessors[i].Value)
		if err != nil {
			c.JSON(http.StatusBadRequest, Error(errors.New("failed to decode accessor value")))
			return
		}

		// Use a switch statement to handle different kinds of accessors
		switch kind {
		case ArrayIndexAccessor:
			if len(value) != 8 {
				c.JSON(http.StatusBadRequest, Error(errors.New("invalid length of array index accessor")))
				return
			}

			// Convert the value into uint64
			arrayIndex, err := strconv.ParseUint(hex.EncodeToString(value), 16, 64)
			if err != nil {
				c.JSON(http.StatusBadRequest, Error(errors.New("failed to convert into array index accessor")))
				return
			}

			// Append element to storage key
			storageKeys = append(storageKeys, pisa.ArrIdx(arrayIndex))

		case MapKeyAccessor:
			if len(value) != 32 {
				c.JSON(http.StatusBadRequest, Error(errors.New("invalid length of map key accessor")))
				return
			}

			// Convert the value into a [32]byte array
			var mapKey [32]byte

			copy(mapKey[:], value)

			// Append element to storage key
			storageKeys = append(storageKeys, pisa.MapKey(mapKey))

		case ClassFieldAccessor:
			if len(value) != 1 {
				c.JSON(http.StatusBadRequest, Error(errors.New("invalid length of class field accessor")))
				return
			}

			// Convert the value into uint8
			classField, err := strconv.ParseUint(hex.EncodeToString(value), 16, 64)
			if err != nil {
				c.JSON(http.StatusBadRequest, Error(errors.New("failed to convert into class field accessor")))
				return
			}

			// Append element to storage key
			storageKeys = append(storageKeys, pisa.ClsFld(uint8(classField)))

		default:
			c.JSON(http.StatusBadRequest, Error(errors.New("invalid accessor kind")))
			return
		}
	}

	// Extract the engine kind from the path
	engine := c.Param("engine")
	if strings.ToUpper(engine) != "PISA" {
		c.JSON(http.StatusBadRequest, Error(core.ErrUnsupportedEngine))
		return
	}

	// Get the storage key
	key := pisa.GenerateStorageKey(request.Slot, storageKeys...)

	c.JSON(http.StatusOK, Success().WithData(StorageKeyResponse{
		Hash: hex.EncodeToString(key),
	}))
}
