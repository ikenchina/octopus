package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	sagarm "github.com/ikenchina/octopus/rm/saga"
)

var (
	service_BasePath = "/paywage/saga"
)

type AccountRecord struct {
	UserID  int
	Account int
}

type RmService struct {
	httpServer *http.Server
	Db         *gorm.DB
	listen     string
	db         *gorm.DB
}

func (rm *RmService) start() error {
	app := gin.New()
	app.POST(service_BasePath+"/:gtid/:branch_id", rm.commitHandler)
	app.DELETE(service_BasePath+"/:gtid/:branch_id", rm.compensationHandler)
	rm.httpServer = &http.Server{
		Addr:    rm.listen,
		Handler: app,
	}
	return rm.httpServer.ListenAndServe()
}

func (rm *RmService) commitHandler(c *gin.Context) {
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
	err = sagarm.HandleCommit(rm.Db, gtid, branchID, string(body),
		func(tx *gorm.DB) error {
			txr := tx.Model(Account{}).Where("id=?", request.UserID).
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
		c.Status(code)
		_, _ = c.Writer.Write([]byte(err.Error()))
		return
	}
}

func (rm *RmService) compensationHandler(c *gin.Context) {
	gtid := c.Param("gtid")
	branchID, _ := strconv.Atoi(c.Param("branch_id"))
	code := http.StatusOK

	err := sagarm.HandleCompensation(rm.Db, gtid, branchID,
		func(tx *gorm.DB, body string) error {
			record := AccountRecord{}
			err := json.Unmarshal([]byte(body), &record)
			if err != nil {
				code = http.StatusBadRequest
				return err
			}

			txr := tx.Model(Account{}).Where("id=?", record.UserID).
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
