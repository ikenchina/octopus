package ap

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func (app *Application) InitHttp(listen string) error {
	app.notifyUrl = fmt.Sprintf("http://localhost%s/saga/notify", listen)
	ginApp := gin.New()
	ginApp.POST("/saga/notify", app.notifyHandler)
	ginApp.GET("/debug/metrics", gin.WrapH(promhttp.Handler()))
	pprof.Register(ginApp, "debug/pprof")
	app.httpServer = &http.Server{
		Addr:    listen,
		Handler: ginApp,
	}
	return app.httpServer.ListenAndServe()
}

type Bank struct {
	Target   string
	Port     string
	Protocol string
}

type Application struct {
	httpServer *http.Server
	banks      []Bank
	mutex      sync.RWMutex
	notifyUrl  string
}

func (app *Application) AddBank(bank Bank) {
	app.banks = append(app.banks, bank)
}

func (app *Application) notifyHandler(c *gin.Context) {
	// update database : distributed transaction is commited or aborted

	_, err := c.GetRawData()
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	//logutil.Logger(context.TODO()).Sugar().Debugf("notify : %s", string(body))
}

func (app *Application) saveResponse(r []byte) {
	//logutil.Logger(context.TODO()).Sugar().Debugf("saveResponse : %s", string(r))
}

func (app *Application) saveGtidToDb(gtid string) {
	// save gtid to database, query tc when recovering
	// or wait for transaction to expire
}

func (app *Application) updateStateToDb(gtid string, state string) {
	//logutil.Logger(context.TODO()).Sugar().Debugf("updateTccStateToDb : %s %s", gtid, state)
}

func jsonMarshal(user interface{}) []byte {
	b, _ := json.Marshal(user)
	return (b)
}
