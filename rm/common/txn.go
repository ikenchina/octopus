package common

import (
	"errors"

	"gorm.io/gorm"
)

var (
	dbSchema = ""
)

var (
	ErrTxnCompleted    = errors.New("transaction is already aborted or committed")
	ErrTxnAborted      = errors.New("transaction is already aborted")
	ErrTxnCommitted    = errors.New("transaction is already committed")
	ErrTxnNotPrepared  = errors.New("transaction is not prepared")
	ErrTxnInvalidState = errors.New("invalid state")

	ErrNotExist = errors.New("not exist")
)

func SetDbSchema(schema string) {
	dbSchema = schema
}

// transaction
type RmTransaction struct {
	Gtid  string
	Bid   int `gorm:"branch_id"`
	State string
	Body  string
}

func (*RmTransaction) TableName() string {
	if len(dbSchema) == 0 {
		return "rmtransaction"
	}
	return dbSchema + ".rmtransaction"
}

func FindTransaction(tx *gorm.DB, gtid string, branch int) (*RmTransaction, error) {
	txn := RmTransaction{}
	txr := tx.Model(RmTransaction{}).Where("gtid=? AND bid=?", gtid, branch).Find(&txn)
	if txr.Error != nil {
		return nil, txr.Error
	}
	if txr.RowsAffected == 0 {
		return nil, nil
	}
	return &txn, nil
}

func UpdateTransactionState(tx *gorm.DB, gtid string, branch int, state string) error {
	txr := tx.Model(RmTransaction{}).Where("gtid=? AND bid=?",
		gtid, branch).Update("state", state)
	return txr.Error
}
