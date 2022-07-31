package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	shttp "github.com/ikenchina/octopus/common/http"
	"github.com/ikenchina/octopus/define"
	pb "github.com/ikenchina/octopus/define/proto/saga/pb"
)

// GrpcClient is grpc client of SAGA AP
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

// HttpClient is http client of AP
type HttpClient struct {
	TcServer string
}

// Get get states of saga transaction
func (cli *HttpClient) Get(ctx context.Context, gtid string) (*define.SagaResponse, error) {
	url := cli.TcServer + "/dtx/saga/" + gtid
	resp := &define.SagaResponse{}
	code, err := shttp.GetJson(ctx, gtid, url, resp)
	if err != nil {
		return nil, err
	}
	if code >= http.StatusBadRequest {
		return resp, fmt.Errorf("status code : %d", code)
	}
	return resp, nil
}

// NewGtid create a unique identifier for transaction
func (cli *HttpClient) NewGtid(ctx context.Context) (string, error) {
	url := cli.TcServer + "/dtx/saga/gtid"
	mm := make(map[string]string)
	code, err := shttp.GetJson(ctx, "", url, &mm)
	if err != nil {
		return "", err
	}
	if code >= http.StatusBadRequest {
		return "", fmt.Errorf("status code : %d", code)
	}

	gtid, ok := mm["gtid"]
	if !ok {
		return "", fmt.Errorf("empty gtid")
	}
	return gtid, nil
}

// Commit commit saga transaction
func (cli *HttpClient) Commit(ctx context.Context, req *define.SagaRequest) (*define.SagaResponse, error) {
	url := cli.TcServer + "/dtx/saga"
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	resp := &define.SagaResponse{}
	code, err := shttp.PostJson(ctx, req.Gtid, url, (data), resp)
	if err != nil {
		return nil, err
	}
	if code >= http.StatusBadRequest {
		return resp, fmt.Errorf("status code : %d", code)
	}
	return resp, nil
}
