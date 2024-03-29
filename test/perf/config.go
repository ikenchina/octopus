package main

import "go.uber.org/zap"

type ApConfig struct {
	Listen      string
	Dsn         string
	Qps         int
	Concurrency int
	UserRange   []int
	TxnSize     int
	FailRate    float32
	TcDomain    string
	Notify      string
}

type RmConfig struct {
	Listen             string
	Dsn                string
	MaxConnections     int
	MaxIdleConnections int
	Protocol           string
}

type Config struct {
	Log *zap.Config
	Ap  *ApConfig
	Rm  []*RmConfig
}
