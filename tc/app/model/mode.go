package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	logutil "github.com/ikenchina/octopus/common/log"
	"github.com/ikenchina/octopus/common/operator"
)

var (
	ErrNotExist         = errors.New("not exist")
	ErrAlreadyExist     = errors.New("already exist")
	ErrIsNotPrepared    = errors.New("state of transaction is not prepared")
	ErrLeaseExpired     = errors.New("lease expired")
	ErrExpired          = errors.New("expired")
	ErrInvalidLessee    = errors.New("invalid lessee")
	ErrNotExpectedState = errors.New("state of transaction is not expected")
)

type ModelStorage interface {
	Timeout() time.Duration

	// query
	Exist(ctx context.Context, gtid string) error
	GetByGtid(ctx context.Context, gtid string) (*Txn, error)

	// update
	Save(ctx context.Context, t *Txn) error
	Update(ctx context.Context, t *Txn) error
	UpdateConditions(ctx context.Context, txn *Txn, cb func(oldTxn *Txn) error) error
	GrantLease(ctx context.Context, txn *Txn) error
	GrantLeaseIncBranch(ctx context.Context, txn *Txn, branch *Branch, leaseDuration time.Duration) error

	// branch
	UpdateBranch(ctx context.Context, b *Branch) error
	UpdateBranchConditions(ctx context.Context, b *Branch, cb func(oldTxn *Txn, ob *Branch) error) error
	RegisterBranches(ctx context.Context, bs []*Branch) error

	// find expired transactions
	FindLeaseExpired(ctx context.Context, txn string, states []string, limit int) ([]*Txn, error)
	FindExpired(ctx context.Context, txn string, limit int) ([]*Txn, error)
	FindEnded(ctx context.Context, txn string, untileTime time.Time, limit int) ([]*Txn, error)
}

type modelStorage struct {
	Db           *gorm.DB
	timeout      time.Duration
	lessee       string
	defaultTxOpt *sql.TxOptions
}

func NewModelStorage(driver string, dsn string,
	timeout time.Duration, maxConn int, MaxIdleConn int,
	lessee string) (ModelStorage, error) {
	store := &modelStorage{}
	switch driver {
	case "postgresql":
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
			SkipDefaultTransaction: true,
			//Logger:                 logger.Default.LogMode(logger.Info),
		})
		if err != nil {
			return nil, err
		}
		store.Db = db
		sdb, err := db.DB()
		if err != nil {
			return nil, err
		}
		sdb.SetMaxOpenConns(maxConn)
		sdb.SetMaxIdleConns(MaxIdleConn)
	default:
		return nil, errors.New("unknown driver")
	}

	store.timeout = timeout
	store.lessee = lessee
	store.defaultTxOpt = &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
	}

	return store, nil
}

func (ms *modelStorage) Timeout() time.Duration {
	return ms.timeout
}

func (ms *modelStorage) Exist(ctx context.Context, gtid string) error {
	ctx, cancel := context.WithTimeout(ctx, ms.timeout)
	defer cancel()

	txn := &Txn{}
	txr := ms.Db.WithContext(ctx).Model(txn).Where("gtid=?", gtid).First(txn)
	if txr.Error != nil {
		if txr.Error == gorm.ErrRecordNotFound {
			return ErrNotExist
		} else {
			return fmt.Errorf("db error : %v", txr.Error)
		}
	}

	return nil
}

