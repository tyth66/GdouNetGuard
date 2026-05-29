//go:build windows

package campus

import "testing"

func TestWindowsCommandLineQuotesSpacesAndQuotes(t *testing.T) {
	got := windowsCommandLine([]string{"-background", "-ssid", "海大校园网", "-credential-file", `C:\Path With Space\cred"file.json`})
	want := `-background -ssid 海大校园网 -credential-file "C:\Path With Space\cred\"file.json"`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
