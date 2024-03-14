//nolint:nlreturn
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-identifiers"

	"github.com/sarvalabs/go-moi/cmd/logiclab/core"
)

type User struct {
	Username string              `json:"username"`
	Address  identifiers.Address `json:"address,omitempty"`
}

// getUser retrieves a user by their username
func (api *API) getUser(c *gin.Context) {
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

	// Extract the value for username
	username := c.Param("name")
	// Retrieve the user from the env
	userAddr, exists := env.Users[username]
	if !exists {
		c.JSON(http.StatusNotFound, Error(core.ErrUserNotFound))
		return
	}

	c.JSON(http.StatusOK, Success().WithData(User{
		Username: username,
		Address:  userAddr,
	}))
}

// createUser creates a new user based on user input
func (api *API) createUser(c *gin.Context) {
	// Decode the request
	request := new(User)
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

	// Register the user with the environment
	if err = env.RegisterUser(request.Username, request.Address); err != nil {
		// User already exists -> error with conflict code
		if errors.Is(err, core.ErrUserAlreadyExists) {
			c.JSON(http.StatusConflict, Error(err))
			return
		}

		c.JSON(http.StatusInternalServerError, Error(err))
		return //nolint:wsl
	}

	c.JSON(http.StatusCreated, Success().WithData(User{
		Username: request.Username,
		Address:  env.Users[request.Username],
	}))
}

// wipeUser deletes a user based on username
func (api *API) wipeUser(c *gin.Context) {
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

	// Extract the value for username
	username := c.Param("name")
	// Check if the user exists in the environment
	if !env.UserExists(username) {
		c.JSON(http.StatusNotFound, Error(core.ErrUserNotFound))
		return
	}

	// Remove the user from the environment
	if err = env.RemoveUser(username); err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	c.JSON(http.StatusOK, Success())
}

// getAllUsers returns a list of all users
func (api *API) getAllUsers(c *gin.Context) {
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

	c.JSON(http.StatusOK, Success().WithData(env.Users))
}

// purgeUsers deletes all the users
func (api *API) purgeUsers(c *gin.Context) {
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

	// Remove all users from the environment
	if err = env.RemoveAllUsers(); err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	c.JSON(http.StatusOK, Success())
}
