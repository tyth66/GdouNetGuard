# GdouNetGuard

Go 版校园网自动认证守护脚本，针对海大校园网 SRUN / 深澜门户实现。版本 1.3.0。

## 功能

- **自动认证**：基于 SRUN challenge 登录流程，自动完成 portal 认证（含协议条款自动同意）
- **持续守护**：按固定间隔检查校园网在线状态，离线时自动登录
- **WLAN 自动重连**：网络不可达时自动调用 `netsh wlan connect` 重连 WiFi（Windows）
- **互联网连通性探测**：不仅检查 portal 在线状态，还探测外网是否可达，连续探测失败时强制重新认证
- **凭据安全存储**：通过 Windows DPAPI 加密保存账号密码，不会以明文写入文件
- **多种运行模式**：一次性检查（`-once`）、持续守护、后台进程（`-background`）、开机自启（`-enable-startup`）
- **日志轮转**：运行时每次写入前检查文件大小，超出阈值即时轮转，长期运行不会占满磁盘
- **PID 互斥锁**：通过 PID 文件防止重复实例运行

> **非 Windows 平台**：守护循环（`-once` / 持续运行）仍可用，但后台进程（`-background`）、
> 开机自启（`-enable-startup`）和 WLAN 自动重连仅支持 Windows。在这些平台上使用对应
> flags 会返回明确的错误提示。

## 快速开始

### 1. 设置账号密码环境变量

```powershell
setx CAMPUS_USERNAME "你的校园网账号"
setx CAMPUS_PASSWORD "你的校园网密码"
```

重新打开 PowerShell 后生效。

### 2. 一次性检查并登录

```powershell
.\GdouNetGuard.exe -once
```

> 首次运行时，如果设置了环境变量，凭据会自动保存到 DPAPI 加密存储。
> 后续即使清除环境变量，也能自动读取已保存的凭据。

### 3. 持续守护（推荐）

```powershell
.\GdouNetGuard.exe
```

每隔 15 秒检查校园网状态，离线时自动登录。按 `Ctrl+C` 优雅退出。

## 凭据管理

工具支持三种凭据来源，优先级从高到低：

| 来源 | 说明 |
|---|---|
| 环境变量 | `CAMPUS_USERNAME` + `CAMPUS_PASSWORD`，适合临时使用 |
| DPAPI 加密存储 | 通过 `-save-credentials` 保存，适合长期使用 |

### 保存凭据（推荐）

先用环境变量设置账号密码，再加密保存：

```powershell
$env:CAMPUS_USERNAME = "你的校园网账号"
$env:CAMPUS_PASSWORD = "你的校园网密码"
.\GdouNetGuard.exe -save-credentials
```

保存成功后即可清除环境变量中的密码：

> **凭据会自动保存**：只要运行时检测到环境变量中有账号密码，就会自动保存到 DPAPI 加密存储。
> 正常使用（`-once` 或持续守护模式）即可触发自动保存，无需手动执行 `-save-credentials`。
> 手动 `-save-credentials` 仅在只想保存凭据而不执行认证时使用。

```powershell
Remove-Item Env:\CAMPUS_USERNAME
Remove-Item Env:\CAMPUS_PASSWORD
```

凭据保存位置：`%AppData%\GdouNetGuard\credentials.json`，使用当前用户 DPAPI 加密，同台机器上其他 Windows 用户无法直接解密。

### 验证

保存凭据后直接运行一次性检查确认凭据可用：

```powershell
.\GdouNetGuard.exe -once
```

### 删除凭据

```powershell
.\GdouNetGuard.exe -forget-credentials
```

### 安全说明

- 密码仅在每次登录时按需加载，登录完成后立即从内存中清除
- SRUN challenge 登录每次都需要原始密码参与 HMAC-MD5 计算，不能只存储不可逆哈希
- 因此使用 Windows DPAPI 加密存储，而非明文文件

## 运行模式

### 一次性检查（`-once`）

运行一轮检查后退出。适合手动触发或脚本调用：

```powershell
.\GdouNetGuard.exe -once
```

如果已在线则无操作；如果离线则自动登录。

### 持续守护（默认）

前台持续运行，按 `-interval` 间隔检查：

```powershell
.\GdouNetGuard.exe
.\GdouNetGuard.exe -interval 30s
```

按 `Ctrl+C` 退出，退出时自动清理 PID 文件。

### 后台守护（`-background`）

启动隐藏的后台进程，命令立即返回：

```powershell
.\GdouNetGuard.exe -background -log-file logs\guard.log
```

仅在 Windows 上可用。后台进程不会输出到控制台，必须配合 `-log-file` 使用才能查看日志。

### 开机自启（`-enable-startup`）

创建当前用户的 Windows 计划任务，登录时自动以后台模式启动守护：

```powershell
.\GdouNetGuard.exe -enable-startup
```

关闭开机自启：

```powershell
.\GdouNetGuard.exe -disable-startup
```

> 开机自启不会把账号密码写入计划任务。首次运行守护模式时凭据会自动保存，
> 确认凭据已保存后再启用自启。自启任务使用后台模式运行，默认任务名称为 `GdouNetGuard`。

### 强制重新认证（`-reauth`）

先注销再登录，一次性操作后退出：

```powershell
.\GdouNetGuard.exe -reauth
```

自动完成协议同意流程，无需手动勾选网页复选框。

### 试运行（`-dry-run`）

构造登录参数但不提交，用于调试：