func (ms *modelStorage) GetByGtid(ctx context.Context, gtid string) (*Txn, error) {
	ctx, cancel := context.WithTimeout(ctx, ms.timeout)
	defer cancel()

	txn := &Txn{}
	err := ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txr := tx.Model(&Txn{}).Where("gtid=?", gtid).Find(txn)
		if txr.Error != nil {
			return txr.Error
		}
		if txr.RowsAffected == 0 {
			return ErrNotExist
		}

		txr = tx.Model(&Branch{}).Where("gtid=?", gtid).Order("id ASC").Find(&txn.Branches)
		if txr.Error != nil && txr.Error != gorm.ErrRecordNotFound {
			return fmt.Errorf("db error : %v", txr.Error)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return txn, err
}

// @todo lease
func (ms *modelStorage) FindExpired(ctx context.Context, txnType string, limit int) ([]*Txn, error) {
	ctx, cancel := context.WithTimeout(ctx, ms.timeout)
	defer cancel()

	states := []string{TxnStatePrepared, TxnStateFailed, TxnStateCommitting}
	txns := make([]*Txn, 0)
	err := ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {

		txa := tx.Model(&Txn{}).Where("txn_type = ? AND state IN ? AND expire_time < NOW()", txnType, states)
		txa.Order("id DESC").Limit(limit) // ORDER BY id DESC
		txr := txa.Find(&txns)
		if txr.Error != nil {
			return txr.Error
		}

		for _, sg := range txns {
			txr = tx.Model(&Branch{}).Where("gtid=?", sg.Gtid).Order("id ASC").Find(&sg.Branches)
			if txr.Error != nil {
				if txr.Error == gorm.ErrRecordNotFound {
					logutil.Logger(ctx).Sugar().Infof("Transaction has not branch : %s", sg.Gtid)
					continue
				} else {
					return fmt.Errorf("db error : %v", txr.Error)
				}
			}
		}

		return nil
	})

	return txns, err
}

func (ms *modelStorage) FindLeaseExpired(ctx context.Context, txnType string, states []string, limit int) ([]*Txn, error) {
	ctx, cancel := context.WithTimeout(ctx, ms.timeout)
	defer cancel()

	if len(states) == 0 {
		states = []string{TxnStatePrepared, TxnStateFailed, TxnStateCommitting}
	}

	txns := make([]*Txn, 0)
	err := ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txa := tx.Model(&Txn{}).Where("txn_type = ? AND state IN ? AND lease_expire_time < NOW()", txnType, states)
		txa.Order("id DESC").Limit(limit)
		txr := txa.Find(&txns)
		if txr.Error != nil {
			return txr.Error
		}

		for _, sg := range txns {
			txr = tx.Model(&Branch{}).Where("gtid=?", sg.Gtid).Order("id ASC").Find(&sg.Branches)
			if txr.Error != nil {
				if txr.Error == gorm.ErrRecordNotFound {
					logutil.Logger(ctx).Sugar().Infof("Transaction has not branch : %s", sg.Gtid)
					continue
				} else {
					return fmt.Errorf("db error : %v", txr.Error)
				}
			}
		}
		return nil
	})

	return txns, err
}

func (ms *modelStorage) FindEnded(ctx context.Context, txnType string, untileTime time.Time, limit int) ([]*Txn, error) {
	ctx, cancel := context.WithTimeout(ctx, ms.timeout)
	defer cancel()

	states := []string{TxnStateAborted, TxnStateCommitted}

	txns := make([]*Txn, 0)
	err := ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txa := tx.Model(&Txn{}).Where("txn_type = ? AND state IN ? AND lease_expire_time < ?", txnType, states, untileTime)
		txa.Order("id ASC").Limit(limit)
		txr := txa.Find(&txns)
		if txr.Error != nil {
			return txr.Error
		}
		return nil
	})

	ids := []string{}
	for _, t := range txns {
		ids = append(ids, t.Gtid)
	}

	maxDeleteLimit := 20
	for len(ids) != 0 {
		dl := operator.IfElse(len(ids) < maxDeleteLimit, len(ids), maxDeleteLimit).(int)
		did := ids[:dl]
		ids = ids[dl:]
		err = ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			txr := tx.Where("gtid IN ?", did).Delete(&Branch{})
			if txr.Error != nil {
				return txr.Error
			}

			txr = tx.Where("gtid IN ?", did).Delete(&Txn{})
			if txr.Error != nil {
				return txr.Error
			}
			return nil
		})
	}
	return txns, err
}

