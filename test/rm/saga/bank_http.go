package saga

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gorm.io/gorm"

	http_util "github.com/ikenchina/octopus/common/http"
	logutil "github.com/ikenchina/octopus/common/log"
	"github.com/ikenchina/octopus/common/metrics"
	sagarm "github.com/ikenchina/octopus/rm/saga"
	pb "github.com/ikenchina/octopus/test/rm/saga/pb"
)

var (
	SagaRmBankServiceBasePath = "/paywage/saga"

	rmExecTimer = metrics.NewTimer("dtx", "saga_rm_txn", "rm timer", []string{"branch"})
)

type BankAccountRecord struct {
	UserID  int
	Account int
	Fail    bool
}

func (ba *BankAccountRecord) ToPb() *pb.SagaRequest {
	return &pb.SagaRequest{
		UserId:  int32(ba.UserID),
		Account: int32(ba.Account),
	}
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
	logutil.Logger(context.TODO()).Sugar().Debugf("saga rm http server : %v", rm.listen)

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

	gtid := c.Param("gtid")
	branchID, _ := strconv.Atoi(c.Param("branch_id"))
	request := &BankAccountRecord{}
	err := http_util.ParseHttpJsonRequest(c, request)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	//logutil.Logger(c.Request.Context()).Sugar().Debugf("commit : %s, %v, [%+v]", gtid, branchID, request)
	if request.Fail {
		c.AbortWithStatus(http.StatusInternalServerError)
	}

	code := http.StatusOK

	//
	// execute in a transaction
	//
	err = sagarm.HandleCommitOrm(c.Request.Context(), rm.db, gtid, branchID,
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
	request := &BankAccountRecord{}
	err := http_util.ParseHttpJsonRequest(c, request)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	code := http.StatusOK

	//logutil.Logger(c.Request.Context()).Sugar().Debugf("compensation : %s, %v, [%+v]",
	//	gtid, branchID, request)

	err = sagarm.HandleCompensationOrm(c.Request.Context(), rm.db, gtid, branchID,
		func(tx *gorm.DB) error {
			txr := tx.Model(BankAccount{}).Where("id=?", request.UserID).
				Update("balance", gorm.Expr("balance+?", -1*request.Account))
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
	id BIGSERIAL PRIMARY key,
	balance INT NOT NULL DEFAULT 0,
	balance_freeze INT NOT NULL DEFAULT 0
);
*/
