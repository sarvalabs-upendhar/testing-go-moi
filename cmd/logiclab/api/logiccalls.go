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
	Name     string `json:"name"`
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

type callDetails struct {
	code  int
	env   *core.Environment
	req   *LogicCallRequest
	logic *core.Logic
}

func (api *API) obtainCallDetails(header string, request *LogicCallRequest) (callDetails, error) {
	// Retrieve the environment
	env, exists, err := api.lab.GetEnvironment(header)
	if err != nil {
		return callDetails{code: http.StatusInternalServerError}, err
	}

	// Environment was not found
	if !exists {
		return callDetails{code: http.StatusNotFound}, core.ErrEnvironmentNotFound
	}

	// Extract the value for name of logic
	logicName := request.Name
	if logicName == "" {
		return callDetails{code: http.StatusNotFound}, errors.New("logic not found")
	}

	// Retrieve the logic from the environment
	logic, err := env.FetchLogic(logicName)
	if err != nil {
		if errors.Is(err, core.ErrLogicNotFound) {
			return callDetails{code: http.StatusNotFound}, err
		}

		return callDetails{code: http.StatusInternalServerError}, err
	}

	return callDetails{
		code: http.StatusOK,
		env:  env, req: request, logic: logic,
	}, nil
}

func (api *API) InteractLogicDeploy(c *gin.Context) {
	request := new(LogicCallRequest)
	// Call BindJSON to bind the received JSON to logicCall
	if err := c.ShouldBindJSON(request); err != nil {
		c.JSON(http.StatusBadRequest, Error(err))
	}

	call, err := api.obtainCallDetails(c.GetHeader(HeaderLabEnv), request)
	if err != nil {
		c.JSON(call.code, Error(err))
		return
	}

	env := call.env
	logic := call.logic

	// Perform deploy gating
	// Only allow to deploy, if logic is not ready.
	if logic.Ready {
		c.JSON(http.StatusBadRequest, Error(errors.New("logic is already deployed")))
		return
	}

	// Get the callsite from the logic, error if not found
	callsite, ok := logic.Object.GetCallsite(request.Callsite)
	if !ok {
		c.JSON(http.StatusNotFound, Error(errors.New("callsite not found on logic")))
		return
	}

	// Check that call kind matches for the callsite
	if callsite.Kind != engineio.CallsiteDeploy {
		c.JSON(http.StatusBadRequest, Error(errors.New("callsite is not deployable")))
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
		core.NewStorageDriver(env.ID, api.lab.Database, logicID.Address(), logicID),
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
	senderContext := core.NewStorageDriver(env.ID, api.lab.Database, senderAddress, logicID)
	// If the logic describes an ephemeral state, enlist the sender (deployer)
	if _, ok = logic.Object.EphemeralState(); ok {
		_ = env.Enlist(senderAddress, logicID)
	}

	// Generate an interaction from the kind, callsite, calldata and manifest
	ixn := core.LogicInteraction{
		Nonce: env.Nonce,
		Limit: env.CallFuel,
		Price: new(big.Int).SetUint64(core.LabFuelPrice),

		Kind: common.IxLogicDeploy,
		Site: request.Callsite,
		Call: func() []byte {
			// Set the calldata as nil if no calldata is provided
			if request.Calldata != "" {
				return calldata
			}

			return nil
		}(),
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

	// Mark the logic as deployed
	logic.Ready = true

	env.IncrementNonce()

	// Get the logic interaction hash
	hash, err := ixn.Hash()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	// Get the events of core.Event type
	events := core.GetEventsFromStream(eventstream, hash)
	// Insert events into the environment
	if err = env.InsertEvents(events); err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	c.JSON(http.StatusOK, Success().WithData(LogicCallResponse{
		Ixhash: hash,
		Result: Result{
			Ok:   result.Ok(),
			Fuel: result.Fuel(),
			Error: func() (err string) {
				if result.Error() != nil {
					err = "0x" + hex.EncodeToString(result.Error())
				}

				return
			}(),
		},
		Events: events,
	}))
}

func (api *API) InteractLogicInvoke(c *gin.Context) {
	request := new(LogicCallRequest)
	// Call BindJSON to bind the received JSON to logicCall
	if err := c.ShouldBindJSON(request); err != nil {
		c.JSON(http.StatusBadRequest, Error(err))
	}

	call, err := api.obtainCallDetails(c.GetHeader(HeaderLabEnv), request)
	if err != nil {
		c.JSON(call.code, Error(err))
		return
	}

	env := call.env
	logic := call.logic

	// Perform deploy gating
	// Only allow to invoke, if logic is ready.
	if !logic.Ready {
		c.JSON(http.StatusBadRequest, Error(errors.New("logic is not ready. must be deployed first")))
		return
	}

	// Get the callsite from the logic, error if not found
	callsite, ok := logic.Object.GetCallsite(request.Callsite)
	if !ok {
		c.JSON(http.StatusNotFound, Error(errors.New("callsite not found on logic")))
		return
	}

	// Check that call kind matches for the callsite
	if callsite.Kind != engineio.CallsiteInvoke {
		c.JSON(http.StatusBadRequest, Error(errors.New("callsite is not invokable")))
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
		core.NewStorageDriver(env.ID, api.lab.Database, logicID.Address(), logicID),
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

	identifier, _ := logicID.Identifier()
	// Check if the logic has an ephemeral state
	if identifier.HasEphemeralState() {
		// Check if sender has been enlisted for the logic ID
		if !env.Enlisted(senderAddress, logicID) {
			c.JSON(http.StatusExpectationFailed, "sender is not enlisted with ephemeral logic")
			return
		}
	}

	// Generate the context object for the sender
	senderContext := core.NewStorageDriver(env.ID, api.lab.Database, senderAddress, logicID)

	// Generate an interaction from the kind, callsite, calldata and manifest
	ixn := core.LogicInteraction{
		Nonce: env.Nonce,
		Limit: env.CallFuel,
		Price: new(big.Int).SetUint64(core.LabFuelPrice),

		Kind: common.IxLogicInvoke,
		Site: request.Callsite,
		Call: func() []byte {
			// Set the calldata as nil if no calldata is provided
			if request.Calldata != "" {
				return calldata
			}

			return nil
		}(),
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

	env.IncrementNonce()

	// Get the logic interaction hash
	hash, err := ixn.Hash()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	// Get the events of core.Event type
	events := core.GetEventsFromStream(eventstream, hash)
	// Insert events into the environment
	if err = env.InsertEvents(events); err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	// Convert output into hex string
	var output string
	if result.Outputs() != nil {
		output = "0x" + hex.EncodeToString(result.Outputs())
	}

	c.JSON(http.StatusOK, Success().WithData(LogicCallResponse{
		Ixhash: hash,
		Events: events,
		Result: Result{
			Ok:   result.Ok(),
			Fuel: result.Fuel(),

			Output: output,
			Error: func() (err string) {
				if result.Error() != nil {
					err = "0x" + hex.EncodeToString(result.Error())
				}

				return
			}(),
		},
	}))
}

func (api *API) InteractLogicEnlist(c *gin.Context) {
	request := new(LogicCallRequest)
	// Call BindJSON to bind the received JSON to logicCall
	if err := c.ShouldBindJSON(request); err != nil {
		c.JSON(http.StatusBadRequest, Error(err))
	}

	call, err := api.obtainCallDetails(c.GetHeader(HeaderLabEnv), request)
	if err != nil {
		c.JSON(call.code, Error(err))
		return
	}

	env := call.env
	logic := call.logic

	// Only allow to enlist, if logic is ready.
	if !logic.Ready {
		c.JSON(http.StatusBadRequest,
			Error(errors.New("logic is not ready. has an persistent that must be initialised with 'deploy'")))
		return
	}

	// Get the callsite from the logic, error if not found
	callsite, ok := logic.Object.GetCallsite(request.Callsite)
	if !ok {
		c.JSON(http.StatusNotFound, Error(errors.New("callsite not found on logic")))
		return
	}

	// Check that call kind matches for the callsite
	if callsite.Kind != engineio.CallsiteEnlist {
		c.JSON(http.StatusBadRequest, Error(errors.New("callsite is not enlister")))
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

	if env.Sender == "" {
		c.JSON(http.StatusNotFound, Error(core.ErrSenderNotConf))
		return
	}

	// Get the address of the sender
	senderAddress := env.Users[env.Sender]
	// Check if the user has already enlisted with logic
	if env.Enlisted(senderAddress, logicID) {
		c.JSON(http.StatusBadRequest, Error(errors.New("user already enlisted with logic")))
		return
	}

	// Spawn an instance for the engine
	instance, err := engine.SpawnInstance(
		logic.Object,
		env.CallFuel,
		core.NewStorageDriver(env.ID, api.lab.Database, logicID.Address(), logicID),
		api.lab,
		eventstream,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Error(errors.Wrap(err, "failed to spawn engine")))
		return
	}

	// Generate the context object for the sender
	senderContext := core.NewStorageDriver(env.ID, api.lab.Database, senderAddress, logicID)

	// Generate an interaction from the kind, callsite and calldata
	ixn := core.LogicInteraction{
		Nonce: env.Nonce,
		Limit: env.CallFuel,
		Price: new(big.Int).SetUint64(core.LabFuelPrice),

		Kind: common.IxLogicEnlist,
		Site: request.Callsite,
		Call: func() []byte {
			// Set the calldata as nil if no calldata is provided
			if request.Calldata != "" {
				return calldata
			}

			return nil
		}(),
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

	env.IncrementNonce()
	_ = env.Enlist(senderAddress, logicID)

	// Get the logic interaction hash
	hash, err := ixn.Hash()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	// Get the events of core.Event type
	events := core.GetEventsFromStream(eventstream, hash)
	// Insert events into the environment
	if err = env.InsertEvents(events); err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	// Convert output into hex string
	var output string
	if result.Outputs() != nil {
		output = "0x" + hex.EncodeToString(result.Outputs())
	}

	c.JSON(http.StatusOK, Success().WithData(LogicCallResponse{
		Ixhash: hash,
		Events: events,
		Result: Result{
			Ok:   result.Ok(),
			Fuel: result.Fuel(),

			Output: output,
			Error: func() (err string) {
				if result.Error() != nil {
					err = "0x" + hex.EncodeToString(result.Error())
				}

				return
			}(),
		},
	}))
}
