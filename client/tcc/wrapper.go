package tcc

import (
	"context"
	"fmt"
	"net/http"
	"time"

	shttp "github.com/ikenchina/octopus/common/http"
	"github.com/ikenchina/octopus/define"
)

type Transaction struct {
	cli  Client
	biz  string
	gtid string
	ctx  context.Context
}

func TccTransaction(ctx context.Context, tcServer string, expire time.Time, tryFunctions func(t *Transaction, gtid string) error) (*define.TccResponse, error) {
	t := &Transaction{
		ctx: ctx,
	}
	t.cli.TcServer = tcServer
	gtid, err := t.cli.NewGtid(ctx)
	if err != nil {
		return nil, err
	}
	t.gtid = gtid

	// begin transaction
	_, err = t.cli.Prepare(ctx, &define.TccRequest{
		Gtid:       gtid,
		Business:   t.biz,
		ExpireTime: expire,
	})
	if err != nil {
		return nil, err
	}

	cancel := func(err error) (*define.TccResponse, error) {
		resp, err2 := t.cli.Cancel(ctx, gtid)
		if err2 == nil {
			return resp, err
		}
		return nil, fmt.Errorf("%v, %v", err, err2)
	}
	confirm := func() (*define.TccResponse, error) {
		return t.cli.Confirm(ctx, gtid)
	}

	// transaction body
	err = tryFunctions(t, gtid)
	if err != nil {
		// rollback
		return cancel(err)
	}
	// commit
	return confirm()
}

func (t *Transaction) Try(branchID int, try string, confirm string, cancel string, payload string, opts ...func(o *Options)) ([]byte, error) {
	opt := Options{
		timeout: time.Second * 2,
		retry:   time.Second * 2,
	}
	for _, o := range opts {
		o(&opt)
	}

	_, err := t.cli.Register(t.ctx, t.gtid, &define.TccBranch{
		BranchId:      branchID,
		ActionConfirm: confirm,
		ActionCancel:  cancel,
		Timeout:       opt.timeout,
		Retry:         opt.retry,
	})
	if err != nil {
		return nil, err
	}

	body, code, err := shttp.Post(t.ctx, t.gtid, try, payload)
	if err != nil {
		return nil, err
	}
	if code >= http.StatusBadRequest {
		return body, fmt.Errorf("status code : %d", code)
	}
	return body, nil
}
