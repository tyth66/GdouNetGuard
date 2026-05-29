//go:build windows

package campus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

const (
	createNoWindow  = 0x08000000
	detachedProcess = 0x00000008
)

type startupTaskPayload struct {
	TaskName       string
	Executable     string
	ArgumentString string
}

// EnableStartup creates or updates the current user's Windows scheduled task.
func EnableStartup(cfg Config) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}
	payload := startupTaskPayload{
		TaskName:       cfg.StartupTaskName,
		Executable:     exe,
		ArgumentString: windowsCommandLine(GuardArgs(cfg, true)),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode startup task payload: %w", err)
	}
	return runPowerShellJSONScript(`
$ErrorActionPreference = 'Stop'
$payload = [Console]::In.ReadToEnd() | ConvertFrom-Json
$principal = New-ScheduledTaskPrincipal -UserId ([System.Security.Principal.WindowsIdentity]::GetCurrent().Name) -LogonType Interactive -RunLevel Limited
$action = New-ScheduledTaskAction -Execute $payload.Executable -Argument $payload.ArgumentString
$trigger = New-ScheduledTaskTrigger -AtLogOn
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -Hidden -ExecutionTimeLimit (New-TimeSpan -Seconds 0)
Register-ScheduledTask -TaskName $payload.TaskName -Action $action -Trigger $trigger -Settings $settings -Principal $principal -Description 'Campus network auto-auth guard' -Force | Out-Null
`, body)
}

// DisableStartup removes the current user's Windows scheduled task.
func DisableStartup(taskName string) error {
	payload := startupTaskPayload{TaskName: taskName}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode startup task payload: %w", err)
	}
	return runPowerShellJSONScript(`
$ErrorActionPreference = 'Stop'
$payload = [Console]::In.ReadToEnd() | ConvertFrom-Json
$task = Get-ScheduledTask -TaskName $payload.TaskName -ErrorAction SilentlyContinue
if ($null -ne $task) {
	Unregister-ScheduledTask -TaskName $payload.TaskName -Confirm:$false
}
`, body)
}

// StartBackground launches the guard as a detached background process.
func StartBackground(cfg Config) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}
	cmd := exec.Command(exe, GuardArgs(cfg, false)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow | detachedProcess,
	}
	nullFile, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open null device: %w", err)
	}
	defer nullFile.Close()
	cmd.Stdout = nullFile
	cmd.Stderr = nullFile
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start background guard: %w", err)
	}
	return cmd.Process.Release()
}

func runPowerShellJSONScript(script string, payload []byte) error {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodePowerShellCommand(script))
	cmd.Stdin = bytes.NewReader(payload)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			return fmt.Errorf("PowerShell startup operation failed: %w", err)
		}
		return fmt.Errorf("PowerShell startup operation failed: %w: %s", err, detail)
	}
	return nil
}

func windowsCommandLine(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, windowsCommandLineArg(arg))
	}
	return strings.Join(quoted, " ")
}

func windowsCommandLineArg(arg string) string {
	if arg != "" && !strings.ContainsAny(arg, " \t\"") {
		return arg
	}
	var b strings.Builder
	b.WriteByte('"')
	backslashes := 0
	for _, r := range arg {
		switch r {
		case '\\':
			backslashes++
		case '"':
			b.WriteString(strings.Repeat("\\", backslashes*2+1))
			b.WriteRune(r)
			backslashes = 0
		default:
			b.WriteString(strings.Repeat("\\", backslashes))
			b.WriteRune(r)
			backslashes = 0
		}
	}
	b.WriteString(strings.Repeat("\\", backslashes*2))
	b.WriteByte('"')
	return b.String()
}
