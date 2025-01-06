package jsonrpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/sarvalabs/go-moi/common"

	"github.com/hashicorp/go-hclog"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/common/config"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
)

var emptyParamsRegex = regexp.MustCompile(`^\[\s*]$`)

type ConnManager interface {
	HasConn() bool
	SetFilterID(filterID string)
	GetFilterID() string
	WriteMessage(messageType int, data []byte) error
}

type serviceData struct {
	serviceValue reflect.Value
	methodMap    map[string]*methodData
}

type methodData struct {
	inNum       int
	reqt        []reflect.Type
	methodValue reflect.Value
	isDyn       bool
}

func (f *methodData) numParams() int {
	return f.inNum - 1
}

// dispatcher handles all websocket requests
type dispatcher struct {
	logger                  hclog.Logger
	fm                      *FilterManager
	jsonRPCBatchLengthLimit uint64

	// Stores the map of all services to methods during registration
	serviceMap map[string]*serviceData
}

func newDispatcher(logger hclog.Logger, cfg *config.Config, filterMan *FilterManager) *dispatcher {
	dispatcher := &dispatcher{
		logger:                  logger.Named("Websocket-dispatcher"),
		fm:                      filterMan,
		jsonRPCBatchLengthLimit: cfg.JSONRPC.BatchLengthLimit,
	}

	go dispatcher.fm.Run()

	return dispatcher
}

// decodeTesseractArgs decodes the json api request parameter made for new tesseracts event subscription
func decodeTesseractArgs(data interface{}) (*TesseractArgs, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	args := new(TesseractArgs)
	if err := json.Unmarshal(raw, args); err != nil {
		return nil, err
	}

	return args, nil
}

// as per https://www.jsonrpc.org/specification, the `id` in JSON-RPC 2.0
// can only be a string or a non-decimal integer
func formatID(id interface{}) (interface{}, Error) {
	switch t := id.(type) {
	case string:
		return t, nil
	case float64:
		if t == math.Trunc(t) {
			return int(t), nil
		} else {
			return "", NewInvalidRequestError("invalid json request")
		}
	case nil:
		return nil, nil
	default:
		return "", NewInvalidRequestError("invalid json request")
	}
}

// handleSingleWs handles all the incoming websocket requests
func (d *dispatcher) handleSingleWs(req Request, conn ConnManager) Response {
	id, err := formatID(req.ID)
	if err != nil {
		return NewRPCResponse(nil, "2.0", nil, err)
	}

	var response []byte

	switch req.Method {
	case "moi.subscribe":
		var filterID string

		// if the request method is moi.subscribe we need to create a new filter with ws connection
		if filterID, err = d.handleSubscribe(req, conn); err == nil {
			response = []byte(fmt.Sprintf("\"%s\"", filterID))
		}
	case "moi.unsubscribe":
		var ok bool

		if ok, err = d.handleUnsubscribe(req); err == nil {
			response = []byte(strconv.FormatBool(ok))
		}
	default:
		// it's a normal query that we handle with the dispatcher
		response, err = d.handleReq(req)
	}

	return NewRPCResponse(id, "2.0", response, err)
}

// handleSubscribe method subscribes to a specific event
func (d *dispatcher) handleSubscribe(req Request, conn ConnManager) (string, Error) {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return "", NewInvalidRequestError("invalid json request")
	}

	if len(params) == 0 {
		return "", NewInvalidParamsError("invalid params")
	}

	subscribeMethod, ok := params[0].(string)
	if !ok {
		return "", NewSubscriptionNotFoundError(subscribeMethod)
	}

	switch rpcargs.SubscriptionType(subscribeMethod) {
	case rpcargs.NewTesseract:
		subscriptionID := d.fm.NewTesseractFilter(conn)

		return subscriptionID, nil
	case rpcargs.NewTesseractsByAccount:
		if len(params) != 2 {
			return "", NewInvalidParamsError("invalid params")
		}

		args, err := decodeTesseractArgs(params[1])
		if err != nil {
			return "", NewInternalError(err.Error())
		}

		addr, _ := identifiers.NewAddressFromHex(args.Address)
		subscriptionID := d.fm.NewTesseractsByAccountFilter(conn, addr)

		return subscriptionID, nil
	case rpcargs.NewLogsByFilter:
		if len(params) != 2 {
			return "", NewInvalidParamsError("invalid params")
		}

		args, err := decodeFilterQuery(params[1])
		if err != nil {
			return "", NewInternalError(err.Error())
		}

		subscriptionID := d.fm.NewLogFilter(conn, args)

		return subscriptionID, nil
	case rpcargs.PendingIxns:
		subscriptionID := d.fm.PendingIxnsFilter(conn)

		return subscriptionID, nil
	default:
		return "", NewSubscriptionNotFoundError(subscribeMethod)
	}
}

