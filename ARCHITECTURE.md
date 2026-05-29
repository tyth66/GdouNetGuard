# ARCHITECTURE — GdouNetGuard

## Project Overview

GdouNetGuard is a Go 1.22 CLI tool for automatic campus network (SRUN portal) authentication. Zero external dependencies. Single binary. Designed for Guangdong Ocean University's SRUN/深澜 portal.

## Module Map

```
GdouNetGuard
├── main.go                  Entry point, PID lock, CLI dispatch, guard loop,
│                            interactive credential prompt, credential auto-save
└── src/
    ├── config.go            Config struct, ParseFlags(), Validate()
    ├── guard.go             Guard struct, EnsureConnected(), DoLogin(), Reauth(),
    │                        reconnectWLAN(), isProfileNotFound(), handleProbeFail()
    ├── srun.go              SRUN protocol: get_challenge, xEncode, srun_portal,
    │                        rad_user_info, detectClientIP, portalLogin/logout,
    │                        agreePortalProtocol, campusOnline, internetReachable
    ├── credentials.go       Credentials struct, DPAPI CredentialStore,
    │                        LoadCredentials(), SaveCredentialsFromEnv()
    ├── logrot.go            RotatingWriter: runtime log rotation on every Write()
    ├── startup.go           GuardArgs(): serialize config back to CLI args
    ├── startup_windows.go   Windows: StartBackground(), EnableStartup(), DisableStartup()
    ├── startup_other.go     Non-Windows: returns explicit errors for all three
    ├── version.go           Version constant
    ├── credentials_test.go  Windows DPAPI + credential round-trip tests
    └── startup_windows_test.go  Windows command-line escaping tests
test/
    ├── guard_test.go        SRUN login params, JSONP unwrap, portal HTML parse
    ├── main_test.go         GuardArgs, Validate, flag conflict tests
    └── startup_test.go      (placeholder)
```

## Dependency Graph

```
main.go
  └── src/
      ├── config.go          (standalone)
      ├── guard.go           ──▶ srun.go, credentials.go, logrot.go
      ├── srun.go            (standalone, net/http)
      ├── credentials.go     (standalone, os/exec → PowerShell DPAPI)
      ├── logrot.go          (standalone)
      ├── startup.go         (standalone)
      ├── startup_windows.go ──▶ startup.go
      └── startup_other.go   (standalone)
```

No external dependencies beyond Go standard library.

## Guard Loop State Machine

```
                    ┌─────────────────────────────┐
                    │    Check campusOnline()     │
                    └──────────────┬──────────────┘
                                   │
              ┌────────────────────┼────────────────────┐
              ▼                    ▼                    ▼
        [unreachable]         [online]             [offline]
              │                    │                    │
              ▼                    ▼                    ▼
     reconnectWLAN()     internetReachable()    hasCreds?
      │        │             │        │           │     │
   success   fail       reachable   fail       yes    no
      │        │             │        │           │     │
      ▼        ▼             ▼        ▼           ▼     ▼
    retry   log err       return   handleProbe  DoLogin  error
    ×2                          ok     Fail() →      │    "no creds"
      │                          │    Reauth()       │
      ▼                          ▼                   ▼
   campusOnline               return            login flow
   retry check                                   (6 steps)
```

### State Transitions

| State | Condition | Action | Next State |
|---|---|---|---|
| START | — | campusOnline() | → check result |
| ON_UNREACHABLE | SSID set | reconnectWLAN(), wait 5s | → RETRY |
| ON_UNREACHABLE | no SSID | return error | → END (this round) |
| RETRY | 1st retry fails | wait 3s, retry campusOnline() | → ON_UNREACHABLE or ONLINE or OFFLINE |
| RETRY | 2nd retry fails | log, return error | → END (this round) |
| ONLINE | probe OK | log, reset probeFailCount | → END (this round) |
| ONLINE | probe fail | handleProbeFail() | → PROBE_FAIL |
| PROBE_FAIL | count < max | log, skip | → END (this round) |
| PROBE_FAIL | count >= max, hasCreds | Reauth() | → END (this round) |
| PROBE_FAIL | count >= max, !hasCreds | log, skip | → END (this round) |
| OFFLINE | hasCreds | DoLogin() → 6-step flow | → END (this round) |
| OFFLINE | !hasCreds | return "no credentials" | → END (this round) |

## SRUN Login Flow (6 Steps)

```
1. agreePortalProtocol()
   POST /v1/srun_portal_agree_new   → get terms
   POST /v1/srun_portal_agree_bind  → agree terms

2. detectClientIP()
   GET /srun_portal_pc?ac_id=153&theme=pro
   Parse HTML for <input name="user_ip"> or IP regex

3. get_challenge()
   GET /cgi-bin/get_challenge?callback=...&username=...
   Returns: challenge token, client IP

4. Build login params:
   info = {SRBX1} + base64(xEncode(JSON(userinfo), challenge))
   password = {MD5} + HMAC-MD5(challenge, password) *not* standard
   chksum = SHA1(concat(all params))

5. portalLogin()
   POST /cgi-bin/srun_portal
   Callback JSONP unwrap → check res/error fields

6. Verify:
   campusOnline() → GET /cgi-bin/rad_user_info
   internetReachable() → GET probe URL
```

