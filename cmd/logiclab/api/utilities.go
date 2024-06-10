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
