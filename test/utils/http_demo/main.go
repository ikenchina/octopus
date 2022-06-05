package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ikenchina/octopus/common/runner"
)

func HealthCheck(c *gin.Context) {
	c.Status(200)
}

type AppConfig struct {
	Listen string
	Name   string
	Route  []struct {
		Method   string
		Path     string
		Status   int
		Body     string
		Duration time.Duration
	}
}

type Configs struct {
	Apps []AppConfig
}

var (
	configPath = flag.String("config", "./conf.json", "config file path")
)

func main() {
	flag.Parse()
	gin.SetMode(gin.ReleaseMode)

	cfgs := &Configs{}
	dd, err := ioutil.ReadFile(*configPath)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(dd, cfgs)
	if err != nil {
		panic(err)
	}

	for _, cfg := range cfgs.Apps {
		app := gin.New()

		app.NoRoute(func(c *gin.Context) {
			c.JSON(404, gin.H{"code": "PAGE_NOT_FOUND", "message": "Page not found"})
		})

		for _, rt := range cfg.Route {
			status := rt.Status
			body := rt.Body
			duration := rt.Duration
			if rt.Method == "*" {
				app.Any(rt.Path, func(c *gin.Context) {
					time.Sleep(duration)
					c.String(status, body)
					//log.Printf("http log : server(%v), method(%v), path(%v), code(%v), body(%s)", srvName, rt.Method, c.FullPath(), c.Writer.Status(), c.GetRawData())
				})
			} else {
				app.Handle(rt.Method, rt.Path, func(c *gin.Context) {
					time.Sleep(duration)
					c.String(status, body)
					//log.Printf("http log : server(%v), method(%v),path(%v), code(%v), body(%s)", srvName, rt.Method, c.FullPath(), c.Writer.Status(), c.GetRawData())
				})
			}
		}

		listen := cfg.Listen
		go func() {
			_ = app.Run(listen)
		}()
	}

	runner.SignalHandle(nil)
}
