package tcc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gorm.io/gorm"

	httputil "github.com/ikenchina/octopus/common/http"
	logutil "github.com/ikenchina/octopus/common/log"

	"github.com/ikenchina/octopus/common/metrics"
	tccrm "github.com/ikenchina/octopus/rm/tcc"
)

var (
	TccRmBankServiceBasePath = "/transfer/tcc"

	rmExecTimer = metrics.NewTimer("dtx", "tcc_rm_txn", "rm timer", []string{"branch"})
)

type BankAccountRecord struct {
	UserID  int
	Account int
	Fail    bool
}

type TccRmBankService struct {
	httpServer *http.Server
	listen     string
	db         *gorm.DB
}

func NewTccRmBankService(listen string, db *gorm.DB) *TccRmBankService {
	return &TccRmBankService{
		listen: listen,
		db:     db,
	}
}

func (rm *TccRmBankService) Start() error {
	app := gin.New()
	app.POST(TccRmBankServiceBasePath+"/:gtid/:branch_id", rm.tryHandler)
	app.PUT(TccRmBankServiceBasePath+"/:gtid/:branch_id", rm.confirmHandler)
	app.DELETE(TccRmBankServiceBasePath+"/:gtid/:branch_id", rm.cancelHandler)
	app.GET("/debug/metrics", gin.WrapH(promhttp.Handler()))
	rm.httpServer = &http.Server{
		Addr:    rm.listen,
		Handler: app,
	}
	return rm.httpServer.ListenAndServe()
}

func (rm *TccRmBankService) tryHandler(c *gin.Context) {
	defer rmExecTimer.Timer()("try")

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
	logutil.Logger(c.Request.Context()).Sugar().Debugf("try : %s %d", gtid, branchID)
	if request.Fail {
		c.AbortWithStatus(http.StatusInternalServerError)
	}

	code := http.StatusOK

	//
	// execute in a transaction
	//
	err = tccrm.HandleTryOrm(c.Request.Context(), rm.db, gtid, branchID,
		func(tx *gorm.DB) error {
			txr := tx.Model(BankAccount{}).
				//Where("id=? AND balance_freeze=0 AND balance+?>=0",
				Where("id=? AND balance_freeze=0 ",
					request.UserID).
				Update("balance_freeze", request.Account)

			if txr.Error != nil {
				code = http.StatusInternalServerError
				return txr.Error
			}
			if txr.RowsAffected == 0 {
				code = http.StatusConflict
				return fmt.Errorf("insufficient balance or freeze != 0")
			}
			return nil
		})
	if err != nil {
		if code == http.StatusOK {
			code = http.StatusInternalServerError
		}
		c.Status(code)
		_, _ = c.Writer.Write([]byte(err.Error()))
		logutil.Logger(context.TODO()).Sugar().Errorf("try err : %s %d %v", gtid, branchID, err)
		return
	}

	logutil.Logger(c.Request.Context()).Sugar().Debugf("http try : %s, %v, %v", gtid, branchID, err)

}

func (rm *TccRmBankService) confirmHandler(c *gin.Context) {
	defer rmExecTimer.Timer()("confirm")

	gtid := c.Param("gtid")
	branchID, _ := strconv.Atoi(c.Param("branch_id"))
	logutil.Logger(c.Request.Context()).Sugar().Debugf("confirm : %s %d", gtid, branchID)
	code := http.StatusOK

	request := &BankAccountRecord{}
	err := httputil.ParseHttpJsonRequest(c, request)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	//
	// execute in a transaction
	//
	err = tccrm.HandleConfirmOrm(c.Request.Context(), rm.db, gtid, branchID,
		func(tx *gorm.DB) error {
			// 将用户冻结资金加到账户余额中，同时清空冻结资金列
			//
			txr := tx.Model(BankAccount{}).Where("id=?", request.UserID).
				Updates(map[string]interface{}{
					"balance":        gorm.Expr("balance+balance_freeze"),
					"balance_freeze": 0})
			if txr.Error != nil {
				code = http.StatusInternalServerError
				return txr.Error
			}
			if txr.RowsAffected == 0 {
				code = http.StatusInternalServerError
				return fmt.Errorf("internal error")
			}
			return nil

		})
	if err != nil {
		if code == http.StatusOK {
			code = http.StatusInternalServerError
		}
		c.Status(code)
		_, _ = c.Writer.Write([]byte(err.Error()))
		logutil.Logger(context.TODO()).Sugar().Errorf("confirm err : %s %d %v", gtid, branchID, err)
		return
	}

	logutil.Logger(c.Request.Context()).Sugar().Debugf("http confirm : %s, %v, %v", gtid, branchID, err)

}

func (rm *TccRmBankService) cancelHandler(c *gin.Context) {
	defer rmExecTimer.Timer()("cancel")

	gtid := c.Param("gtid")
	branchID, _ := strconv.Atoi(c.Param("branch_id"))
	code := http.StatusOK
	logutil.Logger(c.Request.Context()).Sugar().Debugf("cancel : %s %s", gtid, branchID)

	request := &BankAccountRecord{}
	err := httputil.ParseHttpJsonRequest(c, request)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	err = tccrm.HandleCancelOrm(c.Request.Context(), rm.db, gtid, branchID,
		func(tx *gorm.DB) error {
			// 取消事务，将冻结资金列清空
			txr := tx.Model(BankAccount{}).Where("id=?", request.UserID).Update(
				"balance_freeze", 0)
			if txr.Error != nil {
				code = http.StatusInternalServerError
				return txr.Error
			}
			if txr.RowsAffected == 0 {
				code = http.StatusInternalServerError
				return fmt.Errorf("internal error")
			}
			return nil
		})

	if err != nil {
		if code == http.StatusOK {
			code = http.StatusInternalServerError
		}
		c.Status(code)
		_, _ = c.Writer.Write([]byte(err.Error()))
		logutil.Logger(context.TODO()).Sugar().Errorf("cancel err : %s %d %v", gtid, branchID, err)
		return
	}

	logutil.Logger(c.Request.Context()).Sugar().Debugf("cancel try : %s, %v, %v", gtid, branchID, err)

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
	id BIGSERIAL PRIMARY key,
	balance INT NOT NULL DEFAULT 0,
	balance_freeze INT NOT NULL DEFAULT 0
);
*/
