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
	api.router.GET("/logics/:name/storage/:storekey", api.getLogicStorage)

	// Interact APIs
	api.router.POST("/interact/logic/deploy", api.InteractLogicDeploy)
	api.router.POST("/interact/logic/invoke", api.InteractLogicInvoke)
	api.router.POST("/interact/logic/enlist", api.InteractLogicEnlist)

	// Account APIs
	api.router.GET("/accounts/:addr", api.getAccount)
	api.router.GET("/accounts/:addr/storage/:logicID/:storekey", api.getAccountStorage)

	// Event APIs
	api.router.GET("/events", api.getEvents)

	// Start the server on the specified port
	return api.router.Run(fmt.Sprintf(":%d", port))
}

type VersionResponse struct {
	Version string            `json:"version"`
	Engines map[string]string `json:"engines"`
	Website string            `json:"website"`
}

func (api *API) getAPIMetadata(c *gin.Context) {
	engines := make(map[string]string)

	for _, engine := range core.Engines {
		runtime, ok := engineio.FetchEngine(engine)
		if !ok {
			c.JSON(http.StatusInternalServerError, Error(errors.New("failed to fetch engine runtime")))
			return
		}

		engines[engine.String()] = "v" + runtime.Version()
	}

	version := VersionResponse{
		Version: config.ProtocolVersion,
		Engines: engines,
		Website: core.DOCS,
	}

	c.JSON(http.StatusOK, Success().WithData(version))
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
