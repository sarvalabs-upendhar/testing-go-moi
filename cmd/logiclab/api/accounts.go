//nolint:nlreturn
package api

import (
	"net/http"

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
