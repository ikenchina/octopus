package saga

import (
	"context"
	"encoding/json"
	"fmt"

	shttp "github.com/ikenchina/octopus/common/http"
	"github.com/ikenchina/octopus/define"
)

type Client struct {
	TcServer string
}

func (cli *Client) Get(ctx context.Context, gtid string) (*define.SagaResponse, error) {
	url := cli.TcServer + "/dtx/saga/" + gtid
	resp := &define.SagaResponse{}
	_, err := shttp.GetJson(ctx, gtid, url, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (cli *Client) NewGtid(ctx context.Context) (string, error) {
	url := cli.TcServer + "/dtx/saga/gtid"
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

func (cli *Client) Commit(ctx context.Context, req *define.SagaRequest) (*define.SagaResponse, error) {
	url := cli.TcServer + "/dtx/saga"
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	resp := &define.SagaResponse{}
	_, err = shttp.PostJson(ctx, req.Gtid, url, string(data), resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
