package bgclient

import (
	"fmt"
	"log"
	"strconv"

	"github.com/sarvalabs/go-legacy-kramaid"
)

const hostIP = "0.0.0.0"

type ServerConfigCallback func(*ServerConfig)

type Server struct {
	Node          *Node
	ClusterConfig *ClusterConfig
	Config        *ServerConfig
	KramaID       kramaid.KramaID
}

func NewServer(clusterConfig *ClusterConfig, serverConfig *ServerConfig) *Server {
	srv := &Server{
		ClusterConfig: clusterConfig,
		Config:        serverConfig,
	}

	srv.Start(clusterConfig.EnableDebugMode)

	return srv
}

func (server *Server) JSONRPCAddr() string {
	return fmt.Sprintf("http://%s:%d", hostIP, server.Config.JSONRPCPort)
}

func (server *Server) Start(debugMode bool) {
	config := server.Config

	// Build arguments
	args := []string{
		"server",
		// add data dir
		"--data-dir", config.DataDir,
		// add log level
		"--log-level", config.LogLevel,
		// add discovery interval
		"--discovery-interval", config.DiscoveryInterval,
		// add clean db
		"--clean-db", config.CleanDB,
		// add config path
		"--config-path", config.ConfigPath,
		// add genesis path
		"--genesis-path", config.GenesisPath,
		// add operator slots
		"--operator-slots", fmt.Sprintf("%d", config.OperatorSlots),
		// add validator slots
		"--validator-slots", fmt.Sprintf("%d", config.ValidatorSlots),
		// add debug mode
		"--enable-debug-mode", strconv.FormatBool(debugMode),
	}

	// start the server
	stdout := server.ClusterConfig.GetStdout(server.Config.Name)

	node, err := NewNode(server.ClusterConfig.MOIpodBinary, args, stdout)
	if err != nil {
		log.Fatal(err)
	}

	server.Node = node
}

func (server *Server) Stop() {
	err := server.Node.Stop()
	if err != nil {
		log.Fatal(err)
	}

	server.Node = nil
}

func (server *Server) isRunning() bool {
	return server.Node != nil
}