func (ms *modelStorage) Save(ctx context.Context, txn *Txn) error {
	ctx, cancel := context.WithTimeout(ctx, ms.timeout)
	defer cancel()

	err := ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txr := tx.Model(&Txn{}).Create(txn)
		if txr.Error != nil {
			return txr.Error
		}
		if len(txn.Branches) > 0 {
			txr = tx.Model(&Branch{}).Create(txn.Branches)
		}
		return txr.Error
	})

	return err
}

func (ms *modelStorage) UpdateConditions(ctx context.Context, txn *Txn, cb func(oldTxn *Txn) error) error {
	ctx, cancel := context.WithTimeout(ctx, ms.timeout)
	defer cancel()

	txn.BeginSave()
	err := ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		dbTxn := &Txn{}
		txr := tx.Where("gtid=? AND (expire_time < NOW() OR lessee = ? OR lease_expire_time < NOW())",
			txn.Gtid, ms.lessee).Find(dbTxn)
		if txr.Error != nil {
			return txr.Error
		}
		if txr.RowsAffected == 0 {
			return ErrInvalidLessee
		}

		err := cb(dbTxn)
		if err != nil {
			return err
		}

		txr = tx.Model(txn).Select(txn.getUpdateFields()).Updates(txn)
		if txr.Error != nil {
			return txr.Error
		}
		for _, branch := range txn.Branches {
			if len(branch.updateFields) > 0 {
				txr = tx.Model(branch).Select(branch.getUpdateFields()).Updates(branch)
				if txr.Error != nil {
					return txr.Error
				}
			}
		}
		return nil
	}, ms.defaultTxOpt)

	if err == nil {
		txn.EndSave()
	}
	return err
}

func (ms *modelStorage) GrantLease(ctx context.Context, txn *Txn) error {
	ctx, cancel := context.WithTimeout(ctx, ms.timeout)
	defer cancel()
	err := ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txr := tx.Model(Txn{}).Where("gtid=? AND (expire_time < NOW() OR lessee = ? OR lease_expire_time < NOW())",
			txn.Gtid, ms.lessee).Select([]string{"lessee", "lease_expire_time", "updated_time"}).Updates(txn)
		if txr.Error != nil {
			return txr.Error
		}
		if txr.RowsAffected == 0 {
			return ErrInvalidLessee
		}
		return nil
	}, ms.defaultTxOpt)
	return err
}

func (ms *modelStorage) GrantLeaseIncBranch(ctx context.Context, txn *Txn, branch *Branch, leaseDuration time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, ms.timeout)
	defer cancel()

	// postgreSQL
	expire := fmt.Sprintf("NOW() + interval '%v millisecond'", leaseDuration.Milliseconds())
	err := ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txr := tx.Model(Txn{}).
			Where("gtid=? AND (expire_time < NOW() OR lessee = ? OR lease_expire_time < NOW())",
				txn.Gtid, ms.lessee).
			Update("lease_expire_time", gorm.Expr(expire)).
			Update("lessee", txn.Lessee).Update("updated_time", gorm.Expr("NOW()"))
		if txr.Error != nil {
			return txr.Error
		}
		if txr.RowsAffected == 0 {
			return ErrInvalidLessee
		}

		txr = tx.Model(Branch{}).Where("id=?", branch.Id).Update("try_count", gorm.Expr("try_count + 1"))

		dbBranch := &Branch{}
		tx.Model(Branch{}).Where("id=?", branch.Id).Find(&dbBranch)
		return txr.Error
	}, ms.defaultTxOpt)
	return err
}

