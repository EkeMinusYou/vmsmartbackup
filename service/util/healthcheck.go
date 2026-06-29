package util

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

const (
	healthCheckInterval = 5 * time.Second
	healthCheckTimeout  = 5 * time.Minute
)

// WaitUntilReady は指定したURLが200を返すまで定期的にポーリングして待機する。
func WaitUntilReady(ctx context.Context, url string) error {
	ctx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()

	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	client := &http.Client{Timeout: healthCheckInterval}

	for {
		if ok := checkHealth(ctx, client, url); ok {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for %s to become ready: %w", url, ctx.Err())
		case <-ticker.C:
		}
	}
}

func checkHealth(ctx context.Context, client *http.Client, url string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}
