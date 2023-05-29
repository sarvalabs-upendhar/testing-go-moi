package rpc

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/handlers"

	"github.com/sarvalabs/moichain/common"

	"github.com/gorilla/mux"
	"github.com/gorilla/rpc/v2"
	"github.com/gorilla/rpc/v2/json2"
	"github.com/hashicorp/go-hclog"

	"github.com/sarvalabs/moichain/poorna/websocket"
	"github.com/sarvalabs/moichain/utils"
)

// Server is a struct that represents a wrapper for a libp2p RPC server
type Server struct {
	// Represents an RPC router
	router *mux.Router

	// Represents the libp2p RPC server
	server *rpc.Server

	// Represents the RPC server URL
	url string

	// Represent the RPC server addr
	addr *net.TCPAddr

	// Represent the logger instance for RPC service
	logger hclog.Logger

	// Represents the handler module for websocket service
	wsHandler *websocket.Handler

	// Represents the origins that are allowed receive response
	corsAllowedOrigins []string
}

// NewRPCServer is a constructor function that generates and returns
// a new RPC Server for a given URL and addr.
func NewRPCServer(path string, logger hclog.Logger, cfg *common.NetworkConfig, eventMux *utils.TypeMux) *Server {
	// Create a new Server object and return it
	return &Server{
		logger:             logger.Named("json-rpc"),
		router:             mux.NewRouter(),
		server:             rpc.NewServer(),
		url:                path,
		addr:               cfg.JSONRPCAddr,
		corsAllowedOrigins: cfg.CorsAllowedOrigins,
		wsHandler:          websocket.NewHandler(logger, eventMux),
	}
}

func (s *Server) addCORSMiddleware() {
	if len(s.corsAllowedOrigins) != 0 {
		// Create the CORS middleware
		corsMiddleware := handlers.CORS(
			handlers.AllowedOrigins(s.corsAllowedOrigins),     // Allow requests from any origin
			handlers.AllowedHeaders([]string{"Content-Type"}), // Allow specified headers
		)

		s.router.Use(corsMiddleware)
	}
}

// Start is a method of Server that starts the RPC server
func (s *Server) Start() error {
	s.server.RegisterCodec(json2.NewCodec(), "application/json")
	s.router.Handle(s.url, s.server)
	// Web socket route
	s.router.HandleFunc("/ws", s.wsHandler.HandleWsRequests)

	s.addCORSMiddleware()

	// Print the server start message
	s.logger.Info(fmt.Sprintf("RPC Server started on %s:%s", s.url, s.addr))

	// Start the RPC server
	server := &http.Server{
		Addr:              s.addr.String(),
		Handler:           s.router,
		ReadHeaderTimeout: 3 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		s.logger.Error("JSON RPC server stopped", "error", err)

		return err
	}

	return nil
}

// RegisterService is a method of Server that registers a service with the RPC service.
// Accepts the service name and the service receiver and returns an error.
func (s *Server) RegisterService(name string, receiver interface{}) error {
	// Register the service with the RPC service
	return s.server.RegisterService(receiver, name)
}
