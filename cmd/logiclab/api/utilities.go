//nolint:nlreturn
package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/cmd/logiclab/core"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
)

type ErrorDecodeRequest struct {
	Error string `json:"error"`
}

type ErrorDecodeResponse struct {
	Decoded string `json:"decoded"`
}

func (api *API) decodeErrorData(c *gin.Context) {
	// Decode the request
	request := new(ErrorDecodeRequest)
	if err := c.ShouldBindJSON(request); err != nil {
		c.JSON(http.StatusBadRequest, Error(err))
		return
	}

	// Extract the engine kind from the path
	engineKind := c.Param("engine")
	// Get the engine runtime for the given engine
	engine, ok := engineio.FetchEngine(engineio.EngineKindFromString(engineKind))
	if !ok {
		c.JSON(http.StatusBadRequest, Error(core.ErrUnsupportedEngine))
		return
	}

	// Hex-decode the error data
	errdata := common.Hex2Bytes(request.Error)
	// Decode the error with the runtime rules
	errorObject, err := engine.DecodeErrorResult(errdata)
	if err != nil {
		c.JSON(http.StatusBadRequest, Error(errors.Wrap(err, "failed to decode error object")))
		return
	}

	c.JSON(http.StatusOK, Success().WithData(ErrorDecodeResponse{Decoded: errorObject.String()}))
}

func (api *API) convertManifestCodeform(c *gin.Context) {
	// Decode the request
	request := new(Manifest)
	if err := c.ShouldBindJSON(request); err != nil {
		c.JSON(http.StatusBadRequest, Error(err))
		return
	}

	rawManifest := common.Hex2Bytes(request.Content)
	encoding := common.EncodingFromString(strings.ToUpper(request.Encoding))

	manifest, err := engineio.NewManifest(rawManifest, encoding)
	if err != nil {
		c.JSON(http.StatusBadRequest, Error(errors.Wrap(err, "malformed manifest")))
		return
	}

	format := c.Param("format")
	converted := core.ConvertManifestCodeform(manifest, encoding, strings.ToUpper(format))

	c.JSON(http.StatusOK, Success().WithData(Manifest{
		Encoding: strings.ToUpper(request.Encoding),
		Content:  converted,
	}))
}

func (api *API) convertManifestFileform(c *gin.Context) {
	// Decode the request
	request := new(Manifest)
	if err := c.ShouldBindJSON(request); err != nil {
		c.JSON(http.StatusBadRequest, Error(err))
		return
	}

	rawManifest := common.Hex2Bytes(request.Content)
	encoding := common.EncodingFromString(request.Encoding)

	manifest, err := engineio.NewManifest(rawManifest, encoding)
	if err != nil {
		c.JSON(http.StatusBadRequest, Error(errors.Wrap(err, "malformed manifest")))
		return
	}

	format := c.Param("encoding")
	target := common.EncodingFromString(strings.ToUpper(format))

	converted := core.PrintManifest(manifest, target)

	c.JSON(http.StatusOK, Success().WithData(Manifest{
		Encoding: strings.ToUpper(format),
		Content:  converted,
	}))
}