// handleUnsubscribe method unsubscribes from a specific event
func (d *dispatcher) handleUnsubscribe(req Request) (bool, Error) {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return false, NewInvalidRequestError("invalid json request")
	}

	if len(params) != 1 {
		return false, NewInvalidParamsError("invalid params")
	}

	subscriptionID, ok := params[0].(string)
	if !ok {
		return false, NewSubscriptionNotFoundError(subscriptionID)
	}

	return d.fm.Uninstall(subscriptionID), nil
}

// removeSubscription removes a filter corresponding to the websocket connection from the filter manager
func (d *dispatcher) removeSubscription(connManager ConnManager) bool {
	return d.fm.Uninstall(connManager.GetFilterID())
}

func (d *dispatcher) isExceedingBatchLengthLimit(value uint64) bool {
	return d.jsonRPCBatchLengthLimit != 0 && value > d.jsonRPCBatchLengthLimit
}

func (d *dispatcher) handleWs(reqBody []byte, conn ConnManager) ([]byte, error) {
	const (
		openSquareBracket  byte = '['
		closeSquareBracket byte = ']'
		comma              byte = ','
	)

	reqBody = bytes.TrimLeft(reqBody, " \t\r\n")

	// if body begins with [ consider it as a batch request
	if len(reqBody) > 0 && reqBody[0] == openSquareBracket {
		var batchReq BatchRequest

		err := json.Unmarshal(reqBody, &batchReq)
		if err != nil {
			return NewRPCResponse(nil, "2.0", nil,
				NewInvalidRequestError("Invalid json batch request")).Bytes()
		}

		// if not disabled, avoid handling long batch requests
		if d.isExceedingBatchLengthLimit(uint64(len(batchReq))) {
			return NewRPCResponse(
				nil,
				"2.0",
				nil,
				NewInvalidRequestError("batch request length too long"),
			).Bytes()
		}

		responses := make([][]byte, len(batchReq))

		for i, req := range batchReq {
			responses[i], err = d.handleSingleWs(req, conn).Bytes()
			if err != nil {
				return nil, err
			}
		}

		var buf bytes.Buffer

		// batch output should look like:
		// [ { "requestId": "1", "status": 200 }, { "requestId": "2", "status": 200 } ]
		buf.WriteByte(openSquareBracket)                // [
		buf.Write(bytes.Join(responses, []byte{comma})) // join responses with the comma separator
		buf.WriteByte(closeSquareBracket)               // ]

		return buf.Bytes(), nil
	}

	var req Request
	if err := json.Unmarshal(reqBody, &req); err != nil {
		return NewRPCResponse(req.ID, "2.0", nil, NewInvalidRequestError("invalid json request")).Bytes()
	}

	return d.handleSingleWs(req, conn).Bytes()
}

func (d *dispatcher) handle(reqBody []byte) ([]byte, error) {
	x := bytes.TrimLeft(reqBody, " \t\r\n")
	if len(x) == 0 {
		return NewRPCResponse(nil, "2.0", nil, NewInvalidRequestError("invalid json request")).Bytes()
	}

	if x[0] == '{' {
		var req Request
		if err := json.Unmarshal(reqBody, &req); err != nil {
			return NewRPCResponse(nil, "2.0", nil, NewInvalidRequestError("invalid json request")).Bytes()
		}

		resp, err := d.handleReq(req)

		return NewRPCResponse(req.ID, "2.0", resp, err).Bytes()
	}

	// handle batch requests
	var requests BatchRequest
	if err := json.Unmarshal(reqBody, &requests); err != nil {
		return NewRPCResponse(
			nil,
			"2.0",
			nil,
			NewInvalidRequestError("invalid json request"),
		).Bytes()
	}

	// if not disabled, avoid handling long batch requests
	if d.isExceedingBatchLengthLimit(uint64(len(requests))) {
		return NewRPCResponse(
			nil,
			"2.0",
			nil,
			NewInvalidRequestError("batch request length too long"),
		).Bytes()
	}

	responses := make([]Response, len(requests))

	for i, req := range requests {
		response, err := d.handleReq(req)
		if err != nil {
			errorResponse := NewRPCResponse(req.ID, "2.0", response, err)
			responses[i] = errorResponse

			continue
		}

		resp := NewRPCResponse(req.ID, "2.0", response, nil)
		responses[i] = resp
	}

	respBytes, err := json.Marshal(responses)
	if err != nil {
		return NewRPCResponse(nil, "2.0", nil, NewInternalError("Internal error")).Bytes()
	}

	return respBytes, nil
}

