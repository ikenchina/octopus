package main

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
	Listen string
	Dsn    string
}

type Config struct {
	Ap *ApConfig
	Rm []*RmConfig
}