```powershell
.\GdouNetGuard.exe -once -dry-run
```

## 完整参数

| 参数 | 默认值 | 说明 |
|---|---|---|
| `-base-url` | `http://10.129.1.1` | 门户基础地址 |
| `-ac-id` | `153` | SRUN `ac_id` |
| `-ssid` | `海大校园网` | WLAN 配置名称（`netsh wlan connect` 使用的 profile 名） |
| `-interval` | `15s` | 守护模式检查间隔 |
| `-timeout` | `8s` | HTTP 超时 |
| `-probe-url` | `http://www.msftconnecttest.com/connecttest.txt` | 外网连通性探测地址 |
| `-probe-contains` | `Microsoft Connect Test` | 探测响应中应包含的文本；设为空则只检查 HTTP 状态码 |
| `-domain` | — | 可选账号域名后缀，如 `@cmcc` |
| `-username-env` | `CAMPUS_USERNAME` | 账号环境变量名 |
| `-password-env` | `CAMPUS_PASSWORD` | 密码环境变量名 |
| `-save-credentials` | — | 从环境变量读取账号密码，用 Windows DPAPI 加密保存后退出 |
| `-forget-credentials` | — | 删除已保存的账号密码后退出 |
| `-credential-file` | `%AppData%\GdouNetGuard\credentials.json` | 加密凭据文件路径 |
| `-background` | — | 在隐藏后台进程中启动持续守护后退出（仅 Windows） |
| `-enable-startup` | — | 创建或更新当前用户的 Windows 开机自启计划任务后退出 |
| `-disable-startup` | — | 删除当前用户的 Windows 开机自启计划任务后退出 |
| `-startup-task-name` | `GdouNetGuard` | 计划任务名称 |
| `-reauth` | — | 强制注销 → 协议同意 → 重新登录后退出 |
| `-dry-run` | — | 只构造参数，不提交登录 |
| `-once` | — | 只运行一轮后退出 |
| `-version` | — | 输出版本号后退出 |
| `-log-file` | — | 日志文件路径；不设置则输出到 stdout |
| `-log-max-size` | `1048576` (1 MB) | 日志超过此字节数时轮转 |
| `-log-max-backups` | `3` | 保留的轮转日志份数 |
| `-pid-file` | `%TEMP%\GdouNetGuard.pid` | PID 文件路径（互斥锁） |
| `-max-probe-fails` | `3` | 互联网探测连续失败 N 次后强制重登；设为 0 禁用 |

## 工作原理

### 守护循环

```
检查校园网在线状态 (rad_user_info)
├── 在线 + 互联网探测正常 → 跳过本轮
├── 在线 + 互联网探测连续失败 ≥ 阈值 → 强制注销重登
├── 校园网不可达 + 有 SSID → WLAN 重连 → 等待 5s → 重试
│   ├── 重试仍不可达 → 再等 3s 重试
│   └── WLAN profile 丢失 → 提示手动重连 WiFi
├── 离线 + 有凭据 → 执行登录流程
└── 离线 + 无凭据 → 报告错误（WLAN 重连仍会执行）
```

互联网探测用于识别"portal 显示在线但实际无法上网"的假在线状态，常见于认证会话过期但 portal 尚未刷新状态时。连续失败达到 `-max-probe-fails` 次后强制重新认证。

### 登录流程

1. 自动同意门户协议条款（`/v1/srun_portal_agree_new` → `/v1/srun_portal_agree_bind`）
2. 访问登录页读取客户端 IP
3. 调用 `get_challenge` 获取 token
4. 使用 token 计算 `{MD5}` 密码、`{SRBX1}` 用户信息和 `chksum`
5. 调用 `srun_portal` 提交登录
6. 登录后用 `rad_user_info` 和外网探测地址确认状态

### WLAN 恢复

当校园网不可达时，守护进程先尝试 WLAN 重连再尝试认证。WLAN profile 丢失时日志会明确提示 `*** WLAN profile ... not found ***`，在 Windows 中重新连接该 WiFi 并勾选自动连接即可恢复。`netsh` 的具体错误信息会完整记录在日志中。

> **注意**：WLAN 重连只能恢复网络连接，不能替代身份认证。首次运行时设置环境变量即可自动保存凭据，后续无需再设环境变量，网络 + 认证均可全自动恢复。

### 凭据安全

- 密码仅在登录时从环境变量或 DPAPI 加密文件按需加载
- `DoLogin` 返回前立即通过 `defer creds.Clear()` 清除内存中的明文密码
- `credentials.json` 中用户名和密码均经 DPAPI 加密，仅当前 Windows 用户可解密

### 日志轮转

- 启动时检查日志文件大小，超出阈值则轮转
- 运行时每次写入前检查，超阈值即时轮转
- 保留最近 `-log-max-backups` 份历史日志（`guard.log.1`、`guard.log.2`...）
- 长期运行不会因日志文件增长占满磁盘

### 优雅退出

收到 `Ctrl+C`（SIGINT）或 SIGTERM 信号时自动清理 PID 文件后退出。

## 编译

```powershell
go test ./...
go build -o GdouNetGuard.exe .
```

## 说明

该门户使用 SRUN challenge 登录流程，且启用了用户协议条款开关（`UserAgreeSwitch`）。本工具在登录前自动调用协议同意 API，无需手动勾选网页复选框。

如果学校后续改成验证码、短信、扫码或动态二次验证，纯命令行自动登录会失效，需要改成半自动或浏览器自动化方案。
