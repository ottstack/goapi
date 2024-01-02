package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/ottstack/goapi/pkg/ecode"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func New(addr string) *Client {
	return &Client{addr: addr}
}

type Client struct {
	addr string
}

func (c *Client) Call(ctx context.Context, path string, req interface{}, rsp interface{}) error {
	client := http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequest("POST", c.addr+path, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpRsp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpRsp.Body.Close()
	bs, err := io.ReadAll(httpRsp.Body)
	if err != nil {
		return fmt.Errorf("io.ReadAll error %v", err)
	}
	if httpRsp.StatusCode != 200 {
		e := &ecode.APIError{}
		if err := json.Unmarshal(bs, e); err != nil {
			return fmt.Errorf("json.Unmarshal error %v", err)
		}
		return e
	}
	if err := json.Unmarshal(bs, rsp); err != nil {
		return fmt.Errorf("json.Unmarshal error %v", err)
	}
	return nil
}
