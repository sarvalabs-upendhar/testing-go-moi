//nolint:nlreturn
package api

import (
	"context"
	"encoding/hex"
	"math/big"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-moi-engineio"

	"github.com/sarvalabs/go-moi/cmd/logiclab/core"
	"github.com/sarvalabs/go-moi/common"
)

type LogicCallRequest struct {
	Callsite string `json:"callsite"`
	Calldata string `json:"calldata,omitempty"`
}

type LogicCallResponse struct {
	Ok     bool
	Fuel   uint64
	Output []byte
	Error  []byte
}

func (api *API) callLogicEndpoint(c *gin.Context) {
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

	// Extract the kind of endpoint
	endpoint := c.Param("endpoint")
	kind := core.EndpointFromString(strings.ToUpper(endpoint))

	// Perform deploy gating
	// Only allow to deploy, if logic is not ready.
	// Only allow to invoke, if logic is ready.
	if kind == engineio.InvokableCallsite && !logic.Ready {
		c.JSON(http.StatusBadRequest, Error(errors.New("logic is not ready. must be deployed first")))
		return
	} else if kind == engineio.DeployerCallsite && logic.Ready {
		c.JSON(http.StatusBadRequest, Error(errors.New("logic is already deployed")))
		return
	}

	request := new(LogicCallRequest)
	// Call BindJSON to bind the received JSON to logicCall
	if err = c.ShouldBindJSON(request); err != nil {
		c.JSON(http.StatusBadRequest, Error(err))
		return
	}

	// Get the callsite from the logic, error if not found
	callsite, ok := logic.Object.GetCallsite(request.Callsite)
	if !ok {
		c.JSON(http.StatusNotFound, Error(errors.New("callsite not found on logic")))
		return
	}

	// Check that call kind matches for the callsite
	if callsite.Kind != kind {
		c.JSON(http.StatusBadRequest, Error(errors.New("callsite kind does not match")))
		return
	}

	// Obtain the runtime for the logic engine in the header
	runtime, ok := engineio.FetchEngineRuntime(logic.Object.Engine())
	if !ok {
		c.JSON(http.StatusInternalServerError, Error(errors.New("failed to retrieve runtime for logic")))
		return
	}

	// Decode hex string into bytes
	calldata, err := hex.DecodeString(strings.TrimPrefix(request.Calldata, "0x"))
	if err != nil {
		c.JSON(http.StatusBadRequest, Error(err))
		return
	}

	logicID := logic.Object.ID
	// Spawn an engine for the runtime
	engine, err := runtime.SpawnEngine(
		env.CallFuel,
		logic.Object,
		core.NewContextDriver(env.ID, api.lab.Database, logicID.Address(), logicID),
		api.lab,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Error(errors.Wrap(err, "failed to spawn engine")))
		return
	}

	if env.Sender == "" {
		c.JSON(http.StatusNotFound, Error(core.ErrSenderNotConf))
		return
	}

	// Get the address of the sender
	senderAddress := env.Users[env.Sender]
	// Generate the context object for the sender
	senderContext := core.NewContextDriver(env.ID, api.lab.Database, senderAddress, logicID)

	// Generate an interaction from the kind, callsite, calldata and manifest
	ixn := core.LogicInteraction{
		Kind: func() common.IxType {
			switch kind {
			case engineio.DeployerCallsite:
				return common.IxLogicDeploy
			case engineio.InvokableCallsite:
				return common.IxLogicInvoke
			default:
				panic("unhandled logic call case")
			}
		}(),
		Price: new(big.Int).SetUint64(core.LabFuelPrice),
		Limit: env.CallFuel,
		Site:  request.Callsite,
		Call:  calldata,
	}

	// Execute the function
	result, err := engine.Call(context.Background(), ixn, senderContext)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	if kind == engineio.DeployerCallsite && result.Ok() {
		// Mark the logic as deployed
		logic.Ready = true

		c.JSON(http.StatusOK, Success().WithData(LogicCallResponse{
			Ok:    result.Ok(),
			Fuel:  result.Fuel(),
			Error: result.Error(),
		}))

		return
	} else if kind == engineio.InvokableCallsite {
		c.JSON(http.StatusOK, Success().WithData(LogicCallResponse{
			Ok:     result.Ok(),
			Fuel:   result.Fuel(),
			Output: result.Outputs(),
			Error:  result.Error(),
		}))

		return
	}
}
