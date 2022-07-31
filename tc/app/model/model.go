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
	"github.com/ikenchina/octopus/common/metrics"
	"github.com/ikenchina/octopus/common/operator"
	"github.com/ikenchina/octopus/define"
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

var (
	sagaModelTimer = metrics.NewTimer("dtx", "saga_model", "saga model timer", []string{"op", "ret"})
)

type ModelStorage interface {
	Timeout() time.Duration
	SetTimeout(t time.Duration)

	// query
	Exist(ctx context.Context, gtid string) error
	GetByGtid(ctx context.Context, gtid string) (*Txn, error)

	// update
	Save(ctx context.Context, t *Txn) error
	Update(ctx context.Context, t *Txn) error
	UpdateConditions(ctx context.Context, txn *Txn, cb func(oldTxn *Txn) error) error
	UpdateStateConditions(ctx context.Context, txn *Txn, cb func(oldTxn *Txn) error) (err error)
	GrantLease(ctx context.Context, txn *Txn, lease time.Duration) error
	GrantLeaseIncBranchCheckState(ctx context.Context, txn *Txn, branch *Branch,
		leaseDuration time.Duration, states []string) (err error)

	// branch
	UpdateBranch(ctx context.Context, b *Branch) error
	UpdateBranchConditions(ctx context.Context, b *Branch, cb func(oldTxn *Txn, ob *Branch) error) error
	RegisterBranches(ctx context.Context, bs []*Branch) error

	// find expired transactions
	FindRunningLeaseExpired(ctx context.Context, txn string, limit int) ([]*Txn, error)
	FindPreparedExpired(ctx context.Context, txn string, limit int) ([]*Txn, error)
	CleanExpiredTxns(ctx context.Context, txn string, untileTime time.Time, limit int) ([]*Txn, error)
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

func (ms *modelStorage) SetTimeout(t time.Duration) {
	ms.timeout = t
}

func (ms *modelStorage) timeoutContext(ctx context.Context) (context.Context, context.CancelFunc) {
	_, ok := ctx.Deadline()
	if ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, ms.timeout)
}

func (ms *modelStorage) Exist(ctx context.Context, gtid string) (err error) {
	defer sagaModelTimer.Timer()("Exist", operator.IfElse((err != nil), "err", "ok").(string))

	ctx, cancel := ms.timeoutContext(ctx)
	defer cancel()

	txn := &Txn{}
	err = ms.Db.WithContext(ctx).Model(txn).Where("gtid=?", gtid).First(txn).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrNotExist
		} else {
			return fmt.Errorf("db error : %v", err)
		}
	}

	return nil
}

