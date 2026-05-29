package campus_test

import (
	"testing"

	"GdouNetGuard/src"
)

func TestBuildLoginParamsMatchesPortalAlgorithm(t *testing.T) {
	params, err := campus.BuildLoginParams("alice", "secret", "10.0.0.2", "153", "0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}

	assertParam(t, params.Get("password"), "{MD5}fea03f8ea7355c811537d43482714fca")
	assertParam(t, params.Get("info"), "{SRBX1}gW17w5Fin7uK6pdSCJ3uMfHmw+5HOtIL+pzWSXpMYkvrFx3AQ5lfgJVtxCp2cU7+v0u+3Tx3NziR8W4Zrv+yADfJj6pNW8tH07vOL/Qv134ukHRy/GmXdMdS6bmy48Wc")
	assertParam(t, params.Get("chksum"), "d5ba58da9f9b6fdb7d24dec9ccbe3a2ae9d86c7c")
	assertParam(t, params.Get("action"), "login")
	assertParam(t, params.Get("ac_id"), "153")
	assertParam(t, params.Get("n"), "200")
	assertParam(t, params.Get("type"), "1")
}

func TestUnwrapJSONP(t *testing.T) {
	body, err := campus.UnwrapJSONP([]byte(`campusAuth({"error":"ok","res":"ok"})`))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"error":"ok","res":"ok"}` {
		t.Fatalf("unexpected body: %s", body)
	}
}

func assertParam(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
