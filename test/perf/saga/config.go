package main

import "go.uber.org/zap"

type ApConfig struct {
	Listen      string
	Dsn         string
	Qps         int
	Concurrency int
	UserRange   []int
	SagaSize    int
	FailRate    float32
	TcDomain    string
}

type RmConfig struct {
	Listen             string
	Dsn                string
	MaxConnections     int
	MaxIdleConnections int
}

type Config struct {
	Log *zap.Config
	Ap  *ApConfig
	Rm  []*RmConfig
}
