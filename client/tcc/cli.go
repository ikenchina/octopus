package tcc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	shttp "github.com/ikenchina/octopus/common/http"
	"github.com/ikenchina/octopus/define"
)

type Options struct {
	timeout time.Duration
	retry   time.Duration
}

func WithTimeout(timeout time.Duration) func(o *Options) {
	return func(o *Options) {
		o.timeout = timeout
	}
}

func WithRetry(retry time.Duration) func(o *Options) {
	return func(o *Options) {
		o.retry = retry
	}
}

type Client struct {
	TcServer string
}

func (cli *Client) NewGtid(ctx context.Context) (string, error) {
	url := cli.TcServer + "/dtx/tcc/gtid"
	mm := make(map[string]string)
	_, err := shttp.GetJson(ctx, "", url, &mm)
	if err != nil {
		return "", err
	}
	gtid, ok := mm["gtid"]
	if !ok {
		return "", fmt.Errorf("empty gtid")
	}
	return gtid, nil
}

func (cli *Client) Get(ctx context.Context, gtid string) (*define.TccResponse, error) {
	url := cli.TcServer + "/dtx/tcc/" + gtid
	resp := &define.TccResponse{}
	_, err := shttp.GetJson(ctx, gtid, url, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (cli *Client) Prepare(ctx context.Context, req *define.TccRequest) (*define.TccResponse, error) {
	url := cli.TcServer + "/dtx/tcc"
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	resp := &define.TccResponse{}
	_, err = shttp.PostJson(ctx, req.Gtid, url, string(data), resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (cli *Client) Register(ctx context.Context, gtid string, branch *define.TccBranch) (*define.TccResponse, error) {
	url := cli.TcServer + "/dtx/tcc/" + gtid

	request := &define.TccRequest{
		Gtid:     gtid,
		Branches: []define.TccBranch{*branch},
	}

	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	resp := &define.TccResponse{}
	code, err := shttp.PostJson(ctx, gtid, url, string(data), resp)
	if err != nil {
		return nil, err
	}
	if code >= http.StatusBadRequest {
		return nil, fmt.Errorf("status code : %d", code)
	}
	return resp, nil
}

func (cli *Client) Confirm(ctx context.Context, gtid string) (*define.TccResponse, error) {
	url := cli.TcServer + "/dtx/tcc/" + gtid
	resp := &define.TccResponse{}
	code, err := shttp.PutJson(ctx, gtid, url, "", resp)
	if err != nil {
		return nil, err
	}
	if code >= http.StatusBadRequest {
		return nil, fmt.Errorf("status code : %d", code)
	}
	return resp, nil
}

func (cli *Client) Cancel(ctx context.Context, gtid string) (*define.TccResponse, error) {
	url := cli.TcServer + "/dtx/tcc/" + gtid
	resp := &define.TccResponse{}
	code, err := shttp.DeleteJson(ctx, gtid, url, resp)
	if err != nil {
		return nil, err
	}
	if code >= http.StatusBadRequest {
		return nil, fmt.Errorf("status code : %d", code)
	}
	return resp, nil
}
