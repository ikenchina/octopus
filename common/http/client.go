package http

import (
	"bytes"
	"context"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"time"
)

const (
	HTTP_HREADER_GTID         = "DTX_GTID"
	HTTP_HEADER_TC_DATACENTER = "DTX_TC_DATACENTER"
	HTTP_HEADER_TC_NODE       = "DTX_TC_NODE"
	HTTP_HEADER_TXN_TYPE      = "DTX_TXN_TYPE"
)

var HTTPTransport = &http.Transport{
	DialContext: (&net.Dialer{
		Timeout:   3 * time.Second,  // 连接超时时间
		KeepAlive: 60 * time.Second, // 保持长连接的时间
	}).DialContext, // 设置连接的参数
	MaxIdleConns:          1000,             // 最大空闲连接
	IdleConnTimeout:       60 * time.Second, // 空闲连接的超时时间
	ExpectContinueTimeout: 30 * time.Second, // 等待服务第一个响应的超时时间
	MaxIdleConnsPerHost:   100,              // 每个host保持的空闲连接数
}

var client2 = http.Client{Transport: HTTPTransport}

func Send(ctx context.Context, dc int, node int, txn string,
	gtid string, method, url string, body []byte) (int, []byte, error) {
	// @todo delete method
	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	if err != nil {
		return 0, nil, err
	}
	req = req.WithContext(ctx)
	req.Header.Add(HTTP_HREADER_GTID, gtid)
	req.Header.Add(HTTP_HEADER_TC_DATACENTER, strconv.Itoa(dc))
	req.Header.Add(HTTP_HEADER_TC_NODE, strconv.Itoa(node))
	req.Header.Add(HTTP_HEADER_TXN_TYPE, txn)

	resp, err := client2.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	d, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, (d), nil
}

func Get(ctx context.Context, gtid string, url string) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return []byte(""), 0, err
	}

	req = req.WithContext(ctx)
	if len(gtid) != 0 {
		req.Header.Add(HTTP_HREADER_GTID, gtid)
	}

	resp, err := client2.Do(req)
	if err != nil {
		return []byte(""), 0, err
	}
	defer resp.Body.Close()

	d, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []byte(""), 0, err
	}
	return d, resp.StatusCode, nil
}

func Post(ctx context.Context, gtid string, url string, payload []byte) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(payload))
	if err != nil {
		return []byte(""), 0, err
	}

	req = req.WithContext(ctx)
	req.Header.Add(HTTP_HREADER_GTID, gtid)
	hresp, err := client2.Do(req)
	if err != nil {
		return []byte(""), 0, err
	}
	defer hresp.Body.Close()
	d, err := ioutil.ReadAll(hresp.Body)
	if err != nil {
		return []byte(""), 0, err
	}
	return d, hresp.StatusCode, nil
}

func Put(ctx context.Context, gtid string, url string, payload string) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBufferString(payload))
	if err != nil {
		return []byte(""), 0, err
	}

	req = req.WithContext(ctx)
	req.Header.Add(HTTP_HREADER_GTID, gtid)
	resp, err := client2.Do(req)
	if err != nil {
		return []byte(""), 0, err
	}
	defer resp.Body.Close()
	d, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []byte(""), 0, err
	}
	return d, resp.StatusCode, nil
}

func Delete(ctx context.Context, gtid string, url string) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return []byte(""), 0, err
	}

	req = req.WithContext(ctx)
	req.Header.Add(HTTP_HREADER_GTID, gtid)
	resp, err := client2.Do(req)

	if err != nil {
		return []byte(""), 0, err
	}
	defer resp.Body.Close()
	d, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []byte(""), 0, err
	}
	return d, resp.StatusCode, nil
}
