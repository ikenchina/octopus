package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ikenchina/octopus/common/http"
	"github.com/ikenchina/octopus/service"
	"go.uber.org/ratelimit"
)

var (
	configPath  = flag.String("config", "./case.json", "config file path")
	limiter     = flag.Int("ratelimit", 500, "rate limit")
	concurrency = flag.Int("concurrency", 4, "concurrency")
	serveraddr  = flag.String("server", "localhost:8080", "saga server")
)

var (
	sagaTemplate string
)

func newSagaGtid() (string, error) {
	_, body, err := http.Send(context.Background(), "GET", fmt.Sprintf("http://%s/dtx/saga/gtid", *serveraddr), "")
	if err != nil {
		return "", err
	}

	resp := &service.SagaResponse{}
	err = json.Unmarshal([]byte(body), resp)
	if err != nil {
		return "", err
	}

	return resp.Gtid, nil
}

var (
	counter     uint64
	counterTime = time.Now()
	locker      sync.Mutex
)

func sagaDo() {
	gtid, err := newSagaGtid()
	if err != nil {
		fmt.Println(err)
		return
	}

	sagar := &service.SagaRequest{}

	sagars := strings.ReplaceAll(sagaTemplate, "$gtid", gtid)
	err = json.Unmarshal([]byte(sagars), sagar)
	if err != nil {
		fmt.Println(err)
		return
	}
	body, _ := json.Marshal(sagar)

	url := fmt.Sprintf("http://%s/dtx/saga", *serveraddr)
	code, _, err := http.Send(context.Background(), "POST", url, string(body))
	if err != nil {
		fmt.Println(err)
		return
	}

	cc := atomic.AddUint64(&counter, 1)
	if cc%100 == 0 {
		locker.Lock()
		dur := time.Since(counterTime)
		fmt.Println("counter : ", cc, "  duration : ", dur.String())
		counterTime = time.Now()
		locker.Unlock()
	}

	if code != 200 {
		fmt.Println(url)
		fmt.Println("code is not 200 : ", code)
		return
	}
}

func main() {
	flag.Parse()

	rl := ratelimit.New(*limiter)

	dd, err := ioutil.ReadFile(*configPath)
	if err != nil {
		panic(err)
	}
	sagaTemplate = string(dd)

	wait := sync.WaitGroup{}
	for i := 0; i < *concurrency; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for {
				rl.Take()
				sagaDo()
			}
		}()
	}
	wait.Wait()
}