func (d *dispatcher) getMethodHandler(req Request) (*serviceData, *methodData, Error) {
	callName := strings.SplitN(req.Method, ".", 2)

	if len(callName) != 2 {
		return nil, nil, NewMethodNotFoundError(req.Method)
	}

	serviceName, funcName := callName[0], callName[1]

	service, ok := d.serviceMap[serviceName]
	if !ok {
		return nil, nil, NewMethodNotFoundError(req.Method)
	}

	fd, ok := service.methodMap[funcName]
	if !ok {
		return nil, nil, NewMethodNotFoundError(req.Method)
	}

	return service, fd, nil
}

func (d *dispatcher) handleReq(req Request) ([]byte, Error) {
	service, funcData, funcErr := d.getMethodHandler(req)
	if funcErr != nil {
		return nil, funcErr
	}

	inArgs := make([]reflect.Value, funcData.inNum)
	inArgs[0] = service.serviceValue

	inputs := make([]interface{}, funcData.numParams())

	for i := 0; i < funcData.inNum-1; i++ {
		val := reflect.New(funcData.reqt[i+1])
		inputs[i] = val.Interface()
		inArgs[i+1] = val.Elem()
	}

	if funcData.numParams() > 0 {
		// Replace empty `[]` with `[{}]` to ensure proper unmarshalling
		if emptyParamsRegex.MatchString(string(req.Params)) {
			req.Params = json.RawMessage(`[{}]`)
		}

		if err := json.Unmarshal(req.Params, &inputs); err != nil {
			return nil, NewInvalidParamsError("Invalid Params")
		}
	}

	var (
		data []byte
		err  error
		ok   bool
	)

	output := funcData.methodValue.Call(inArgs) // call rpc endpoint function

	if err := getError(output[1]); err != nil {
		if res := output[0].Interface(); res != nil {
			data, ok = res.([]byte)

			if !ok {
				return nil, NewInternalError(err.Error())
			}
		}

		return data, NewInvalidRequestError(err.Error())
	}

	if res := output[0].Interface(); res != nil {
		data, err = json.Marshal(res)
		if err != nil {
			return nil, NewInternalError("Internal error")
		}
	}

	return data, nil
}

func getError(v reflect.Value) error {
	if v.IsNil() {
		return nil
	}

	extractedErr, ok := v.Interface().(error)
	if !ok {
		return errors.New("invalid type assertion, unable to extract error")
	}

	return extractedErr
}

func (d *dispatcher) registerService(serviceName string, service interface{}) error {
	if d.serviceMap == nil {
		d.serviceMap = map[string]*serviceData{}
	}

	if serviceName == "" {
		return common.ErrEmptyServiceName
	}

	funcMap := make(map[string]*methodData)

	st := reflect.TypeOf(service)
	if st.Kind() == reflect.Struct {
		return fmt.Errorf("jsonrpc: service '%s' must be a pointer to struct", serviceName)
	}

	for i := 0; i < st.NumMethod(); i++ {
		method := st.Method(i)
		if method.PkgPath != "" { // skip unexported methods
			continue
		}

		funcName := serviceName + "." + method.Name

		funcData := &methodData{
			methodValue: method.Func,
		}

		var err error

		// Validate the method signature
		if funcData.inNum, funcData.reqt, err = validateMethod(funcName, funcData.methodValue, true); err != nil {
			return fmt.Errorf("validate function error: %w", err)
		}

		// check if last item is a pointer
		if funcData.numParams() != 0 {
			last := funcData.reqt[funcData.numParams()]
			if last.Kind() == reflect.Ptr {
				funcData.isDyn = true
			}
		}

		funcMap[method.Name] = funcData
	}

	d.serviceMap[serviceName] = &serviceData{
		serviceValue: reflect.ValueOf(service),
		methodMap:    funcMap,
	}

	return nil
}

//nolint:nakedret
func validateMethod(methodName string, methodValue reflect.Value, _ bool) (inNum int, reqt []reflect.Type, err error) {
	if methodName == "" {
		err = common.ErrEmptyMethodName

		return
	}

	ft := methodValue.Type()
	if ft.Kind() != reflect.Func {
		err = fmt.Errorf("'%s' must be a method instead of %s", methodName, ft)

		return
	}

	inNum = ft.NumIn()

	if outNum := ft.NumOut(); ft.NumOut() != 2 {
		err = fmt.Errorf("unexpected number of output arguments in the method '%s': %d. Expected 2", methodName, outNum)

		return
	}

	if !isErrorType(ft.Out(1)) {
		err = fmt.Errorf(
			"unexpected type for the second return value of the method '%s': '%s'. Expected '%s'",
			methodName,
			ft.Out(1),
			errt,
		)

		return
	}

	reqt = make([]reflect.Type, inNum)
	for i := 0; i < inNum; i++ {
		reqt[i] = ft.In(i)
	}

	return
}

var errt = reflect.TypeOf((*error)(nil)).Elem()

func isErrorType(t reflect.Type) bool {
	return t.Implements(errt)
}
