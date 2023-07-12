package websocket

import (
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"

	"github.com/sarvalabs/moichain/common/utils"
)

type Handler struct {
	// Represent the logger instance for websocket service
	logger hclog.Logger

	// Represents the dispatcher module for websocket service
	dispatcher Dispatcher
}

// wsUpgrader defines upgrade parameters for the WS connection
var wsUpgrader = websocket.Upgrader{
	// Uses the default HTTP buffer sizes for Read / Write buffers.
	// Documentation specifies that they are 4096B in size.
	// There is no need to have them be 4x in size when requests / responses
	// shouldn't exceed 1024B
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// CORS - Allow requests from anywhere
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func NewHandler(logger hclog.Logger, eventMux *utils.TypeMux) *Handler {
	return &Handler{
		logger:     logger.Named("Websocket-Handler"),
		dispatcher: NewDispatcher(logger, eventMux),
	}
}

// isCompatibleMsgType returns a flag which indicates whether the message type is compatible or not
func isCompatibleMsgType(messageType int) bool {
	return messageType == websocket.TextMessage || messageType == websocket.BinaryMessage
}

func (h *Handler) HandleWsRequests(w http.ResponseWriter, req *http.Request) {
	// Upgrade the HTTP connection to the WebSocket protocol
	currentWSConn, err := wsUpgrader.Upgrade(w, req, nil)
	if err != nil {
		h.logger.Error("Failed to upgrade to a websocket connection", "err", err)

		return
	}

	// Handle websocket connection closure
	defer func() {
		if err := currentWSConn.Close(); err != nil {
			h.logger.Error("Failed to gracefully close websocket connection", "err", err)
		}
	}()

	connManager := NewConnectionManager(currentWSConn)

	h.logger.Info("Websocket connection established")

	// Run the loop to listen for the incoming messages
	for {
		messageType, message, err := connManager.ReadMessage()
		if err != nil {
			// Check whether it's a close error with accepted close codes
			if websocket.IsCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseNormalClosure,
				websocket.CloseAbnormalClosure,
				websocket.CloseNoStatusReceived,
			) {
				h.logger.Info("Closing websocket connection")
			} else {
				h.logger.Error("Failed to read websocket message", "err", err)
				h.logger.Info("Closing websocket connection with error")
			}

			h.dispatcher.RemoveSubscription(connManager)

			break
		}

		if isCompatibleMsgType(messageType) {
			go func() {
				resp, handleErr := h.dispatcher.handleRequests(message, connManager)
				if handleErr != nil {
					h.logger.Error("Failed to handle websocket request", "err", handleErr)

					writeErr := connManager.WriteMessage(
						messageType,
						[]byte(fmt.Sprintf("WebSocket Error: %s", handleErr.Error())),
					)

					if writeErr != nil {
						h.logger.Error("Failed to send the response to client", "err", writeErr)
					}

					return
				}

				writeErr := connManager.WriteMessage(messageType, resp)
				if writeErr != nil {
					h.logger.Error("Failed to send the response to client", "err", writeErr)
				}
			}()
		}
	}
}
