package bgclient

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sarvalabs/battleground/common"
	"github.com/sarvalabs/battleground/types"
)

type Config struct {
	CloudCfg      types.CloudConfig
	ClusterConfig *ClusterConfig
	ServerConfigs []*ServerConfig
	Network       NetworkType
	EndPoint      string
	DialTimeout   time.Duration
}

type ServerConfig struct {
	Name              string
	JSONRPCPort       int64
	LogLevel          string
	DataDir           string
	OperatorSlots     int64
	ValidatorSlots    int64
	GenesisPath       string
	ConfigPath        string
	CleanDB           string
	DiscoveryInterval string
}

type ClusterConfig struct {
	BootNode            *Node
	BootNodePort        int
	BootstrapID         string
	MOIpodBinary        string
	McutilsBinary       string
	LogLevel            string
	ConfigPath          string
	Libp2pPort          int64
	JSONRPCPort         int64
	EnableDebugMode     bool
	ValidatorCount      int
	NonValidatorCount   int
	OperatorCount       int
	BootNodeCount       int
	TempDir             string
	GenesisAccountCount int
	PremineAmount       int
	GenesisAssetCount   int
	BehaviouralCount    int
	RandomCount         int
	LogsDir             string
	WithLogs            bool
	logsDirOnce         sync.Once
	WithStdout          bool
	GuardianPath        string
	ShouldExecute       bool
	OldState            bool
}

func DefaultClusterConfig() *ClusterConfig {
	return &ClusterConfig{
		MOIpodBinary:        "moipod",
		McutilsBinary:       "mcutils",
		LogLevel:            "DEBUG",
		Libp2pPort:          6000,
		JSONRPCPort:         1600,
		BootNodePort:        5000,
		EnableDebugMode:     true,
		ValidatorCount:      20,
		OperatorCount:       1,
		BootNodeCount:       1,
		TempDir:             "./tmp",
		GenesisAccountCount: 15,
		GenesisAssetCount:   1,
		PremineAmount:       300000,
		BehaviouralCount:    1,
		RandomCount:         1,
		WithLogs:            true,
		WithStdout:          true,
		GuardianPath:        "./",
		ShouldExecute:       true,
	}
}

func (c *ClusterConfig) GetOpenPortForServer() int64 {
	return atomic.AddInt64(&c.JSONRPCPort, 1)
}

func (c *ClusterConfig) Dir(name string) string {
	return filepath.Join(c.TempDir, name)
}

func (c *ClusterConfig) GuardianPathDir(name string) string {
	return filepath.Join(c.GuardianPath, name)
}

func (c *ClusterConfig) initLogsDir() {
	logsDir := c.Dir(fmt.Sprintf("e2e-logs-%d", time.Now().UTC().UnixMilli()))

	if err := common.CreateDirSafe(logsDir, 0o0750); err != nil {
		log.Fatal(err)
	}

	c.LogsDir = logsDir
}

func (c *ClusterConfig) GetStdout(name string, custom ...io.Writer) io.Writer {
	writers := make([]io.Writer, 0)

	if c.WithLogs {
		c.logsDirOnce.Do(
			func() {
				c.initLogsDir()
			},
		)

		f, err := os.OpenFile(filepath.Join(c.LogsDir, name+".log"), os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o0600)
		if err != nil {
			log.Fatal(err)
		}

		writers = append(writers, f)
	}

	if c.WithStdout {
		writers = append(writers, os.Stdout)
	}

	if len(custom) > 0 {
		writers = append(writers, custom...)
	}

	if len(writers) == 0 {
		return io.Discard
	}

	return io.MultiWriter(writers...)
}
