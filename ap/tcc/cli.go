package tcc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	shttp "github.com/ikenchina/octopus/common/http"
	"github.com/ikenchina/octopus/define"
	"github.com/ikenchina/octopus/define/proto/tcc/pb"
)

// GrpcClient is grpc client of TCC AP
type GrpcClient struct {
	pb.TcClient
}

// NewGrpcClient create a grpc client
//   target is address of grpc server
func NewGrpcClient(target string) (*GrpcClient, error) {
	conn, err := grpc.Dial(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	cli := &GrpcClient{}
	cli.TcClient = pb.NewTcClient(conn)
	return cli, err
}

type options struct {
	timeout time.Duration
	retry   time.Duration
}

// WithTimeout set timeout duration
func WithTimeout(timeout time.Duration) func(o *options) {
	return func(o *options) {
		o.timeout = timeout
	}
}

// WithRetry set retry duration
func WithRetry(retry time.Duration) func(o *options) {
	return func(o *options) {
		o.retry = retry
	}
}

// HttpClient is http client of TCC AP
type HttpClient struct {
	TcServer string
}

// NewGtid create a unique identifier for transaction
func (cli *HttpClient) NewGtid(ctx context.Context) (string, error) {
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

// Get get states of saga transaction
func (cli *HttpClient) Get(ctx context.Context, gtid string) (*define.TccResponse, error) {
	url := cli.TcServer + "/dtx/tcc/" + gtid
	resp := &define.TccResponse{}
	_, err := shttp.GetJson(ctx, gtid, url, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Prepare prepare transaction
func (cli *HttpClient) Prepare(ctx context.Context, req *define.TccRequest) (*define.TccResponse, error) {
	url := cli.TcServer + "/dtx/tcc"
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	resp := &define.TccResponse{}
	_, err = shttp.PostJson(ctx, req.Gtid, url, (data), resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Register register a branch transaction for TCC transaction
func (cli *HttpClient) Register(ctx context.Context, gtid string, branch *define.TccBranch) (*define.TccResponse, error) {
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
	code, err := shttp.PostJson(ctx, gtid, url, (data), resp)
	if err != nil {
		return nil, err
	}
	if code >= http.StatusBadRequest {
		return nil, fmt.Errorf("status code : %d", code)
	}
	return resp, nil
}

// Confirm commit TCC transaction
func (cli *HttpClient) Confirm(ctx context.Context, gtid string) (*define.TccResponse, error) {
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

// Cancel rollback TCC transaction
func (cli *HttpClient) Cancel(ctx context.Context, gtid string) (*define.TccResponse, error) {
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