func (ms *modelStorage) GetByGtid(ctx context.Context, gtid string) (txn *Txn, err error) {
	defer sagaModelTimer.Timer()("GetByGtid", operator.IfElse((err != nil), "err", "ok").(string))

	ctx, cancel := ms.timeoutContext(ctx)
	defer cancel()

	txn = &Txn{}
	err = ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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

// clean it : expired, commited,aborted
// rolling it : expired, prepared
// execute it : lease expired, not committed,aborted

//
// expired, and state is prepared
func (ms *modelStorage) FindPreparedExpired(ctx context.Context, txnType string, limit int) (txns []*Txn, err error) {
	defer sagaModelTimer.Timer()("FindPreparedExpired", operator.IfElse((err != nil), "err", "ok").(string))

	ctx, cancel := ms.timeoutContext(ctx)
	defer cancel()

	txns = make([]*Txn, 0)
	err = ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {

		txa := tx.Model(&Txn{}).Where("txn_type = ? AND state='prepared' AND expire_time < NOW()", txnType)
		txa.Order("expire_time DESC").Limit(limit) // ORDER BY id DESC
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

// lease is expired, and state is not one of commmitted, aborted, prepared
func (ms *modelStorage) FindRunningLeaseExpired(ctx context.Context, txnType string, limit int) (txns []*Txn, err error) {
	defer sagaModelTimer.Timer()("FindRunningLeaseExpired", operator.IfElse((err != nil), "err", "ok").(string))

	ctx, cancel := ms.timeoutContext(ctx)
	defer cancel()

	txns = make([]*Txn, 0)
	err = ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txa := tx.Model(&Txn{}).Where("txn_type = ? AND state NOT IN ('committed', 'aborted', 'prepared') AND lease_expire_time < NOW()", txnType)
		txa.Order("lease_expire_time DESC").Limit(limit)
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

// expired, and state is committed or aborted
func (ms *modelStorage) CleanExpiredTxns(ctx context.Context, txnType string, untilTime time.Time, limit int) (txns []*Txn, err error) {
	defer sagaModelTimer.Timer()("CleanExpiredTxns", operator.IfElse((err != nil), "err", "ok").(string))

	ctx, cancel := ms.timeoutContext(ctx)
	defer cancel()

	txns = make([]*Txn, 0)
	err = ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txa := tx.Model(&Txn{}).Where("txn_type = ? AND state IN ('committed', 'aborted') AND expire_time < ?", txnType, untilTime)
		txa.Order("expire_time ASC").Limit(limit)
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

func (ms *modelStorage) Save(ctx context.Context, txn *Txn) (err error) {
	defer sagaModelTimer.Timer()("Save", operator.IfElse((err != nil), "err", "ok").(string))
	ctx, cancel := ms.timeoutContext(ctx)
	defer cancel()

	err = ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txr := tx.Model(&Txn{}).Create(txn)
		if txr.Error != nil {
			return txr.Error
		}
		if len(txn.Branches) > 0 {
			txr = tx.Model(&Branch{}).Create(txn.Branches)
		}
		return txr.Error
	})

	txn.EndSave()
	return err
}

func (ms *modelStorage) UpdateStateConditions(ctx context.Context, txn *Txn,
	cb func(oldTxn *Txn) error) (err error) {
	defer sagaModelTimer.Timer()("UpdateStateConditions", operator.IfElse((err != nil), "err", "ok").(string))

	ctx, cancel := ms.timeoutContext(ctx)
	defer cancel()

	txn.BeginSave()
	err = ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		dbTxn := &Txn{}
		txr := tx.Where("gtid=? AND (lessee = ? OR lease_expire_time < NOW())",
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

		txr = tx.Model(&Txn{}).Where("gtid=?", txn.Gtid).
			Updates(map[string]interface{}{"state": txn.State, "updated_time": gorm.Expr("NOW()")})
		return txr.Error
	}, ms.defaultTxOpt)

	if err == nil {
		txn.EndSave()
	}
	return err
}

func (ms *modelStorage) UpdateConditions(ctx context.Context, txn *Txn, cb func(oldTxn *Txn) error) (err error) {
	defer sagaModelTimer.Timer()("UpdateConditions", operator.IfElse((err != nil), "err", "ok").(string))

	ctx, cancel := ms.timeoutContext(ctx)
	defer cancel()

	txn.BeginSave()
	err = ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		dbTxn := &Txn{}
		txr := tx.Where("gtid=? AND (lessee = ? OR lease_expire_time < NOW())",
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

func (ms *modelStorage) GrantLease(ctx context.Context, txn *Txn, lease time.Duration) (err error) {
	defer sagaModelTimer.Timer()("GrantLease", operator.IfElse((err != nil), "err", "ok").(string))

	expire := fmt.Sprintf("NOW() + interval '%v millisecond'", lease.Milliseconds())
	ctx, cancel := ms.timeoutContext(ctx)
	defer cancel()
	err = ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txr := tx.Model(Txn{}).
			Where("gtid=? AND (lessee = ? OR lease_expire_time < NOW())", txn.Gtid, ms.lessee).
			Updates(map[string]interface{}{
				"lessee":            txn.Lessee,
				"lease_expire_time": gorm.Expr(expire),
				"updated_time":      gorm.Expr("NOW()")})
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

func (ms *modelStorage) GrantLeaseIncBranchCheckState(ctx context.Context, txn *Txn, branch *Branch,
	leaseDuration time.Duration, states []string) (err error) {
	defer sagaModelTimer.Timer()("GrantLeaseIncBranchCheckState", operator.IfElse((err != nil), "err", "ok").(string))

	ctx, cancel := ms.timeoutContext(ctx)
	defer cancel()

	// postgreSQL
	expire := fmt.Sprintf("NOW() + interval '%v millisecond'", leaseDuration.Milliseconds())
	err = ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txr := tx.Model(Txn{}).
			Where("gtid=? AND (lessee = ? OR lease_expire_time < NOW()) AND state IN ?", txn.Gtid, ms.lessee, states).
			Updates(map[string]interface{}{"lease_expire_time": gorm.Expr(expire),
				"lessee": txn.Lessee, "updated_time": gorm.Expr("NOW()")})
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

func (ms *modelStorage) Update(ctx context.Context, txn *Txn) (err error) {
	defer sagaModelTimer.Timer()("Update", operator.IfElse((err != nil), "err", "ok").(string))
	ctx, cancel := ms.timeoutContext(ctx)
	defer cancel()

	txn.BeginSave()

	err = ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		dbTxn := &Txn{}
		txr := tx.Model(Txn{}).
			Where("gtid=? AND (lessee = ? OR lease_expire_time < NOW())", txn.Gtid, ms.lessee).
			Find(dbTxn)
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

func (ms *modelStorage) UpdateBranch(ctx context.Context, branch *Branch) (err error) {
	defer sagaModelTimer.Timer()("UpdateBranch", operator.IfElse((err != nil), "err", "ok").(string))
	ctx, cancel := ms.timeoutContext(ctx)
	defer cancel()

	branch.BeginSave()
	err = ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		dbtxn := &Txn{}
		txr := tx.
			Where("gtid=? AND (lessee = ? OR lease_expire_time < NOW())", branch.Gtid, ms.lessee).
			Find(dbtxn)
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

func (ms *modelStorage) UpdateBranchConditions(ctx context.Context, branch *Branch, cb func(oldTxn *Txn, oldBranch *Branch) error) (err error) {
	defer sagaModelTimer.Timer()("UpdateBranchConditions", operator.IfElse((err != nil), "err", "ok").(string))
	ctx, cancel := ms.timeoutContext(ctx)
	defer cancel()

	branch.BeginSave()

	err = ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		dbTxn := &Txn{}
		txr := tx.Where("gtid=? AND (lessee = ? OR lease_expire_time < NOW())",
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

func (ms *modelStorage) RegisterBranches(ctx context.Context, branches []*Branch) (err error) {
	defer sagaModelTimer.Timer()("RegisterBranches", operator.IfElse((err != nil), "err", "ok").(string))
	if len(branches) == 0 {
		return nil
	}

	ctx, cancel := ms.timeoutContext(ctx)
	defer cancel()

	err = ms.Db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		dbtxn := &Txn{}
		txr := tx.Where("gtid=? AND expire_time > NOW() and state = ?",
			branches[0].Gtid, define.TxnStatePrepared).Find(dbtxn)
		if txr.Error != nil {
			return txr.Error
		}

		if txr.RowsAffected == 0 {
			return ErrInvalidLessee
		}
		if dbtxn.State != define.TxnStatePrepared {
			return ErrIsNotPrepared
		}

		txr = tx.Model(&Branch{}).Create(branches)
		return txr.Error
	})
	return err
}