func (ms *modelStorage) Update(ctx context.Context, txn *Txn) error {
	ctx, cancel := context.WithTimeout(ctx, ms.timeout)
	defer cancel()

	txn.BeginSave()

	err := ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		dbTxn := &Txn{}
		txr := tx.Model(Txn{}).Where(
			"gtid=? AND (expire_time < NOW() OR lessee = ? OR lease_expire_time < NOW())",
			txn.Gtid, ms.lessee).Find(dbTxn)
		if txr.Error != nil {
			return txr.Error
		}
		if txr.RowsAffected == 0 {
			return ErrInvalidLessee
		}

		if len(txn.updateFields) > 0 {
			txr = tx.Model(txn).Select(txn.getUpdateFields()).Updates(txn)
			if txr.Error != nil {
				return txr.Error
			}
		}

		for _, branch := range txn.Branches {
			if len(branch.updateFields) > 0 {
				txr = tx.Model(branch).Select(branch.getUpdateFields()).Updates(branch)
				if txr.Error != nil {
					return txr.Error
				}
			}
		}
		return nil
	}, ms.defaultTxOpt)

	if err == nil {
		txn.EndSave()
	}
	return err
}

func (ms *modelStorage) UpdateBranch(ctx context.Context, branch *Branch) error {
	ctx, cancel := context.WithTimeout(ctx, ms.timeout)
	defer cancel()

	branch.BeginSave()
	err := ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		dbtxn := &Txn{}
		txr := tx.Where("gtid=? AND (expire_time < NOW() OR lessee = ? OR lease_expire_time < NOW())",
			branch.Gtid, ms.lessee).Find(dbtxn)
		if txr.Error != nil {
			return txr.Error
		}
		if txr.RowsAffected == 0 {
			return ErrInvalidLessee
		}

		txr = tx.Model(branch).Select(branch.getUpdateFields()).Updates(branch)
		if txr.Error != nil {
			return txr.Error
		}
		if txr.RowsAffected == 0 {
			return ErrNotExist
		}
		return nil
	}, ms.defaultTxOpt)

	if err == nil {
		branch.EndSave()
	}
	return err
}

func (ms *modelStorage) UpdateBranchConditions(ctx context.Context, branch *Branch, cb func(oldTxn *Txn, oldBranch *Branch) error) error {
	ctx, cancel := context.WithTimeout(ctx, ms.timeout)
	defer cancel()

	branch.BeginSave()

	err := ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		dbTxn := &Txn{}
		txr := tx.Where("gtid=? AND (expire_time < NOW() OR lessee = ? OR lease_expire_time < NOW())",
			branch.Gtid, ms.lessee).Find(dbTxn)
		if txr.Error != nil {
			return txr.Error
		}
		if txr.RowsAffected == 0 {
			return ErrInvalidLessee
		}

		dbBranch := &Branch{}
		txr = tx.Model(Branch{}).Where("id=?", branch.Id).First(&dbBranch)
		if txr.Error != nil {
			return tx.Error
		}

		err := cb(dbTxn, dbBranch)
		if err != nil {
			return err
		}

		txr = tx.Model(branch).Select(branch.getUpdateFields()).Updates(branch)
		if txr.Error != nil {
			return txr.Error
		}
		if txr.RowsAffected == 0 {
			return ErrNotExist
		}
		return nil
	}, ms.defaultTxOpt)

	if err == nil {
		branch.EndSave()
	}
	return err
}

func (ms *modelStorage) RegisterBranches(ctx context.Context, branches []*Branch) error {
	if len(branches) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, ms.timeout)
	defer cancel()

	err := ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		dbtxn := &Txn{}
		txr := tx.Where("gtid=? AND expire_time > NOW() and state = ?", branches[0].Gtid, TxnStatePrepared).Find(dbtxn)
		if txr.Error != nil {
			return txr.Error
		}

		if txr.RowsAffected == 0 {
			return ErrInvalidLessee
		}
		if dbtxn.State != TxnStatePrepared {
			return ErrIsNotPrepared
		}

		txr = tx.Model(&Branch{}).Create(branches)
		return txr.Error
	})
	return err
}
