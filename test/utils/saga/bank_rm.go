package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gorm.io/gorm"

	logutil "github.com/ikenchina/octopus/common/log"
	"github.com/ikenchina/octopus/common/metrics"
	sagarm "github.com/ikenchina/octopus/rm/saga"
)

var (
	SagaRmBankServiceBasePath = "/paywage/saga"

	rmExecTimer = metrics.NewTimer("dtx", "rm_txn", "rm timer", []string{"branch"})
)

type BankAccountRecord struct {
	UserID  int
	Account int
	Fail    bool
}

type SagaRmBankService struct {
	httpServer *http.Server
	listen     string
	db         *gorm.DB
}

func NewSagaRmBankService(listen string, db *gorm.DB) *SagaRmBankService {
	return &SagaRmBankService{
		listen: listen,
		db:     db,
	}
}

func (rm *SagaRmBankService) Start() error {
	app := gin.New()
	app.POST(SagaRmBankServiceBasePath+"/:gtid/:branch_id", rm.commitHandler)
	app.DELETE(SagaRmBankServiceBasePath+"/:gtid/:branch_id", rm.compensationHandler)
	app.GET("/debug/metrics", gin.WrapH(promhttp.Handler()))
	rm.httpServer = &http.Server{
		Addr:    rm.listen,
		Handler: app,
	}
	return rm.httpServer.ListenAndServe()
}

func (rm *SagaRmBankService) commitHandler(c *gin.Context) {

	defer rmExecTimer.Timer()("commit")

	body, err := c.GetRawData()
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	request := &BankAccountRecord{}
	err = json.Unmarshal(body, request)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	gtid := c.Param("gtid")
	branchID, _ := strconv.Atoi(c.Param("branch_id"))

	logutil.Logger(c.Request.Context()).Sugar().Debugf("commit : %s %d", gtid, branchID)

	if request.Fail {
		c.AbortWithStatus(http.StatusInternalServerError)
	}

	code := http.StatusOK

	//
	// execute in a transaction
	//
	err = sagarm.HandleCommit(rm.db, gtid, branchID, string(body),
		func(tx *gorm.DB) error {
			txr := tx.Model(BankAccount{}).Where("id=?", request.UserID).
				Update("balance", gorm.Expr("balance+?", request.Account))
			if txr.Error != nil {
				code = http.StatusInternalServerError
				return txr.Error
			}
			if txr.RowsAffected == 0 {
				code = http.StatusNotFound
				return fmt.Errorf("user does not exist")
			}
			return nil
		})
	if err != nil {
		if code == http.StatusOK {
			code = http.StatusInternalServerError
		}
		c.Status(code)
		_, _ = c.Writer.Write([]byte(err.Error()))
		logutil.Logger(context.TODO()).Sugar().Debugf("commit err : %s %d %v", gtid, branchID, err)
		return
	}
}

func (rm *SagaRmBankService) compensationHandler(c *gin.Context) {

	defer rmExecTimer.Timer()("compensation")

	gtid := c.Param("gtid")
	branchID, _ := strconv.Atoi(c.Param("branch_id"))
	code := http.StatusOK

	logutil.Logger(c.Request.Context()).Sugar().Debugf("compensation : %s %s", gtid, branchID)

	err := sagarm.HandleCompensation(rm.db, gtid, branchID,
		func(tx *gorm.DB, body string) error {
			record := BankAccountRecord{}
			err := json.Unmarshal([]byte(body), &record)
			if err != nil {
				code = http.StatusBadRequest
				return err
			}

			txr := tx.Model(BankAccount{}).Where("id=?", record.UserID).
				Update("balance", gorm.Expr("balance+?", -1*record.Account))
			if txr.Error != nil {
				code = http.StatusInternalServerError
				return txr.Error
			}
			if txr.RowsAffected == 0 {
				code = http.StatusNotFound
				return fmt.Errorf("user does not exist")
			}
			return nil
		})

	if err != nil {
		if code == http.StatusOK {
			code = http.StatusInternalServerError
		}
		c.Status(code)
		_, _ = c.Writer.Write([]byte(err.Error()))
		logutil.Logger(context.TODO()).Sugar().Debugf("compensation err : %s %d %v", gtid, branchID, err)
		return
	}
}

type BankAccount struct {
	Id      int
	Balance int
	Freeze  int `gorm:"balance_freeze"`
}

func (*BankAccount) TableName() string {
	return "dtx.account"
}

/*

CREATE TABLE IF NOT ExISTS dtx.account(
	id INT NOT NULL,
	balance INT NOT NULL DEFAULT 0,
	balance_freeze INT NOT NULL DEFAULT 0
);
*/
