//nolint:nlreturn
package api

import (
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/cmd/logiclab/core"
	"github.com/sarvalabs/go-moi/cmd/logiclab/db"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
)

type Logic struct {
	Name    string `json:"name"`
	Ready   bool   `json:"ready"`
	Edition uint64 `json:"edition"`
	Engine  string `json:"engine"`

	LogicID  string `json:"logic_id"`
	Address  string `json:"address"`
	Manifest string `json:"manifest"`

	Callsites map[string]core.LogicCallsite `json:"callsites"`
}

type Manifest struct {
	Encoding string `json:"encoding"`
	Content  string `json:"content"`
}

// getLogic retrieves a logic by the logic name
func (api *API) getLogic(c *gin.Context) {
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

	// Extract the value for logic name
	logicName := c.Param("name")
	// Retrieve the logic from the environment
	logic, err := env.FetchLogic(logicName)
	if err != nil {
		if errors.Is(err, core.ErrLogicNotFound) {
			c.JSON(http.StatusNotFound, Error(err))
			return
		}

		c.JSON(http.StatusInternalServerError, Error(err))
		return //nolint:wsl
	}

	// Obtain the identifier of the logic ID
	identifier, _ := logic.Object.ID.Identifier()
	manifest := logic.Object.ManifestHash()

	c.JSON(http.StatusOK, Success().WithData(Logic{
		Name:    logic.Name,
		Ready:   logic.Ready,
		Edition: identifier.Edition(),
		Engine:  logic.Object.EngineKind.String(),

		LogicID:  logic.Object.ID.String(),
		Address:  logic.Object.ID.Address().String(),
		Manifest: hex.EncodeToString(manifest[:]),

		Callsites: logic.Callsites,
	}))
}

type CreateLogicRequest struct {
	Name     string   `json:"name"`
	Manifest Manifest `json:"manifest"`
}

type CreateLogicResponse struct {
	LogicID  string `json:"logic_id"`
	Consumed uint64 `json:"consumed"`
}

// compileLogic creates a new logic based on logic input
func (api *API) createLogic(c *gin.Context) {
	// Decode the request
	request := new(CreateLogicRequest)
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

	// Check if a logic with name already exists
	if env.LogicExists(request.Name) {
		c.JSON(http.StatusConflict, Error(core.ErrLogicAlreadyExists))
		return
	}

	// Decode the manifest from the given format
	manifest, err := engineio.NewManifest(
		common.Hex2Bytes(request.Manifest.Content),
		common.EncodingFromString(request.Manifest.Encoding),
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, Error(errors.Wrap(err, "malformed manifest")))
		return
	}

	// Compile the manifest into a Logic
	logic, fuel, err := core.NewLogic(request.Name, manifest, env.CallFuel)
	if err != nil {
		c.JSON(http.StatusBadRequest, Error(errors.Wrap(err, "invalid manifest")))
		return
	}

	// Register the logic with the environment
	if err = env.RegisterLogic(logic, manifest); err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	c.JSON(http.StatusCreated, Success().WithData(CreateLogicResponse{
		LogicID:  logic.Object.ID.String(),
		Consumed: fuel,
	}))
}

// wipeLogic deletes a logic based on the logic name
func (api *API) wipeLogic(c *gin.Context) {
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

	// Extract the value for name of logic
	logicName := c.Param("name")
	// Check if a logic with name exists
	if !env.LogicExists(logicName) {
		c.JSON(http.StatusFound, Error(core.ErrLogicNotFound))
		return
	}

	// Remove the logic from the environment
	if err = env.RemoveLogic(logicName); err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	c.JSON(http.StatusOK, Success())
}

// getAllLogics returns a list of all logics
func (api *API) getAllLogics(c *gin.Context) {
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

	c.JSON(http.StatusOK, Success().WithData(env.Logics))
}

// purgeLogics deletes all the logics
func (api *API) purgeLogics(c *gin.Context) {
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

	// Remove all logics from the environment
	if err = env.RemoveAllLogics(); err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	c.JSON(http.StatusOK, Success())
}

func (api *API) getLogicManifest(c *gin.Context) {
	// Get the environment ID
	envID := c.GetHeader(HeaderLabEnv)
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

	// Extract the value for name of logic
	logicName := c.Param("name")
	// Get the logic ID and check if the logic exists
	logicID, ok := env.Logics[logicName]
	if !ok {
		c.JSON(http.StatusNotFound, Error(core.ErrLogicNotFound))
		return
	}

	// Get the logic manifest for the given name
	value, err := api.lab.Database.Get(db.LogicManifestKey(env.ID, logicID))
	if errors.Is(err, db.ErrKeyNotFound) {
		c.JSON(http.StatusNotFound, Error(errors.New("unable to find logic manifest")))
		return
	}

	// Decode the value into a manifest
	manifest, err := engineio.NewManifest(value, common.POLO)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Error(errors.Wrap(err, "malformed manifest")))
		return
	}

	// Extract the value for encoding
	encoding := c.Param("encoding")

	// Convert the manifest into expected format
	target := common.EncodingFromString(strings.ToUpper(encoding))
	converted := core.PrintManifest(manifest, target)

	c.JSON(http.StatusOK, Success().WithData(Manifest{
		Encoding: strings.ToUpper(encoding),
		Content:  converted,
	}))
}

type LogicStorageValue struct {
	Key string `json:"key"`
	Val string `json:"val"`
}

func (api *API) getLogicStorage(c *gin.Context) {
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

	// Extract the value for name of logic
	logicName := c.Param("name")
	// Get the logic ID of the logic
	logicID, exists := env.Logics[logicName]
	if !exists {
		c.JSON(http.StatusNotFound, Error(core.ErrLogicNotFound))
		return
	}

	// Extract the storage key
	storekey := c.Param("storekey")
	// Generate the db key for the storage key
	dbkey := db.StorageKey(env.ID, logicID.Address(), logicID, common.Hex2Bytes(storekey))

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
