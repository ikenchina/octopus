package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	tccrm "github.com/ikenchina/octopus/rm/tcc"
)

var (
	service_BasePath = "/transafer/tcc"
)

type RmServiceA struct {
	httpServer *http.Server
	Db         *gorm.DB
}

func (rm *RmServiceA) start(listen string, db *gorm.DB) error {
	rm.Db = db
	app := gin.New()
	app.POST(service_BasePath+"/:gtid/:branch_id", rm.tryHandler)
	app.PUT(service_BasePath+"/:gtid/:branch_id", rm.confirmHandler)
	app.DELETE(service_BasePath+"/:gtid/:branch_id", rm.cancelHandler)
	rm.httpServer = &http.Server{
		Addr:    listen,
		Handler: app,
	}
	return rm.httpServer.ListenAndServe()
}

type AccountRecord struct {
	UserID  int
	Account int
}

func (rm *RmServiceA) tryHandler(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	request := &AccountRecord{}
	err = json.Unmarshal(body, request)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	gtid := c.Param("gtid")
	branchID, _ := strconv.Atoi(c.Param("branch_id"))
	code := http.StatusOK

	//
	// execute in a transaction
	//
	err = tccrm.HandleTry(rm.Db, gtid, branchID, string(body),
		func(tx *gorm.DB) error {
			// execute try
			txr := tx.Model(Account{}).Where("id=? AND balance_freeze=0 AND balance+?>=0",
				request.UserID, request.Account).
				Update("balance_freeze", request.Account)
			if txr.Error != nil {
				code = http.StatusInternalServerError
				return txr.Error
			}
			if txr.RowsAffected == 0 {
				code = http.StatusConflict
				return fmt.Errorf("insufficient balance or freeze != 0") // avoid insufficient balance or concurrency
			}
			return nil
		})
	if err != nil {
		c.Status(code)
		_, _ = c.Writer.Write([]byte(err.Error()))
		return
	}
}

func (rm *RmServiceA) confirmHandler(c *gin.Context) {
	gtid := c.Param("gtid")
	branchID, _ := strconv.Atoi(c.Param("branch_id"))
	code := http.StatusOK

	err := tccrm.HandleConfirm(rm.Db, gtid, branchID,
		func(tx *gorm.DB, tryBody string) error {
			record := AccountRecord{}
			err := json.Unmarshal([]byte(tryBody), &record)
			if err != nil {
				code = http.StatusBadRequest
				return err
			}

			txr := tx.Model(Account{}).Where("id=?", record.UserID).
				Updates(map[string]interface{}{
					"balance":        gorm.Expr("balance+?", record.Account),
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
		c.Status(code)
		_, _ = c.Writer.Write([]byte(err.Error()))
		return
	}
}

func (rm *RmServiceA) cancelHandler(c *gin.Context) {
	gtid := c.Param("gtid")
	branchID, _ := strconv.Atoi(c.Param("branch_id"))
	code := http.StatusOK

	err := tccrm.HandleCancel(rm.Db, gtid, branchID,
		func(tx *gorm.DB, tryBody string) error {
			record := AccountRecord{}
			err := json.Unmarshal([]byte(tryBody), &record)
			if err != nil {
				code = http.StatusBadRequest
				return err
			}

			txr := tx.Model(Account{}).Where("id=?", record.UserID).Update(
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
		c.Status(code)
		_, _ = c.Writer.Write([]byte(err.Error()))
		return
	}
}

type Account struct {
	Id      int
	Balance int
	Freeze  int `gorm:"balance_freeze"`
}

func (*Account) TableName() string {
	return "dtx.account"
}

/*

CREATE TABLE IF NOT ExISTS dtx.account(
	id INT NOT NULL,
	balance INT NOT NULL DEFAULT 0,
	balance_freeze INT NOT NULL DEFAULT 0
);
*/
