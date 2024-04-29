//nolint:nlreturn
package api

import (
	"net/http"

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

type ManifestConversionRequest struct {
	Target   string   `json:"target"`
	Manifest Manifest `json:"manifest"`
}

func (api *API) convertManifestCodeform(c *gin.Context) {
	// Decode the request
	request := new(ManifestConversionRequest)
	if err := c.ShouldBindJSON(request); err != nil {
		c.JSON(http.StatusBadRequest, Error(err))
		return
	}

	rawManifest := common.Hex2Bytes(request.Manifest.Content)
	encoding := common.EncodingFromString(request.Manifest.Encoding)

	manifest, err := engineio.NewManifest(rawManifest, encoding)
	if err != nil {
		c.JSON(http.StatusBadRequest, Error(errors.Wrap(err, "malformed manifest")))
		return
	}

	converted := core.ConvertManifestCodeform(manifest, encoding, request.Target)

	c.JSON(http.StatusOK, Success().WithData(Manifest{
		Encoding: request.Manifest.Encoding,
		Content:  converted,
	}))
}

func (api *API) convertManifestFileform(c *gin.Context) {
	// Decode the request
	request := new(ManifestConversionRequest)
	if err := c.ShouldBindJSON(request); err != nil {
		c.JSON(http.StatusBadRequest, Error(err))
		return
	}

	rawManifest := common.Hex2Bytes(request.Manifest.Content)
	encoding := common.EncodingFromString(request.Manifest.Encoding)
	target := common.EncodingFromString(request.Target)

	manifest, err := engineio.NewManifest(rawManifest, encoding)
	if err != nil {
		c.JSON(http.StatusBadRequest, Error(errors.Wrap(err, "malformed manifest")))
		return
	}

	converted := core.PrintManifest(manifest, target)

	c.JSON(http.StatusOK, Success().WithData(Manifest{
		Encoding: request.Target,
		Content:  converted,
	}))
}
