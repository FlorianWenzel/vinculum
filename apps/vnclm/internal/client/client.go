package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTP is a thin JSON HTTP client pointed at a forwarded port.
type HTTP struct {
	base string
	hc   *http.Client
}

func New(base string) *HTTP {
	return &HTTP{base: base, hc: &http.Client{Timeout: 30 * time.Second}}
}

func (c *HTTP) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.hc.Do(req)
}

func (c *HTTP) GetJSON(ctx context.Context, path string, out any) error {
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return errorFromResp(resp)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *HTTP) PostJSON(ctx context.Context, path string, body any, out any) error {
	resp, err := c.do(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return errorFromResp(resp)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *HTTP) Delete(ctx context.Context, path string) error {
	resp, err := c.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return errorFromResp(resp)
	}
	return nil
}

// Stream issues GET and streams the response body to w until EOF.
func (c *HTTP) Stream(ctx context.Context, path string, w io.Writer) error {
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return errorFromResp(resp)
	}
	_, err = io.Copy(w, resp.Body)
	return err
}

func errorFromResp(resp *http.Response) error {
	b, _ := io.ReadAll(resp.Body)
	msg := string(bytes.TrimSpace(b))
	if msg == "" {
		msg = resp.Status
	}
	return fmt.Errorf("%d: %s", resp.StatusCode, msg)
}
