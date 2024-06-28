//nolint:nlreturn
package api

import (
	"encoding/hex"
	"net/http"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi/cmd/logiclab/db"
	"github.com/sarvalabs/go-moi/common"

	"github.com/gin-gonic/gin"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/cmd/logiclab/core"
)

type AccountLookupResponse struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

func (api *API) getAccount(c *gin.Context) {
	// Retrieve the environment
	env, exists, err := api.lab.GetEnvironment(c.GetHeader(HeaderLabEnv))
	if err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	// Environment was not found
	if !exists {
		c.JSON(http.StatusNotFound, Error(core.ErrEnvironmentNotFound))
		return
	}

	// Extract the address
	addr := c.Param("addr")

	// Generate identifiers.Address from addr
	address, err := identifiers.NewAddressFromHex(addr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	// Retrieve the account kind and name
	kind, name := env.LookupAccount(address)

	c.JSON(http.StatusOK, Success().WithData(AccountLookupResponse{
		Kind: kind.String(),
		Name: name,
	}))
}

func (api *API) getAccountStorage(c *gin.Context) {
	// Retrieve the environment
	env, exists, err := api.lab.GetEnvironment(c.GetHeader(HeaderLabEnv))
	if err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	// Environment was not found
	if !exists {
		c.JSON(http.StatusNotFound, Error(core.ErrEnvironmentNotFound))
		return
	}

	// Extract the address
	addr := c.Param("addr")

	// Generate identifiers.Address from addr
	address, err := identifiers.NewAddressFromHex(addr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	if !env.AddrExists(address) {
		c.JSON(http.StatusNotFound, Error(core.ErrAddrNotFound))
		return
	}

	// Extract the value for name of logic
	logic := c.Param("logicID")

	// Convert to logicID type
	logicID, err := identifiers.NewLogicID(logic)
	if err != nil {
		c.JSON(http.StatusBadRequest, Error(err))
		return
	}

	// Extract the storage key
	storekey := c.Param("storekey")
	// Generate the db key for the storage key
	dbkey := db.StorageKey(env.ID, address, logicID, common.Hex2Bytes(storekey))

	// Get the logic state for the given name
	storeval, err := api.lab.Database.Get(dbkey)
	if errors.Is(err, db.ErrKeyNotFound) {
		c.JSON(http.StatusNotFound, Error(core.ErrStorageValueNotFound))
		return
	}

	c.JSON(http.StatusOK, Success().WithData(LogicStorageValue{
		Key: storekey,
		Val: hex.EncodeToString(storeval),
	}))
}
