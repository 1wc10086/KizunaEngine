package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func httpTimeout(spec Spec) time.Duration {
	if spec.Timeout <= 0 {
		return 120 * time.Second
	}
	return spec.Timeout
}

func newHTTPClient(spec Spec) *http.Client {
	return &http.Client{Timeout: httpTimeout(spec)}
}

func effectiveMaxTokens(n int) int {
	if n <= 0 {
		return 4096
	}
	return n
}

func backoffDelay(retry int) time.Duration {
	d := time.Second << retry
	if d <= 0 || d > 30*time.Second {
		return 30 * time.Second
	}
	return d
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func scrubText(s, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, "[redacted]")
}

func scrubErr(err error, key string) error {
	if err == nil || key == "" {
		return err
	}
	if !strings.Contains(err.Error(), key) {
		return err
	}
	return errors.New(scrubText(err.Error(), key))
}

func snippet(body []byte) string {
	if len(body) > 300 {
		return string(body[:300])
	}
	return string(body)
}

func statusError(status int, body []byte, key string) error {
	return fmt.Errorf("provider request failed with status %d: %s", status, scrubText(snippet(body), key))
}

func doPost(ctx context.Context, client *http.Client, url string, header map[string]string, data []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return 0, nil, err
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, body, nil
}

func postJSON(ctx context.Context, client *http.Client, url string, header map[string]string, payload any, maxRetries int, key string) (int, []byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}
	if maxRetries < 0 {
		maxRetries = 0
	}
	for attempt := 0; ; attempt++ {
		status, body, transportErr := doPost(ctx, client, url, header, data)
		if transportErr != nil {
			if ctx.Err() != nil {
				return 0, nil, ctx.Err()
			}
			if attempt >= maxRetries {
				return 0, nil, scrubErr(transportErr, key)
			}
			if err := sleepCtx(ctx, backoffDelay(attempt)); err != nil {
				return 0, nil, err
			}
			continue
		}
		if status == http.StatusTooManyRequests || status >= 500 {
			err := statusError(status, body, key)
			if attempt >= maxRetries {
				return 0, nil, err
			}
			if serr := sleepCtx(ctx, backoffDelay(attempt)); serr != nil {
				return 0, nil, serr
			}
			continue
		}
		return status, body, nil
	}
}
