package campus

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// requestFactory creates a new HTTP request. It is called once per retry
// attempt so that request bodies (io.Reader) can be re-created.
type requestFactory func() (*http.Request, error)

// doWithRetry executes an HTTP request through the portal client's
// transport, retrying on transient network errors up to retryMax times with
// exponential backoff starting at retryBaseDelay.
func (c *portalClient) doWithRetry(ctx context.Context, factory requestFactory) (*http.Response, error) {
	if c.retryMax <= 0 {
		req, err := factory()
		if err != nil {
			return nil, err
		}
		return c.http.Do(req)
	}

	var lastErr error
	delay := c.retryBaseDelay
	for attempt := 0; attempt <= c.retryMax; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
				delay *= 2
			}
		}
		req, err := factory()
		if err != nil {
			return nil, err
		}
		resp, doErr := c.http.Do(req)
		if doErr == nil {
			return resp, nil
		}
		if !isRetryable(doErr) {
			return nil, doErr
		}
		lastErr = doErr
	}
	return nil, fmt.Errorf("request failed after %d retries: %w", c.retryMax+1, lastErr)
}

// isRetryable reports whether an HTTP client error is transient and worth
// retrying.
func isRetryable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}
	return false
}
