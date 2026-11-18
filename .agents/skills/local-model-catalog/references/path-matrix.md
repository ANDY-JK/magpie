# 路径矩阵与探测命令

原则：**先看真实进程和安装记录，再看环境变量，最后才用 OS 默认路径。** 默认路径只是候选，不是事实。

## Windows

| 用途 | 候选 |
|---|---|
| 用户数据 | `%APPDATA%\<App>`、`%LOCALAPPDATA%\<App>`、`%LOCALAPPDATA%\Programs\<App>`、`~\.local\share\<App>`、`~\.config\<App>` |
| 程序 | `%ProgramFiles%\<App>`、`%ProgramFiles(x86)%\<App>`、`%LOCALAPPDATA%\Programs\<App>`、自定义盘（如 `I:\Programs\<App>`） |
| 安装记录 | `<dataDir>\install.json`、`<dataDir>\<app>-install.json` 的 `dataDir` / `installDir` |
| Electron 子目录 | `<dataDir>` 本身、`<dataDir>\desktop-app` |
| 账号 | `auth.json` 的 `activeProfileId`（常为 UUID）→ `profiles[].bceAccountId` / `userId`；磁盘目录可能用后者 |
| 当前账号 | `<dataRoot>\current-owner.json` 的 `ownerDir`（也可能叫 `scope` / `accountId`） |
| 配置根 | 可能不等于数据根：数据在 `~\.local\share\<App>`，配置在 `~\.config\<App>` |

探测：

```powershell
Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -match '<app>' } |
  Select-Object ProcessId,Name,ExecutablePath,CommandLine | Format-List

$s = (New-Object -ComObject WScript.Shell).CreateShortcut("$env:APPDATA\Microsoft\Windows\Start Menu\Programs\<App>.lnk")
$s.TargetPath; $s.WorkingDirectory
```

注意：

- junction / symlink 很常见；`install.json` 的 `dataDir` 比 `%APPDATA%` 可信。
- 数据可能在 `C:`，程序在 `I:`；不要因为程序在自定义盘就以为数据也在那里。
- 有的应用（例如 TeleAgent）在 Windows 也把数据放 `~/.local/share`，不是 `%APPDATA%`。

## macOS

| 用途 | 候选 |
|---|---|
| 用户数据 | `~/Library/Application Support/<App>`、`~/Library/Application Support/<App>/desktop-app` |
| 偏好 | `~/Library/Preferences/<bundle-id>.plist` |
| 缓存 | `~/Library/Caches/<App>`、`~/Library/Caches/<bundle-id>` |
| 程序 | `/Applications/<App>.app`、`~/Applications/<App>.app` |
| 凭据 | 钥匙串 service，例如 `"<App> Safe Storage"` |

探测：

```bash
ps aux | grep -i '<app>'
ls -la ~/Library/Application\ Support | grep -i '<app>'
security find-generic-password -s '<App> Safe Storage' -w   # 只在需要时；不要打印到日志
```

## Linux

| 用途 | 候选 |
|---|---|
| 配置 | `$XDG_CONFIG_HOME/<app>` 或 `~/.config/<app>` |
| 数据 | `$XDG_DATA_HOME/<app>` 或 `~/.local/share/<app>` |
| 状态 | `$XDG_STATE_HOME/<app>` 或 `~/.local/state/<app>` |
| 程序 | `/opt/<App>`、`/usr/lib/<app>`、AppImage、`~/.local/bin` |
| desktop 文件 | `~/.local/share/applications/*.desktop`、`/usr/share/applications/*.desktop` |

## 显式覆盖优先级

统一按这个顺序，代码里也这样实现：

1. 明确的自定义配置文件路径（如 `<APP>_OPENCODE_JSON` / `<APP>_MODEL_CONFIG`）
2. XDG/数据根目录变量（如 `<APP>_XDG_DIR`、`<APP>_DATA`、`<APP>_CONFIG`）
3. 安装记录里的 `dataDir`
4. OS 默认候选（含 `desktop-app/` 子目录）
5. 扫描账号目录，取存在且可解析的最近一个

## 账号目录定位

不要写死账号 ID，也不要按目录顺序取第一个配置。多账号应用（例如 TeleAgent）可能同时存在多个 `users/<scope>/`，旧账号的配置也在；先读 `current-owner.json` 的 `ownerDir`，再用它定位配置。找不到 owner 文件时，才退回 `auth.json` 的 profile 映射。

```text
auth.json:
  activeProfileId = <uuid>
  accountProfiles[] = [{ profileId: <uuid>, bceAccountId: <dir-name>, displayName: ... }]

current-owner.json:
  ownerDir = <scope>          # 优先用这个

磁盘:
  <dataRoot>/<xdg>/<bceAccountId>/config/opencode/opencode.json
  <configRoot>/users/<scope>/<App>.jsonc          # 应用自有布局
```

同时试 `ownerDir`、`bceAccountId`、`profileId`、`activeProfileId`，按顺序取第一个存在的；都没有再扫描同级目录，按 mtime 选最新可解析的。

## 常见误判

- 只看 `%APPDATA%`：漏掉 `~/.local/share`、自定义盘、`desktop-app/`。
- 把 UUID 当目录名：目录可能是 `bceAccountId`。
- 把安装目录当数据目录：`installDir` 与 `dataDir` 不同。
- 把网页登录痕迹（Edge/Chrome IndexedDB）当桌面配置：那只是浏览器使用记录，没有模型清单。
