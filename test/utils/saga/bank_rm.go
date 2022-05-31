package saga

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
	SagaRmBankServiceBasePath = "/paywage/saga"
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
	rm.httpServer = &http.Server{
		Addr:    rm.listen,
		Handler: app,
	}
	return rm.httpServer.ListenAndServe()
}

func (rm *SagaRmBankService) commitHandler(c *gin.Context) {

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

	if request.Fail {
		c.AbortWithStatus(http.StatusInternalServerError)
	}

	gtid := c.Param("gtid")
	branchID, _ := strconv.Atoi(c.Param("branch_id"))
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
		c.Status(code)
		_, _ = c.Writer.Write([]byte(err.Error()))
		return
	}
}

func (rm *SagaRmBankService) compensationHandler(c *gin.Context) {
	gtid := c.Param("gtid")
	branchID, _ := strconv.Atoi(c.Param("branch_id"))
	code := http.StatusOK

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
		c.Status(code)
		_, _ = c.Writer.Write([]byte(err.Error()))
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
