package campus

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type fakeProtector struct{}

func (fakeProtector) Protect(secret string) (string, error) {
	return base64.StdEncoding.EncodeToString([]byte("protected:" + secret)), nil
}

func (fakeProtector) Unprotect(secret string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(string(raw), "protected:"), nil
}

func TestCredentialStoreRoundTripDoesNotWritePlaintext(t *testing.T) {
	store := testCredentialStore(t)
	want := Credentials{Username: "alice", Password: "secret"}

	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), want.Username) || strings.Contains(string(body), want.Password) {
		t.Fatalf("credential store contains plaintext: %s", body)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestLoadCredentialsPrefersEnvironment(t *testing.T) {
	store := testCredentialStore(t)
	if err := store.Save(Credentials{Username: "saved-user", Password: "saved-pass"}); err != nil {
		t.Fatal(err)
	}
	cfg := testCredentialConfig()
	t.Setenv(cfg.UsernameEnv, "env-user")
	t.Setenv(cfg.PasswordEnv, "env-pass")

	got, source, err := LoadCredentials(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "env-user" || got.Password != "env-pass" {
		t.Fatalf("got %+v", got)
	}
	if source != "environment" {
		t.Fatalf("got source %q", source)
	}
}

func TestLoadCredentialsFallsBackToStore(t *testing.T) {
	store := testCredentialStore(t)
	want := Credentials{Username: "saved-user", Password: "saved-pass"}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}

	got, source, err := LoadCredentials(testCredentialConfig(), store)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if source != "saved credential store" {
		t.Fatalf("got source %q", source)
	}
}

func TestLoadCredentialsRejectsPartialEnvironment(t *testing.T) {
	store := testCredentialStore(t)
	if err := store.Save(Credentials{Username: "saved-user", Password: "saved-pass"}); err != nil {
		t.Fatal(err)
	}
	cfg := testCredentialConfig()
	t.Setenv(cfg.UsernameEnv, "env-user")

	_, _, err := LoadCredentials(cfg, store)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "credentials are incomplete") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCredentialStoreMissing(t *testing.T) {
	store := testCredentialStore(t)

	_, err := store.Load()
	if !errors.Is(err, ErrCredentialStoreMissing) {
		t.Fatalf("got %v, want %v", err, ErrCredentialStoreMissing)
	}
}

func TestWindowsDPAPIProtectorRoundTrip(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows DPAPI is only available on Windows")
	}
	protector := windowsDPAPIProtector{}

	protected, err := protector.Protect("secret with trailing space ")
	if err != nil {
		t.Fatal(err)
	}
	got, err := protector.Unprotect(protected)
	if err != nil {
		t.Fatal(err)
	}
	if got != "secret with trailing space " {
		t.Fatalf("got %q", got)
	}
}

func testCredentialStore(t *testing.T) CredentialStore {
	t.Helper()
	return CredentialStore{
		path:      filepath.Join(t.TempDir(), "credentials.json"),
		protector: fakeProtector{},
	}
}

func testCredentialConfig() Config {
	return Config{
		UsernameEnv: "CAMPUS_TEST_USERNAME",
		PasswordEnv: "CAMPUS_TEST_PASSWORD",
	}
}
