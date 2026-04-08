package jsonrpc

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	gorillaWS "github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
	"github.com/sarvalabs/go-moi/common/config"
)

// Server is a struct that represents a wrapper for a JSON-RPC RPC server
type Server struct {
	// Represents an RPC router
	router *http.ServeMux

	// Represents the RPC server URL
	url string

	// Represent the RPC server addr
	addr *net.TCPAddr

	// Represent the logger instance for RPC service
	logger hclog.Logger

	// Represents the origins that are allowed receive response
	corsAllowedOrigins []string

	// Represents the dispatcher module for websocket service
	dispatcher *dispatcher
}

// NewRPCServer is a constructor function that generates and returns
// a new RPC Server for a given URL and addr.
func NewRPCServer(
	path string,
	logger hclog.Logger,
	cfg *config.Config,
	filterMan *FilterManager,
) *Server {
	// Create a new Server object and return it
	return &Server{
		logger:             logger.Named("JSON-RPC-Server"),
		router:             http.NewServeMux(),
		url:                path,
		addr:               cfg.Network.JSONRPCAddr,
		corsAllowedOrigins: cfg.Network.CorsAllowedOrigins,
		dispatcher:         newDispatcher(logger, cfg, filterMan),
	}
}

func middlewareFactory(cors []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Handle preflight OPTIONS request
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

				return
			}

			// Set CORS headers for non-OPTIONS requests
			for _, allowedOrigin := range cors {
				if allowedOrigin == "*" {
					w.Header().Set("Access-Control-Allow-Origin", "*")

					break
				}

				if allowedOrigin == origin {
					w.Header().Set("Access-Control-Allow-Origin", origin)

					break
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Start is a method of Server that starts the RPC server
func (s *Server) Start() error {
	// Register the JSON-RPC and WebSocket handlers
	jsonRPCHandler := http.HandlerFunc(s.handle)
	s.router.Handle("/", middlewareFactory(s.corsAllowedOrigins)(jsonRPCHandler))

	s.router.HandleFunc("/ws", s.handleWs)

	server := &http.Server{
		Addr:              s.addr.String(),
		Handler:           s.router,
		ReadHeaderTimeout: 3 * time.Second,
	}

	s.logger.Info("RPC Server started on", "server-URL", s.url, "server-addr", s.addr)

	if err := server.ListenAndServe(); err != nil {
		s.logger.Error("JSON RPC server stopped", "err", err)

		return err
	}

	return nil
}

func (s *Server) handleJSONRPCRequest(w http.ResponseWriter, req *http.Request) {
	data, err := io.ReadAll(req.Body)
	if err != nil {
		_, _ = w.Write([]byte(err.Error()))

		return
	}

	resp, err := s.dispatcher.handle(data)
	if err != nil {
		_, _ = w.Write([]byte(err.Error()))
	} else {
		_, _ = w.Write(resp)
	}
}

func (s *Server) handle(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set(
		"Access-Control-Allow-Headers",
		"Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization",
	)

	switch req.Method {
	case "POST":
		s.handleJSONRPCRequest(w, req)
	case "OPTIONS":
		// nothing to return
	default:
		_, _ = w.Write([]byte("method " + req.Method + " not allowed"))
	}
}

// wsUpgrader defines upgrade parameters for the WS connection
var wsUpgrader = gorillaWS.Upgrader{
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

// isCompatibleMsgType returns a flag which indicates whether the message type is compatible or not
func isCompatibleMsgType(messageType int) bool {
	return messageType == gorillaWS.TextMessage || messageType == gorillaWS.BinaryMessage
}

func (s *Server) handleWs(w http.ResponseWriter, req *http.Request) {
	// Upgrade the HTTP connection to the WebSocket protocol
	currentWSConn, err := wsUpgrader.Upgrade(w, req, nil)
	if err != nil {
		s.logger.Error("failed to upgrade to a websocket connection", "err", err)

		return
	}

	// Handle websocket connection closure
	defer func() {
		if err := currentWSConn.Close(); err != nil {
			s.logger.Error("failed to gracefully close websocket connection", "err", err)
		}
	}()

	connManager := NewConnectionManager(currentWSConn)

	s.logger.Info("Websocket connection established")

	// Run the loop to listen for the incoming messages
	for {
		messageType, message, err := connManager.ReadMessage()
		if err != nil {
			// Check whether it's a close error with accepted close codes
			if gorillaWS.IsCloseError(err,
				gorillaWS.CloseGoingAway,
				gorillaWS.CloseNormalClosure,
				gorillaWS.CloseAbnormalClosure,
				gorillaWS.CloseNoStatusReceived,
			) {
				s.logger.Info("Closing websocket connection")
			} else {
				s.logger.Error("failed to read websocket message", "err", err)
				s.logger.Info("Closing websocket connection with error")
			}

			s.dispatcher.removeSubscription(connManager)

			break
		}

		if isCompatibleMsgType(messageType) {
			go func() {
				resp, handleErr := s.dispatcher.handleWs(message, connManager)
				if handleErr != nil {
					s.logger.Error("failed to handle websocket request", "err", handleErr)

					writeErr := connManager.WriteMessage(
						messageType,
						[]byte(fmt.Sprintf("WebSocket Error: %s", handleErr.Error())),
					)

					if writeErr != nil {
						s.logger.Error("failed to send the response to client", "err", writeErr)
					}

					return
				}

				writeErr := connManager.WriteMessage(messageType, resp)
				if writeErr != nil {
					s.logger.Error("failed to send the response to client", "err", writeErr)
				}
			}()
		}
	}
}

func (s *Server) RegisterService(serviceName string, service interface{}) error {
	return s.dispatcher.registerService(serviceName, service)
}
