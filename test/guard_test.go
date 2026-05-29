package campus_test

import (
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"GdouNetGuard/src"
)

func TestDetectClientIPFromPortalHTML(t *testing.T) {
	portalHTML := `<script>
    var CONFIG = {
        page   : 'account',
        ip     : "10.0.0.99",
        nas    : "",
    };
</script>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "srun_portal_pc") {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(portalHTML))
			return
		}
		if strings.Contains(r.URL.Path, "get_challenge") {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`campusAuth({"challenge":"abc","client_ip":"10.0.0.77","error":"ok","res":"ok"})`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	credLoader := func() (*campus.Credentials, error) {
		return &campus.Credentials{Username: "testuser", Password: "testpass"}, nil
	}

	guard := campus.NewGuard(campus.Config{
		BaseURL:   srv.URL,
		ACID:      "153",
		Timeout:   5 * time.Second,
		Interval:  15 * time.Second,
		ProbeURL:  "",
	}, credLoader, "test", true)

	logger := log.New(os.Stdout, "", 0)
	err := guard.DoLogin(context.Background(), logger)
	if err == nil {
		t.Fatal("expected login to fail against test server, but got nil")
	}
	if !strings.Contains(err.Error(), "srun_portal") && !strings.Contains(err.Error(), "login rejected") {
		t.Fatalf("unexpected error after IP detection: %v", err)
	}
}

func TestDetectClientIPFallbackToChallenge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "srun_portal_pc") {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html>no ip field here</html>"))
			return
		}
		if strings.Contains(r.URL.Path, "get_challenge") {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`campusAuth({"challenge":"abc","client_ip":"10.0.0.77","error":"ok","res":"ok"})`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	credLoader := func() (*campus.Credentials, error) {
		return &campus.Credentials{Username: "testuser", Password: "testpass"}, nil
	}

	guard := campus.NewGuard(campus.Config{
		BaseURL:   srv.URL,
		ACID:      "153",
		Timeout:   5 * time.Second,
		Interval:  15 * time.Second,
		ProbeURL:  "",
	}, credLoader, "test", true)

	logger := log.New(os.Stdout, "", 0)
	err := guard.DoLogin(context.Background(), logger)
	if err == nil {
		t.Fatal("expected login to fail, but got nil")
	}
	if !strings.Contains(err.Error(), "srun_portal") && !strings.Contains(err.Error(), "login rejected") {
		t.Fatalf("unexpected error after fallback IP detection: %v", err)
	}
}
