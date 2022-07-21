package common

import (
	"database/sql"
	"errors"

	"gorm.io/gorm"
)

var (
	dbSchema = "dtx"
)

var (
	ErrTxnCompleted    = errors.New("transaction is already aborted or committed")
	ErrTxnAborted      = errors.New("transaction is already aborted")
	ErrTxnCommitted    = errors.New("transaction is already committed")
	ErrTxnNotPrepared  = errors.New("transaction is not prepared")
	ErrTxnInvalidState = errors.New("invalid state")
	ErrNotExist        = errors.New("not exist")
)

func SetDbSchema(schema string) {
	dbSchema = schema
}

// transaction
type RmTransaction struct {
	Id    int64
	Gtid  string
	Bid   int
	State string
}

func (*RmTransaction) TableName() string {
	if len(dbSchema) == 0 {
		return "rmtransaction"
	}
	return dbSchema + ".rmtransaction"
}

func FindTransactionRaw(tx *sql.Tx, gtid string, branch int) (*RmTransaction, error) {
	t := &RmTransaction{}
	row := tx.QueryRow("SELECT id, gtid, bid, state FROM $1 WHERE gtid=$2 AND bid=$3", t.TableName(), gtid, branch)
	err := row.Scan(&t.Id, &t.Gtid, &t.Bid, &t.State)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
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

func UpdateTransactionStateRaw(tx *sql.Tx, gtid string, branch int, state string) error {
	rm := RmTransaction{}
	rx, err := tx.Exec("UPDATE $1 SET state=$2 WHERE gtid=$3 AND bid=$4", rm.TableName(), state, gtid, branch)
	if err != nil {
		return err
	}
	ar, err := rx.RowsAffected()
	if err != nil {
		return err
	}
	if ar == 0 {
		return ErrNotExist
	}
	return nil
}

func CreateTransaction(tx *sql.Tx, gtid string, branch int, state string) error {
	tr := RmTransaction{}
	_, err := tx.Exec("INSERT INTO $1(gtid, bid, state) VALUES($2, $3, $4)", tr.TableName(), gtid, branch, state)
	return err
}
