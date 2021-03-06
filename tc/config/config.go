package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"time"

	"go.uber.org/zap"

	logutil "github.com/ikenchina/octopus/common/log"
)

type StorageConfig struct {
	Driver                    string
	Dsn                       string
	MaxConnections            int
	MaxIdleConnections        int
	Timeout                   time.Duration
	CheckLeaseExpiredDuration time.Duration
	LeaseExpiredLimit         int
	CleanExpired              time.Duration
	CleanLimit                int
	CheckExpiredDuration      time.Duration
}

type NodeConfig struct {
	NodeId       int
	DataCenterId int
}

type Config struct {
	Node                NodeConfig
	HttpListen          string
	GrpcListen          string
	MaxConcurrentTask   int
	MaxConcurrentBranch int
	Storages            map[string]StorageConfig
	Log                 zap.Config
}

var (
	cfg Config
)

func Get() *Config {
	return &cfg
}

func InitConfig(configPath string) error {
	dd, err := ioutil.ReadFile(configPath)
	if err != nil {
		return err
	}

	err = json.Unmarshal(dd, &cfg)
	if err != nil {
		return err
	}

	if cfg.Node.NodeId == 0 || cfg.Node.DataCenterId == 0 {
		dcStr := os.Getenv("OCTOPUS_TC_DATACENTER_ID")
		ndStr := os.Getenv("OCTOPUS_TC_NODE_ID")
		if len(dcStr) == 0 || len(ndStr) == 0 {
			return errors.New("environment variable is missing : OCTOPUS_TC_DATACENTER_ID or OCTOPUS_TC_NODE_ID")
		}
		dc, err := strconv.ParseInt(dcStr, 10, 32)
		if err != nil {
			return err
		}
		nd, err := strconv.ParseInt(ndStr, 10, 32)
		if err != nil {
			return err
		}
		cfg.Node.DataCenterId = int(dc)
		cfg.Node.NodeId = int(nd)
	}
	if len(cfg.Storages) == 0 {
		stor := os.Getenv("OCTOPUS_STORAGES")
		err = json.Unmarshal([]byte(stor), &cfg.Storages)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error : %s", err.Error())
			return err
		}
	}

	err = logutil.InitLog(&cfg.Log)
	if err != nil {
		return err
	}
	return nil
}
