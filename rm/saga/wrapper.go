package saga

import (
	"context"
	"database/sql"

	"gorm.io/gorm"

	"github.com/ikenchina/octopus/define"
	. "github.com/ikenchina/octopus/rm/common"
)

func HandleCommit(ctx context.Context, db *gorm.DB, gtid string,
	branchID int, requestBody string, commit func(*gorm.DB) error) error {

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// check : status of try is running or is done
		txn, err := FindTransaction(tx, gtid, branchID)
		if err != nil {
			return err
		}

		if txn != nil {
			if txn.State == define.TxnStateCommitted {
				return nil
			} else if txn.State == define.TxnStateAborted {
				return ErrTxnAborted
			}
			return ErrTxnInvalidState
		}

		// execute try
		err = commit(tx)
		if err != nil {
			return err
		}

		txr := tx.Model(RmTransaction{}).Create(&RmTransaction{
			Gtid:    gtid,
			Bid:     branchID,
			State:   define.TxnStateCommitted,
			Payload: requestBody,
		})
		return txr.Error
	}, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
	})

	return err
}

func HandleCompensation(ctx context.Context, db *gorm.DB, gtid string, branchID int, compensate func(*gorm.DB, string) error) error {

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// check : status of try is running or is done
		txn, err := FindTransaction(tx, gtid, branchID)
		if err != nil {
			return err
		}

		if txn != nil {
			if txn.State == define.TxnStateAborted {
				return nil
			}
		} else {
			txr := tx.Model(RmTransaction{}).Create(&RmTransaction{
				Gtid:  gtid,
				Bid:   branchID,
				State: define.TxnStateAborted,
			})
			return txr.Error
		}

		// execute try
		err = compensate(tx, txn.Payload)
		if err != nil {
			return err
		}

		return UpdateTransactionState(tx, gtid, branchID, define.TxnStateAborted)
	}, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
	})

	return err
}
