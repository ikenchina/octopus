package saga

import (
	"context"
	"database/sql"

	"gorm.io/gorm"

	"github.com/ikenchina/octopus/define"
	. "github.com/ikenchina/octopus/rm/common"
)

// HandleCommit implement commit logical of RM as a block.
//   db is database handle
//   gtid is global identifier of saga transaction
//   branchID is identifier of branch transaction
//   commit is commit logical, saga transaction will be aborted if it returns error
func HandleCommit(ctx context.Context, db *sql.DB, gtid string, branchID int,
	commit func(*sql.Tx) error) error {

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}

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

// HandleCommitOrm is same as HandleCommit
//   db is database handle of gorm.DB
func HandleCommitOrm(ctx context.Context, db *gorm.DB, gtid string, branchID int,
	commit func(*gorm.DB) error) error {

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// check : status of try is running or is done
		txn, err := FindTransactionOrm(tx, gtid, branchID)
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

// HandleCommit implement commit logical of RM as a block.
//   db is database handle
//   gtid is global identifier of saga transaction
//   branchID is identifier of branch transaction
//   commit is commit logical, saga transaction will be aborted if it returns error
func HandleCompensation(ctx context.Context, db *sql.DB, gtid string, branchID int,
	compensate func(*sql.Tx) error) error {

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}

	txn, err := FindTransaction(tx, gtid, branchID)
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

	err = UpdateTransactionState(tx, gtid, branchID, define.TxnStateAborted)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// HandleCompensationOrm is same as HandleCompensation
//   db is database handle of gorm.DB
func HandleCompensationOrm(ctx context.Context, db *gorm.DB, gtid string, branchID int,
	compensate func(*gorm.DB) error) error {

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// check : status of try is running or is done
		txn, err := FindTransactionOrm(tx, gtid, branchID)
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

		return UpdateTransactionStateOrm(tx, gtid, branchID, define.TxnStateAborted)
	}, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
	})

	return err
}
