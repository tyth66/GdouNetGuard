package campus

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"regexp"
)

// detectACID attempts to discover the portal ac_id by visiting the base URL
// and extracting the value from a redirect or the HTML response.
func (c *portalClient) detectACID(ctx context.Context) string {
	resp, err := c.doWithRetry(ctx, func() (*http.Request, error) {
		r, e := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL, nil)
		if e != nil {
			return nil, e
		}
		r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GdouNetGuard/1.3.0")
		return r, nil
	})
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	// 1. Check Location header for redirect with ac_id
	if loc := resp.Header.Get("Location"); loc != "" {
		if u, err := url.Parse(loc); err == nil {
			if acID := u.Query().Get("ac_id"); acID != "" {
				return acID
			}
		}
	}

	// 2. Check final URL query params
	if resp.Request != nil && resp.Request.URL != nil {
		if acID := resp.Request.URL.Query().Get("ac_id"); acID != "" {
			return acID
		}
	}

	// 3. Parse HTML body for ac_id patterns
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return ""
	}

	patterns := []string{
		`name=["']ac_id["']\s+value=["']([^"']+)["']`,
		`ac_id\s*[=:]\s*["']([^"']+)["']`,
	}
	for _, pat := range patterns {
		re := regexp.MustCompile(pat)
		if m := re.FindSubmatch(body); len(m) >= 2 {
			return string(m[1])
		}
	}

	return ""
}

// AutoDetectACID attempts to auto-detect the portal ac_id when the current
// value equals the default (i.e. the user has not explicitly set it).
func AutoDetectACID(cfg *Config) {
	if cfg.ACID == "" {
		return
	}
	client := newPortalClient(cfg.BaseURL, cfg.Timeout, cfg.RetryMax, cfg.RetryBaseDelay)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()
	if detected := client.detectACID(ctx); detected != "" {
		cfg.ACID = detected
	}
}
