//nolint:nlreturn,wsl
package api

import (
	"net/http"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/pkg/errors"

	"github.com/gin-gonic/gin"
	"github.com/sarvalabs/go-moi/cmd/logiclab/core"
)

type DefaultFuel struct {
	Fuel uint64 `json:"fuel"`
}

type DefaultUser struct {
	Username string `json:"username"`
}

func (api *API) setDefaultFuelAmount(c *gin.Context) {
	request := new(DefaultFuel)
	// Call BindJSON to bind the received JSON to newUser
	if err := c.ShouldBindJSON(request); err != nil {
		c.JSON(http.StatusBadRequest, Error(err))
		return
	}

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

	// Update the environment
	env.CallFuel = request.Fuel

	c.JSON(http.StatusOK, Success())
}

func (api *API) getDefaultFuelAmount(c *gin.Context) {
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

	c.JSON(http.StatusOK, Success().WithData(DefaultFuel{Fuel: env.CallFuel}))
}

func (api *API) setDefaultSender(c *gin.Context) {
	// Decode the request
	request := new(DefaultUser)
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, Error(err))
		return
	}

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

	// Register the user with the environment
	if err = env.SetDefaultSender(request.Username); err != nil {
		// User not found -> error with status not found code
		if errors.Is(err, core.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, Error(core.ErrUserNotFound))
			return
		}

		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	c.JSON(http.StatusOK, Success())
}

func (api *API) getDefaultSender(c *gin.Context) {
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

	if env.Sender == "" {
		c.JSON(http.StatusNotFound, Error(core.ErrSenderNotConf).WithData(User{
			Username: "",
			ID:       identifiers.Nil,
		}))

		return
	}

	// Get the sender
	sender := env.Sender
	// Get the sender id
	id := env.Users[sender]

	c.JSON(http.StatusOK, Success().WithData(User{
		Username: sender,
		ID:       id,
	}))
}

func (api *API) wipeDefaultSender(c *gin.Context) {
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

	// Unset the default sender
	env.Sender = ""

	c.JSON(http.StatusOK, Success())
}
