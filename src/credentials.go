package campus

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"
)

const (
	credentialStoreVersion = 1
	credentialProtection   = "windows-dpapi-current-user"
)

// ErrCredentialStoreMissing is returned when no saved credentials exist.
var ErrCredentialStoreMissing = errors.New("saved credentials not found")

// Credentials holds a campus network username and password.
type Credentials struct {
	Username string
	Password string
}

// Clear removes the plaintext password from the credential struct. The caller
// should invoke this as soon as the password is no longer needed, so that
// the plaintext is eligible for GC collection rather than persisting in memory
// for the lifetime of the guard process.
func (c *Credentials) Clear() {
	c.Password = ""
}

type secretProtector interface {
	Protect(string) (string, error)
	Unprotect(string) (string, error)
}

// CredentialStore provides DPAPI-encrypted credential persistence.
type CredentialStore struct {
	path      string
	protector secretProtector
}

type savedCredentials struct {
	Version    int    `json:"version"`
	Protection string `json:"protection"`
	Username   string `json:"username"`
	Password   string `json:"password"`
}

type windowsDPAPIProtector struct{}

// NewCredentialStore creates a credential store at the given path.
// If path is empty, the default Windows user config directory is used.
func NewCredentialStore(path string) (CredentialStore, error) {
	if path == "" {
		defaultPath, err := defaultCredentialStorePath()
		if err != nil {
			return CredentialStore{}, err
		}
		path = defaultPath
	}
	return CredentialStore{
		path:      path,
		protector: windowsDPAPIProtector{},
	}, nil
}

func defaultCredentialStorePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config directory: %w", err)
	}
	return filepath.Join(configDir, "GdouNetGuard", "credentials.json"), nil
}

// Path returns the file path of the credential store.
func (s CredentialStore) Path() string {
	return s.path
}

// Save encrypts and persists credentials.
func (s CredentialStore) Save(creds Credentials) error {
	if creds.Username == "" || creds.Password == "" {
		return errors.New("username and password are required")
	}
	username, err := s.protector.Protect(creds.Username)
	if err != nil {
		return fmt.Errorf("protect username: %w", err)
	}
	password, err := s.protector.Protect(creds.Password)
	if err != nil {
		return fmt.Errorf("protect password: %w", err)
	}
	payload := savedCredentials{
		Version:    credentialStoreVersion,
		Protection: credentialProtection,
		Username:   username,
		Password:   password,
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode credentials: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return fmt.Errorf("create credential directory: %w", err)
	}
	if err := os.WriteFile(s.path, append(body, '\n'), 0600); err != nil {
		return fmt.Errorf("write credential store: %w", err)
	}
	return nil
}

// Load decrypts and returns saved credentials.
func (s CredentialStore) Load() (Credentials, error) {
	body, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Credentials{}, ErrCredentialStoreMissing
		}
		return Credentials{}, fmt.Errorf("read credential store: %w", err)
	}
	var payload savedCredentials
	if err := json.Unmarshal(body, &payload); err != nil {
		return Credentials{}, fmt.Errorf("decode credential store: %w", err)
	}
	if payload.Version != credentialStoreVersion {
		return Credentials{}, fmt.Errorf("unsupported credential store version %d", payload.Version)
	}
	if payload.Protection != credentialProtection {
		return Credentials{}, fmt.Errorf("unsupported credential protection %q", payload.Protection)
	}
	username, err := s.protector.Unprotect(payload.Username)
	if err != nil {
		return Credentials{}, fmt.Errorf("unprotect username: %w", err)
	}
	password, err := s.protector.Unprotect(payload.Password)
	if err != nil {
		return Credentials{}, fmt.Errorf("unprotect password: %w", err)
	}
	if username == "" || password == "" {
		return Credentials{}, errors.New("saved credentials are incomplete")
	}
	return Credentials{Username: username, Password: password}, nil
}

// Delete removes the credential store file.
func (s CredentialStore) Delete() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete credential store: %w", err)
	}
	return nil
}

func credentialsFromEnv(cfg Config) (Credentials, bool, bool) {
	username := os.Getenv(cfg.UsernameEnv)
	password := os.Getenv(cfg.PasswordEnv)
	return Credentials{Username: username, Password: password}, username != "", password != ""
}

// SaveCredentialsFromEnv reads credentials from environment variables and
// saves them with Windows DPAPI encryption.
func SaveCredentialsFromEnv(cfg Config, store CredentialStore) error {
	creds, hasUsername, hasPassword := credentialsFromEnv(cfg)
	if !hasUsername || !hasPassword {
		return fmt.Errorf("set both %s and %s before using -save-credentials", cfg.UsernameEnv, cfg.PasswordEnv)
	}
	return store.Save(creds)
}

// LoadCredentials loads credentials from environment variables if set,
// falling back to the credential store.
func LoadCredentials(cfg Config, store CredentialStore) (Credentials, string, error) {
	creds, hasUsername, hasPassword := credentialsFromEnv(cfg)
	if hasUsername && hasPassword {
		return creds, "environment", nil
	}
	if hasUsername || hasPassword {
		return Credentials{}, "", fmt.Errorf("credentials are incomplete; set both %s and %s or unset both to use saved credentials", cfg.UsernameEnv, cfg.PasswordEnv)
	}

	creds, err := store.Load()
	if err != nil {
		if errors.Is(err, ErrCredentialStoreMissing) {
			return Credentials{}, "", fmt.Errorf("credentials are missing; set %s and %s, or run -save-credentials after setting them", cfg.UsernameEnv, cfg.PasswordEnv)
		}
		return Credentials{}, "", err
	}
	return creds, "saved credential store", nil
}

func (windowsDPAPIProtector) Protect(secret string) (string, error) {
	if runtime.GOOS != "windows" {
		return "", errors.New("persistent credentials require Windows DPAPI; use environment variables on this OS")
	}
	return runPowerShellSecretScript(`
$ErrorActionPreference = 'Stop'
$secret = [Console]::In.ReadToEnd()
$secure = ConvertTo-SecureString -String $secret -AsPlainText -Force
$encrypted = ConvertFrom-SecureString -SecureString $secure
[Console]::Out.Write($encrypted)
`, secret)
}

func (windowsDPAPIProtector) Unprotect(secret string) (string, error) {
	if runtime.GOOS != "windows" {
		return "", errors.New("persistent credentials require Windows DPAPI; use environment variables on this OS")
	}
	return runPowerShellSecretScript(`
$ErrorActionPreference = 'Stop'
$encrypted = [Console]::In.ReadToEnd().Trim()
$secure = ConvertTo-SecureString -String $encrypted
$bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
try {
	[Console]::Out.Write([Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr))
} finally {
	if ($bstr -ne [IntPtr]::Zero) {
		[Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
	}
}
`, secret)
}

func runPowerShellSecretScript(script, input string) (string, error) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodePowerShellCommand(script))
	cmd.Stdin = strings.NewReader(input)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			return "", fmt.Errorf("PowerShell credential operation failed: %w", err)
		}
		return "", fmt.Errorf("PowerShell credential operation failed: %w: %s", err, detail)
	}
	return strings.TrimRight(stdout.String(), "\r\n"), nil
}

func encodePowerShellCommand(script string) string {
	encoded := utf16.Encode([]rune(script))
	raw := make([]byte, len(encoded)*2)
	for i, value := range encoded {
		binary.LittleEndian.PutUint16(raw[i*2:], value)
	}
	return base64.StdEncoding.EncodeToString(raw)
}
