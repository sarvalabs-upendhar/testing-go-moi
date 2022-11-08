package rpc

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/rpc/v2/json2"
	"github.com/hashicorp/go-hclog"

	"github.com/gorilla/mux"
	"github.com/gorilla/rpc/v2"
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
}

// NewRPCServer is a constructor function that generates and returns
// a new RPC Server for a given URL and addr.
func NewRPCServer(path string, logger hclog.Logger, addr *net.TCPAddr) *Server {
	// Create a new Server object and return it
	return &Server{
		logger: logger.Named("json-rpc"),
		router: mux.NewRouter(),
		server: rpc.NewServer(),
		url:    path,
		addr:   addr,
	}
}

// Start is a method of Server that starts the RPC server
func (s *Server) Start() {
	s.server.RegisterCodec(json2.NewCodec(), "application/json")
	s.router.Handle(s.url, s.server)

	// Print the server start message
	s.logger.Info(fmt.Sprintf("RPC Server started on %s:%s", s.url, s.addr))
	// Start the RPC server
	server := &http.Server{
		Addr:              s.addr.String(),
		Handler:           s.router,
		ReadHeaderTimeout: 3 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Panic(err)
	}
}

// RegisterService is a method of Server that registers a service with the RPC service.
// Accepts the service name and the service receiver and returns an error.
func (s *Server) RegisterService(name string, receiver interface{}) error {
	// Register the service with the RPC service
	return s.server.RegisterService(receiver, name)
}
