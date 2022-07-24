package tcc

import (
	"context"
	"database/sql"

	"gorm.io/gorm"

	"github.com/ikenchina/octopus/define"
	. "github.com/ikenchina/octopus/rm/common"
)

// HandleTry implement try logical of RM as a block.
//   db is database handle
//   gtid is global identifier of TCC transaction
//   branchID is identifier of branch transaction
//   try is try logical, TCC transaction will be aborted if it returns error
func HandleTry(ctx context.Context, db *sql.DB, gtid string, branchID int,
	try func(stx *sql.Tx) error) error {

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}

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
	err = CreateTransaction(tx, gtid, branchID, define.TxnStatePrepared)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// HandleTryOrm is same as HandleTry
//   db is database handle of gorm.DB
func HandleTryOrm(ctx context.Context, db *gorm.DB, gtid string, branchID int,
	try func(stx *gorm.DB) error) error {

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// check : status of try is running or is done
		txn, err := FindTransactionOrm(tx, gtid, branchID)
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
			Gtid:  gtid,
			Bid:   branchID,
			State: define.TxnStatePrepared,
		})
		return txr.Error
	}, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
	})

	return err
}

// HandleConfirm implement confirm logical of RM as a block.
//   db is database handle
//   gtid is global identifier of TCC transaction
//   branchID is identifier of branch transaction
//   confirm is confirm logical, TCC transaction will be aborted if it returns error
func HandleConfirm(ctx context.Context, db *sql.DB, gtid string, branchID int,
	confirm func(*sql.Tx) error) error {

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}

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

	err = confirm(tx)
	if err != nil {
		return err
	}
	err = UpdateTransactionState(tx, gtid, branchID, define.TxnStateCommitted)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// HandleConfirmOrm is same as HandleConfirm
//   db is database handle of gorm.DB
func HandleConfirmOrm(ctx context.Context, db *gorm.DB, gtid string, branchID int,
	confirm func(stx *gorm.DB) error) error {

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {

		// check : status of try is running or is done
		txn, err := FindTransactionOrm(tx, gtid, branchID)
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

		err = confirm(tx)
		if err != nil {
			return err
		}

		return UpdateTransactionStateOrm(tx, gtid, branchID, define.TxnStateCommitted)
	}, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
	})
	return err
}

// HandleCancel implement cancel logical of RM as a block.
//   db is database handle
//   gtid is global identifier of TCC transaction
//   branchID is identifier of branch transaction
//   cancel is cancel logical, TCC transaction will be aborted if it returns error
func HandleCancel(ctx context.Context, db *sql.DB, gtid string, branchID int,
	cancel func(*sql.Tx) error) error {

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}

	txn, err := FindTransaction(tx, gtid, branchID)
	if err != nil {
		return err
	}

	if txn == nil {
		err = CreateTransaction(tx, gtid, branchID, define.TxnStateAborted)
		if err != nil {
			return err
		}
		return nil
	}

	// transaction is already committed,
	if txn.State == define.TxnStateCommitted {
		return ErrTxnCommitted
	} else if txn.State == define.TxnStateAborted {
		return nil
	}

	err = cancel(tx)
	if err != nil {
		return err
	}

	err = UpdateTransactionState(tx, gtid, branchID, define.TxnStateAborted)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// HandleCancelOrm is same as HandleCancel
//   db is database handle of gorm.DB
func HandleCancelOrm(ctx context.Context, db *gorm.DB, gtid string, branchID int,
	cancel func(stx *gorm.DB) error) error {

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {

		// check : status of try is running or is done
		txn, err := FindTransactionOrm(tx, gtid, branchID)
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

		err = cancel(tx)
		if err != nil {
			return err
		}

		return UpdateTransactionStateOrm(tx, gtid, branchID, define.TxnStateAborted)
	}, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
	})
	return err
}
