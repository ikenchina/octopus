package saga

import (
	"context"
	"database/sql"

	"gorm.io/gorm"

	"github.com/ikenchina/octopus/define"
	. "github.com/ikenchina/octopus/rm/common"
)

func HandleCommitRaw(ctx context.Context, db *sql.DB, gtid string, branchID int,
	commit func(*sql.Tx) error) error {

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}

	txn, err := FindTransactionRaw(tx, gtid, branchID)
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
	err = commit(tx)
	if err != nil {
		return err
	}

	err = CreateTransaction(tx, gtid, branchID, define.TxnStateCommitted)
	if err != nil {
		return err
	}

	err = tx.Commit()
	return err
}

func HandleCommit(ctx context.Context, db *gorm.DB, gtid string, branchID int,
	commit func(*gorm.DB) error) error {

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
			Gtid:  gtid,
			Bid:   branchID,
			State: define.TxnStateCommitted,
		})
		return txr.Error
	}, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
	})

	return err
}

func HandleCompensationRaw(ctx context.Context, db *sql.DB, gtid string, branchID int,
	compensate func(*sql.Tx) error) error {

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}

	txn, err := FindTransactionRaw(tx, gtid, branchID)
	if err != nil {
		return err
	}

	if txn != nil {
		if txn.State == define.TxnStateAborted {
			return nil
		}
	} else {
		err = CreateTransaction(tx, gtid, branchID, define.TxnStateAborted)
		if err != nil {
			return err
		}
	}

	// execute try
	err = compensate(tx)
	if err != nil {
		return err
	}

	err = UpdateTransactionStateRaw(tx, gtid, branchID, define.TxnStateAborted)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func HandleCompensation(ctx context.Context, db *gorm.DB, gtid string, branchID int,
	compensate func(*gorm.DB) error) error {

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
		err = compensate(tx)
		if err != nil {
			return err
		}

		return UpdateTransactionState(tx, gtid, branchID, define.TxnStateAborted)
	}, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
	})

	return err
}
