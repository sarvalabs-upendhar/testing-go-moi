//nolint:nlreturn
package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/cmd/logiclab/core"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/compute/engineio"
)

const HeaderLabEnv = "X-LogicLab-Environment"

type API struct {
	lab    *core.Lab
	router *gin.Engine
}

func NewAPI(lab *core.Lab) *API {
	return &API{
		lab:    lab,
		router: gin.Default(),
	}
}

func (api *API) Start(port int) error {
	// Basic API Primitives
	api.router.GET("/", api.getAPIMetadata)
	api.router.DELETE("/", api.resetLabDB)
	api.router.GET("/engines", api.getEngineRuntimes)

	// Environment APIs
	api.router.GET("/environments", api.getAllEnvironments)
	api.router.POST("/environments", api.createEnvironment)
	api.router.GET("/environments/:id", api.getEnvironment)
	api.router.DELETE("/environments/:id", api.deleteEnvironment)

	// Default Value Management APIs
	api.router.POST("/defaults/fuel", api.setDefaultFuelAmount)
	api.router.GET("/defaults/fuel", api.getDefaultFuelAmount)
	api.router.POST("/defaults/sender", api.setDefaultSender)
	api.router.GET("/defaults/sender", api.getDefaultSender)
	api.router.DELETE("/defaults/sender", api.wipeDefaultSender)
	api.router.POST("/defaults/receiver", api.setDefaultReceiver)
	api.router.GET("/defaults/receiver", api.getDefaultReceiver)
	api.router.DELETE("/defaults/receiver", api.wipeDefaultReceiver)

	// User Management APIs
	api.router.GET("/users", api.getAllUsers)
	api.router.POST("/users", api.createUser)
	api.router.DELETE("/users", api.purgeUsers)
	api.router.GET("/users/:name", api.getUser)
	api.router.DELETE("/users/:name", api.wipeUser)

	// Logic Management APIs
	api.router.GET("/logics", api.getAllLogics)
	api.router.POST("/logics", api.createLogic)
	api.router.DELETE("/logics", api.purgeLogics)
	api.router.GET("/logics/:name", api.getLogic)
	api.router.DELETE("/logics/:name", api.wipeLogic)
	api.router.GET("/logics/:name/manifest", api.getLogicManifest)
	api.router.GET("/logics/:name/manifest/:encoding", api.getEncodedLogicManifest)

	// Logic & Engine Utilities APIs
	api.router.POST("/errdecode/:engine", api.decodeErrorData)
	api.router.POST("/storagekey/:engine", api.generateStorageKey)
	api.router.POST("/convert/codeform/:format", api.convertManifestCodeform)
	api.router.POST("/convert/fileform/:encoding", api.convertManifestFileform)

	// Logic APIs
	api.router.GET("/logics/:name/state/:storekey", api.getLogicStorage)
	api.router.POST("/logics/:name/call/:endpoint", api.callLogicEndpoint)

	// Account APIs
	api.router.GET("accounts/:addr", api.getAccount)

	// Event APIs
	api.router.GET("/events", api.getEvents)

	// Start the server on the specified port
	return api.router.Run(fmt.Sprintf(":%d", port))
}

type VersionResponse struct {
	Version string `json:"version"`
	Website string `json:"website"`
}

func (api *API) getAPIMetadata(c *gin.Context) {
	version := VersionResponse{
		Version: config.ProtocolVersion,
		Website: core.DOCS,
	}

	c.JSON(http.StatusOK, Success().WithData(version))
}

func (api *API) getEngineRuntimes(c *gin.Context) {
	engines := make(map[string]string)

	for _, engine := range core.Engines {
		runtime, ok := engineio.FetchEngine(engine)
		if !ok {
			c.JSON(http.StatusInternalServerError, Error(errors.New("failed to fetch engine runtime")))
			return
		}

		engines[engine.String()] = "v" + runtime.Version()
	}

	c.JSON(http.StatusOK, Success().WithData(engines))
}

func (api *API) resetLabDB(c *gin.Context) {
	if err := api.lab.Database.DropAll(); err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	c.JSON(http.StatusOK, Success())
}

func Success() Response {
	return Response{Status: "success"}
}

func Error(err error) Response {
	return Response{Status: "error", Message: err.Error()}
}

type Response struct {
	Status  string      `json:"status"`
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
}

func (response Response) WithData(data any) Response {
	response.Data = data
	return response
}
