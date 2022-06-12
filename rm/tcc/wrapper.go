package tcc

import (
	"context"
	"database/sql"

	"gorm.io/gorm"

	"github.com/ikenchina/octopus/define"
	. "github.com/ikenchina/octopus/rm/common"
)

func HandleTry(ctx context.Context, db *gorm.DB, gtid string, branchID int, tryBody string, try func(stx *gorm.DB) error) error {

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// check : status of try is running or is done
		txn, err := FindTransaction(tx, gtid, branchID)
		if err != nil {
			return err
		}

		if txn != nil {
			// transaction is already committed,
			// try has been executed
			if txn.State == define.TxnStatePrepared {
				return nil
			}
			// transaction is already aborted or committed
			// cancel has been executed
			return ErrTxnCompleted
		}

		// execute try
		err = try(tx)
		if err != nil {
			return err
		}

		txr := tx.Model(RmTransaction{}).Create(&RmTransaction{
			Gtid:    gtid,
			Bid:     branchID,
			State:   define.TxnStatePrepared,
			Payload: tryBody,
		})
		return txr.Error
	}, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
	})

	return err
}

func HandleConfirm(ctx context.Context, db *gorm.DB, gtid string, branchID int,
	confirm func(stx *gorm.DB, tryBody string) error) error {

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {

		// check : status of try is running or is done
		txn, err := FindTransaction(tx, gtid, branchID)
		if err != nil {
			return err
		}

		if txn == nil {
			return ErrTxnNotPrepared
		}

		// transaction is already committed,
		if txn.State == define.TxnStateCommitted {
			return nil
		} else if txn.State == define.TxnStateAborted {
			// transaction is already aborted, cancel has been executed
			return ErrTxnAborted
		} else if txn.State != define.TxnStatePrepared { // impossible
			return ErrTxnNotPrepared
		}

		err = confirm(tx, txn.Payload)
		if err != nil {
			return err
		}

		return UpdateTransactionState(tx, gtid, branchID, define.TxnStateCommitted)
	}, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
	})
	return err
}

func HandleCancel(ctx context.Context, db *gorm.DB, gtid string, branchID int, cancel func(stx *gorm.DB, tryBody string) error) error {

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {

		// check : status of try is running or is done
		txn, err := FindTransaction(tx, gtid, branchID)
		if err != nil {
			return err
		}

		if txn == nil {
			txr := tx.Model(RmTransaction{}).Create(&RmTransaction{
				Gtid:  gtid,
				Bid:   branchID,
				State: define.TxnStateAborted,
			})
			if txr.Error != nil {
				return txr.Error
			}
			return nil
		}

		// transaction is already committed,
		if txn.State == define.TxnStateCommitted {
			return ErrTxnCommitted
		} else if txn.State == define.TxnStateAborted {
			return nil
		}

		err = cancel(tx, txn.Payload)
		if err != nil {
			return err
		}

		return UpdateTransactionState(tx, gtid, branchID, define.TxnStateCommitted)
	}, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
	})
	return err
}
