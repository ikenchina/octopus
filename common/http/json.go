package http

import (
	"context"
	"encoding/json"
)

func PostJson(ctx context.Context, gtid string, url string, payload []byte, resp interface{}) (int, error) {
	body, code, err := Post(ctx, gtid, url, payload)
	if err != nil {
		return code, err
	}

	if resp != nil {
		err = json.Unmarshal(body, resp)
		if err != nil {
			return 0, err
		}
	}
	return code, nil
}

func GetJson(ctx context.Context, gtid string, url string, resp interface{}) (int, error) {
	body, code, err := Get(ctx, gtid, url)
	if err != nil {
		return code, err
	}
	return code, json.Unmarshal(body, resp)
}

func PutJson(ctx context.Context, gtid string, url string, payload string, resp interface{}) (int, error) {
	body, code, err := Put(ctx, gtid, url, payload)
	if err != nil {
		return code, err
	}
	return code, json.Unmarshal(body, resp)
}

func DeleteJson(ctx context.Context, gtid string, url string, resp interface{}) (int, error) {
	body, code, err := Delete(ctx, gtid, url)
	if err != nil {
		return code, err
	}
	return code, json.Unmarshal(body, resp)
}
