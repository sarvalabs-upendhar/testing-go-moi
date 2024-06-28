//nolint:nlreturn
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	identifiers "github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/cmd/logiclab/core"
)

type EnvironmentRequest struct {
	Name string `json:"name"`
}

type EnvironmentListResponse struct {
	Environments []string `json:"environments"`
}

type EnvironmentInventoryResponse struct {
	Name     string                         `json:"name"`
	Users    map[string]identifiers.Address `json:"users"`
	Logics   map[string]core.LogicMetadata  `json:"logics"`
	Defaults EnvironmentDefaults            `json:"defaults"`
}

type EnvironmentDefaults struct {
	Sender string `json:"sender"`
	Fuel   uint64 `json:"fuel"`
}

func (api *API) createEnvironment(c *gin.Context) {
	// Decode the request
	request := new(EnvironmentRequest)
	if err := c.ShouldBindJSON(request); err != nil {
		c.JSON(http.StatusBadRequest, Error(err))
		return
	}

	if err := api.lab.AddEnvironment(request.Name); err != nil {
		if errors.Is(err, core.ErrEnvAlreadyExists) {
			c.JSON(http.StatusConflict, Error(core.ErrEnvAlreadyExists))
			return
		}

		c.JSON(http.StatusInternalServerError, Error(err))
		return //nolint:wsl
	}

	c.JSON(http.StatusCreated, Success())
}

func (api *API) getAllEnvironments(c *gin.Context) {
	// Get all the environments
	environments, err := api.lab.GetAllEnvironments()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	c.JSON(http.StatusOK, Success().WithData(EnvironmentListResponse{Environments: environments}))
}

func (api *API) getEnvironment(c *gin.Context) {
	// Extract the value for environment ID
	envID := c.Param("id")
	// Retrieve the environment
	env, exists, err := api.lab.GetEnvironment(envID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	// Environment was not found
	if !exists {
		c.JSON(http.StatusNotFound, Error(core.ErrEnvironmentNotFound))
		return
	}

	c.JSON(http.StatusOK, Success().WithData(EnvironmentInventoryResponse{
		Name:   env.ID,
		Users:  env.Users,
		Logics: env.Logics,
		Defaults: EnvironmentDefaults{
			Sender: env.Sender,
			Fuel:   env.CallFuel,
		},
	}))
}

func (api *API) deleteEnvironment(c *gin.Context) {
	// Extract the value for environment ID
	env := c.Param("id")
	// Delete the environment in totality
	if err := api.lab.DelEnvironment(env); err != nil {
		if errors.Is(err, core.ErrEnvironmentNotFound) {
			c.JSON(http.StatusNotFound, Error(core.ErrEnvironmentNotFound))
			return
		}

		c.JSON(http.StatusInternalServerError, Error(err))
		return //nolint:wsl
	}

	c.JSON(http.StatusOK, Success())
}