### Portal API Endpoints

| Endpoint | Method | Purpose |
|---|---|---|
| `/cgi-bin/rad_user_info?callback=campusAuth` | GET | Online status check |
| `/cgi-bin/get_challenge?callback=...&username=...` | GET | SRUN challenge token |
| `/cgi-bin/srun_portal` | POST | Submit login |
| `/cgi-bin/srun_portal` (logout) | POST | Submit logout |
| `/srun_portal_pc?ac_id=153&theme=pro` | GET | Login page (IP extraction) |
| `/v1/srun_portal_agree_new` | POST | Fetch agreement terms |
| `/v1/srun_portal_agree_bind` | POST | Agree to terms |

## Security Model

```
Credential Source Priority:
  1. Environment variables (CAMPUS_USERNAME, CAMPUS_PASSWORD)
  2. Interactive prompt (first run or -save-credentials without env vars)
  3. DPAPI-encrypted file (%AppData%\GdouNetGuard\credentials.json)

Credential lifecycle:
  Load → Login (HMAC-MD5 calc) → Clear() immediately via defer
  └── Password only lives in memory during DoLogin() execution

DPAPI encryption:
  Protect:   PowerShell ConvertTo-SecureString → ConvertFrom-SecureString
  Unprotect: PowerShell ConvertTo-SecureString → SecureStringToBSTR → PtrToStringBSTR
  └── BSTR zeroed via ZeroFreeBSTR after read

Auto-save:
  When credentials are loaded from environment, they are automatically
  persisted to the DPAPI store. When no credentials exist and the guard
  is running in foreground daemon mode (not -once/-reauth/-background),
  the user is interactively prompted and the response is saved immediately.
```

## Startup & Background Models

```
Direct foreground:  GdouNetGuard.exe
                     ├── If no credentials: interactive prompt → DPAPI save
                     └── main loop, stdout, Ctrl+C to stop

-save-credentials:  GdouNetGuard.exe -save-credentials
                     ├── Env vars set → save from env → exit
                     └── Env vars not set → interactive prompt → save → exit

Background:         GdouNetGuard.exe -background
                     └── PowerShell Start-Process -WindowStyle Hidden
                         └── GdouNetGuard.exe -log-file <path>
                             └── main loop, detached from console

Startup task:       GdouNetGuard.exe -enable-startup
                     └── schtasks /Create /SC ONLOGON
                         └── On user login → GdouNetGuard.exe -background -log-file <path>

Non-Windows:        All three return explicit errors
                    ("requires Windows; use systemd/launchd/nohup on this OS")
```

## Log Rotation

```
Startup:  RotateIfNeeded() — one-shot size check
Runtime:  RotatingWriter.Write() checks Size()+len(p) > maxSize before every write
           └── rotateBatch() renames: log → log.1, log.1 → log.2, ...
           └── maxBackups oldest file is deleted

Default:  1 MB max size, 3 backups kept
```

## Test Coverage

| Test | File | What It Covers |
|---|---|---|
| TestCredentialStoreRoundTripDoesNotWritePlaintext | credentials_test.go | DPAPI encrypt/decrypt, no plaintext on disk |
| TestLoadCredentialsPrefersEnvironment | credentials_test.go | Env vars override store |
| TestLoadCredentialsFallsBackToStore | credentials_test.go | Store fallback when no env |
| TestLoadCredentialsRejectsPartialEnvironment | credentials_test.go | Incomplete env = error |
| TestCredentialStoreMissing | credentials_test.go | Missing store = ErrCredentialStoreMissing |
| TestWindowsDPAPIProtectorRoundTrip | credentials_test.go | Real DPAPI round-trip |
| TestWindowsCommandLineQuotesSpacesAndQuotes | startup_windows_test.go | Argument escaping |
| TestDetectClientIPFromPortalHTML | guard_test.go | HTML IP parsing |
| TestDetectClientIPFallbackToChallenge | guard_test.go | IP from challenge response |
| TestBuildLoginParamsMatchesPortalAlgorithm | guard_test.go | SRUN HMAC-MD5 + xEncode |
| TestUnwrapJSONP | guard_test.go | JSONP callback unwrapping |
| TestGuardArgsIncludeOnlyNonDefaultNonSecretSettings | main_test.go | Config → CLI args |
| TestValidateRejectsConflictingStartupFlags | main_test.go | Flag conflict detection |
| TestValidateRejectsBackgroundOnce | main_test.go | -background + -once conflict |

## Key Design Decisions

- **Zero external deps**: Only Go standard library. No module downloads needed.
- **DPAPI via PowerShell**: Windows credential encryption uses PowerShell subprocess with UTF-16LE base64-encoded scripts. No CGo, no Windows API bindings.
- **Immutable config**: Config is parsed once at startup via `flag`. No runtime config changes.
- **PID file mutual exclusion**: Prevents duplicate guard instances. Uses `os.FindProcess` to check liveness.
- **Defer-based credential cleanup**: `defer creds.Clear()` in DoLogin/Reauth ensures password is cleared even on panic.
- **Interactive first-run**: When no credentials exist, foreground daemon prompts via stdin and auto-saves to DPAPI. Background and scheduled task modes skip interactive prompt since stdin is unavailable.
