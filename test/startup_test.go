package campus_test

import (
	"reflect"
	"testing"

	"GdouNetGuard/src"
)

func TestGuardArgsPassesConfigAndNonDefaultSettings(t *testing.T) {
	cfg := campus.Config{
		ConfigFile:    `C:\Users\Alice\AppData\Local\GdouNetGuard\config.json`,
		CredentialFile: `C:\Users\Alice\AppData\Local\GdouNetGuard\credentials.json`,
	}

	got := campus.GuardArgs(cfg, true)
	want := []string{
		"-background",
		"-config", `C:\Users\Alice\AppData\Local\GdouNetGuard\config.json`,
		"-credential-file", `C:\Users\Alice\AppData\Local\GdouNetGuard\credentials.json`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestGuardArgsOmitsDefaultCredentialFile(t *testing.T) {
	cfg := campus.Config{
		ConfigFile: `C:\Users\Bob\AppData\Local\GdouNetGuard\config.json`,
	}

	got := campus.GuardArgs(cfg, false)
	want := []string{
		"-config", `C:\Users\Bob\AppData\Local\GdouNetGuard\config.json`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
