//nolint:nlreturn
package api

import (
	"context"
	"encoding/hex"
	"math/big"
	"net/http"
	"strings"

	"github.com/sarvalabs/go-moi/compute"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/cmd/logiclab/core"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
)

type LogicCallRequest struct {
	Callsite string `json:"callsite"`
	Calldata string `json:"calldata,omitempty"`
}

type Result struct {
	Ok     bool   `json:"ok"`
	Fuel   uint64 `json:"fuel"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error"`
}

type LogicCallResponse struct {
	Ixhash common.Hash  `json:"ixhash"`
	Result Result       `json:"result"`
	Events []core.Event `json:"events"`
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
	if kind == engineio.CallsiteInvokable && !logic.Ready {
		c.JSON(http.StatusBadRequest, Error(errors.New("logic is not ready. must be deployed first")))
		return
	} else if kind == engineio.CallsiteDeployer && logic.Ready {
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

	// Obtain the engine for the logic engine in the header
	engine, ok := engineio.FetchEngine(logic.Object.Engine())
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
	eventstream := compute.NewEventStream(logicID)

	// Spawn an instance for the engine
	instance, err := engine.SpawnInstance(
		logic.Object,
		env.CallFuel,
		core.NewContextDriver(env.ID, api.lab.Database, logicID.Address(), logicID),
		api.lab,
		eventstream,
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
			case engineio.CallsiteDeployer:
				return common.IxLogicDeploy
			case engineio.CallsiteInvokable:
				return common.IxLogicInvoke
			default:
				panic("unhandled logic call case")
			}
		}(),
		Nonce: env.Nonce,
		Price: new(big.Int).SetUint64(core.LabFuelPrice),
		Limit: env.CallFuel,
		Site:  request.Callsite,
	}

	env.IncrementNonce()

	// Set the calldata as nil if no calldata is provided
	if request.Calldata == "" {
		ixn.Call = nil
	} else {
		ixn.Call = calldata
	}

	// Validate the calldata
	err = engine.ValidateCalldata(logic.Object, ixn)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	// Execute the function
	result, err := instance.Call(context.Background(), ixn, senderContext)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	// Get the logic interaction hash
	hash, err := ixn.Hash()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	// Get the events of core.Event type
	events := core.GetEventsFromStream(eventstream, hash)

	err = env.InsertEvent(events)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	if kind == engineio.CallsiteDeployer {
		// Mark the logic as deployed
		logic.Ready = true

		// Convert error into hex string
		var err string
		if result.Error() != nil {
			err = "0x" + hex.EncodeToString(result.Error())
		}

		c.JSON(http.StatusOK, Success().WithData(LogicCallResponse{
			Ixhash: hash,
			Result: Result{
				Ok:    result.Ok(),
				Fuel:  result.Fuel(),
				Error: err,
			},
			Events: events,
		}))

		return
	} else if kind == engineio.CallsiteInvokable {
		// Convert output into hex string
		var output string
		if result.Outputs() != nil {
			output = "0x" + hex.EncodeToString(result.Outputs())
		}

		// Convert error into hex string
		var err string
		if result.Error() != nil {
			err = "0x" + hex.EncodeToString(result.Error())
		}

		c.JSON(http.StatusOK, Success().WithData(LogicCallResponse{
			Ixhash: hash,
			Result: Result{
				Ok:     result.Ok(),
				Fuel:   result.Fuel(),
				Output: output,
				Error:  err,
			},
			Events: events,
		}))

		return
	}
}
