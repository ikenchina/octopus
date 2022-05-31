package http

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"strconv"
)

const (
	HTTP_HREADER_GTID         = "DTX_GTID"
	HTTP_HEADER_TC_DATACENTER = "DTX_TC_DATACENTER"
	HTTP_HEADER_TC_NODE       = "DTX_TC_NODE"
	HTTP_HEADER_TXN_TYPE      = "DTX_TXN_TYPE"
)

func Send(ctx context.Context, dc int, node int, txn string,
	gtid string, method, url, body string) (int, string, error) {
	// @todo delete method
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		return 0, "", err
	}
	req = req.WithContext(ctx)
	req.Header.Add(HTTP_HREADER_GTID, gtid)
	req.Header.Add(HTTP_HEADER_TC_DATACENTER, strconv.Itoa(dc))
	req.Header.Add(HTTP_HEADER_TC_NODE, strconv.Itoa(node))
	req.Header.Add(HTTP_HEADER_TXN_TYPE, txn)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	d, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, "", err
	}
	return resp.StatusCode, string(d), nil
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

	resp, err := http.DefaultClient.Do(req)
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

func Post(ctx context.Context, gtid string, url string, payload string) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(payload))
	if err != nil {
		return []byte(""), 0, err
	}

	req = req.WithContext(ctx)
	req.Header.Add(HTTP_HREADER_GTID, gtid)
	hresp, err := http.DefaultClient.Do(req)
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
	resp, err := http.DefaultClient.Do(req)
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
	resp, err := http.DefaultClient.Do(req)

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
