//nolint:nlreturn
package api

import (
	"fmt"
	"net/http"

	"github.com/sarvalabs/go-moi/common"

	"github.com/gin-gonic/gin"
	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-moi/cmd/logiclab/core"
)

const (
	QueryLogicID       = "logicid"
	QueryParticipantID = "id"
	QueryIxHash        = "ixhash"
	QueryName          = "name"
)

func (api *API) getEvents(c *gin.Context) {
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

	filters := make([]core.EventFilter, 0)

	if queryLogicID := c.Query(QueryLogicID); queryLogicID != "" {
		logicID, err := identifiers.NewLogicIDFromHex(queryLogicID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, Error(err))
			return
		}

		filters = append(filters, core.FilterByLogicID(logicID))
	}

	if queryParticipantID := c.Query(QueryParticipantID); queryParticipantID != "" {
		participantID, err := identifiers.NewParticipantIDFromHex(queryParticipantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, Error(err))
			return
		}

		filters = append(filters, core.FilterByIdentifier(participantID.AsIdentifier()))
	}

	if ixhash := c.Query(QueryIxHash); ixhash != "" {
		filters = append(filters, core.FilterByIxHash(common.HexToHash(ixhash)))
	}

	if name := c.Query(QueryName); name != "" {
		filters = append(filters, core.FilterByName(name))
	}

	for i := 1; i < core.MaxTopics; i++ {
		topicParam := fmt.Sprintf("topic%d", i)
		topic := c.Query(topicParam)

		if topic != "" {
			filters = append(filters, core.FilterByTopic(i, common.HexToHash(topic)))
		}
	}

	events, err := env.GetEvents(filters...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Error(err))
		return
	}

	c.JSON(http.StatusOK, Success().WithData(events))
}
