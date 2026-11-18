---
name: local-model-catalog
description: 从本机桌面应用、客户端或插件里定位并解析真实模型清单，导入 magpie 插件并在插件自己的 models.json 中独立保存：路径探测、登录态只读解密、模型键与真实 API id 区分、内部/mock/OCR/向量/重排入口过滤、按 API id 去重、源文件缺失时保留最近有效清单、保留用户手动模型，以及卸载独立性验证。用户说“解析/提取/导入某个客户端或控件的真实模型列表”“新控件/新插件接入”“模型列表不对/不全”“看看卸载后还能不能用”时使用。Use when the user asks to parse or import a provider's real model catalog from a desktop app or local plugin, add a new provider widget, fix a model list, or verify a provider still works without its desktop app.
---

# 本机真实模型清单 → magpie 插件

目标：把某个客户端**真正可用的模型**提取出来，变成 magpie 里稳定、可离线、可独立保存的清单，并验证请求真的能通。

核心原则：**只信本机真实文件与真实请求，不信界面文案、README 或占位值。**

## 0. 先判定类型

| 类型 | 特征 | 模型来源 | 登录来源 |
|---|---|---|---|
| 桌面应用 + 本地配置 | 有安装目录、`auth.json`/`token.json`、`opencode.json`/`*.jsonc` | 本地 provider 配置 | 本地加密凭据 |
| 纯云端插件 | 无桌面安装，只有 API 域名 | 云端模型目录接口 | magpie 的 `plugin-auth.json` |
| 混合 | 有桌面端，但模型/登录走云端 | 云端目录 + 本地缓存 | 本地凭据换 token |

先回答“有没有本地模型配置”；没有就不要硬造解析器，直接走云端目录并说明。

## 1. 红线

- 登录态只在内存里读、解密、换 token；**不输出、不落盘、不写进 models.json** 任何 Cookie、token、密钥、完整认证文件。只报“cookie 数量 / 是否含关键项 / 到期时间 / 账号名”。
- 不退出账号、不重启 magpie、不停相关服务；插件 host 因安装插件而重载是正常现象，不要重启整个应用。
- 改配置前先备份：`plugins.json`、`plugin-auth.json`、`plugin-providers.json`、`plugins/package.json`、`bun.lock`。
- 不修改包管理器维护的 `node_modules`；用本地插件目录（见第 5 步）。
- 不把 `127.0.0.1` 本地代理当成卸载后仍可用的云端 API；不把占位 `apiKey` 当用户密钥。
- 失败如实报阶段（定位/解密/换 token/云端请求），不为通过测试偷偷改走本地代理。

## 2. 定位真实安装与数据

按顺序查，别只按默认路径猜：

1. **进程**：`Get-CimInstance Win32_Process | ? {$_.ExecutablePath -match '<app>'}` / `ps aux | grep`，拿到真实 exe 路径。
2. **快捷方式**：Windows 解 `*.lnk` 的 TargetPath；macOS 看 `/Applications`、`~/Applications`；Linux 看 `.desktop`。
3. **安装记录**：`install.json`、`*-install.json`、`app.json` 里的 `dataDir` / `installDir`（自定义盘、portable、junction 都靠它）。
4. **环境变量**：`<APP>_USER_DATA_DIR`、`<APP>_DATA`、`<APP>_CONFIG`、`<APP>_XDG_DIR` 等显式覆盖优先。
5. **数据目录候选**：根目录本身 **和** `desktop-app/` 子目录都要查；Windows 不一定是 `%APPDATA%`，也可能在 `~/.local/share/<App>`；macOS 是 `~/Library/Application Support/<App>`；Linux 是 `~/.config/<App>` 或 `~/.local/share/<App>`。
6. **账号目录**：配置里的 `activeProfileId` 可能只是 UUID，磁盘目录可能用 `bceAccountId`/`userId`；两个都试，别写死账号 ID。
7. **当前账号文件**：多账号应用常有 `current-owner.json` / `owner.json`，字段是 `ownerDir` / `scope` / `accountId`。**先读它再找账号目录**；不要拿“第一个 `*/App.jsonc`”，那可能是旧账号（试跑时 TeleAgent 就有两个账号，第一个是旧的）。
8. **配置根 ≠ 数据根**：有的应用数据在 `~/.local/share/<App>`，配置在 `~/.config/<App>`；两个根都要当候选，并支持用 `accountConfig(root, account)` 自定义账号配置文件路径。

详细路径矩阵见 `references/path-matrix.md`。

## 3. 只读解析登录态

找这些文件（按应用而定）：

- `auth.json` + `.cookie-key`：AES-256-GCM 常见布局 `iv(12) + tag(16) + ciphertext`；
- `token.json`：Electron `safeStorage`（Windows DPAPI / macOS Keychain / Linux keyring），或明文/自定义加密；
- 浏览器 Cookie / IndexedDB：只用于确认“有没有网页登录”，不要当桌面配置。

做法：

- 写一个**独立进程**的探针脚本，读入路径作为参数，不把脚本放进下载目录。
- 解密成功只打印：cookie 数量、关键 cookie 名是否存在、token 长度、到期时间、账号名；绝不打印值。
- 解密失败就报具体阶段与错误类型，不要尝试绕过系统凭据保护。

判断“能否卸载独立”：

| 凭据位置 | 卸载后 | 结论 |
|---|---|---|
| 桌面应用数据目录里的 token | 立即失效 | 不能独立；除非用户另有登录方式 |
| magpie `plugin-auth.json` 里的 Cookie/token | 保留 | 可独立，到期前可用 |
| 云端 OAuth + 可刷新 | 保留且可续 | 可独立 |
| 只有桌面端 token 且无 refresh | 到期即失效 | 明确告知到期日与重登方式 |

## 4. 找到真实模型配置

常见位置：

- OpenCode 风格：`<dataDir>/<xdg>/<account>/config/opencode/opencode.json`，看 `provider.<Name>.models`；
- 应用自有：`<configDir>/users/<scope>/<App>.jsonc`，看 `provider.<Name>.models`；
- 云端：模型目录接口（如 `/api/v1/.../models`），无本地文件。

提取规则见 `references/extraction-rules.md`，要点：

- **键 ≠ API id**：配置键可能是显示/路由名，`id` 才是请求要发的值（例：`glm-5` 的 `id` 是 `model-text`）。
- 只取：`id`、`name`、`limit.context`/`limit.input`、`limit.output`、`modalities.input/output`、`tool_call`、`reasoning`、`attachment`。
- 不取：`apiKey`、`baseURL`、`headers`、`plugins`、`commands`、`permission`。
- 排除内部入口：`mock`、`ocr`、`embedding`、`reranker`、`validate`、`safety`、内部任务角色（`*-routing`、`*-followup`、`*-summary`、`*-assist`、`*-validate` 等）。
- 按真实 API id 去重；键等于 API id 的条目优先，别名只补名称。
- 排序稳定，便于 diff。

## 5. 独立保存 + 本地插件接入

### models.json（插件自己的文件）

放在插件目录，结构至少：

```json
{
  "version": 1,
  "importedAt": "2026-01-01T00:00:00.000Z",
  "sourceKind": "Provider.models",
  "models": { "<api-id>": { "id": "<api-id>", "name": "...", "limit": {"context": 0, "output": 0}, "modalities": {"input": ["text"], "output": ["text"]} } },
  "custom": {}
}
```

行为：

- 源文件存在且可解析 → 重新导入，覆盖 `models`；
- 源文件缺失/损坏 → **保留最近一次有效 `models`**，绝不清空；
- 用户手动添加的模型和覆盖设置 → 放 `custom`，每次导入都保留，且最后覆盖同名项；
- 第一次运行且从未导入过 → 用最小内置清单兜底，并明确标记未导入。

模板：`assets/catalog-importer.mjs`。

### 插件安装

1. 复制整个插件到 `~/.config/magpie/plugin-local/<name>`（或项目约定的本地插件目录）。
2. 用 magpie 自己的命令切换，不手改 `node_modules`：
   - `magpie plugin add "<绝对路径>"`
   - 若旧包 spec 还在并争同一个 provider：`magpie plugin rm "<旧 spec>"`
3. 保持 provider id 不变，`plugin-auth.json` 里的登录态按 provider id 保存，换 spec 不会丢。
4. 插件 `config` 钩子里不要用 `??=` 锁死模型；用 `...was` 保留用户设置，再显式设置 `models`。

### 让模型真的可见

模型没出现，按顺序排查：

1. `magpie provider models <id> all`（或 `+<model>`）——是否被“Select none / 只选了部分”挡住；
2. `magpie provider listed <id> yes`——是否被 “Only through routing groups / unlisted” 挡住；
3. `magpie plugin list --json` 与 `magpie provider <id>`——插件是否加载、是否已登录；
4. `GET /v1/models`——网关实际给 agent 的清单；
5. 仍不对，看 `plugin-providers.json` 里该 provider 的 `models` / `accounts`。

## 6. 验证（必须做）

1. **清单**：插件目录下 `models.json` 有预期条数；`magpie provider <id>` 显示相同条数。
2. **可见**：`/v1/models` 含 `<provider>/<target-id>`。
3. **极小请求**：经 magpie 网关 `POST /v1/chat/completions`，`max_tokens: 1`，确认 HTTP 200 且 `model` 回显正确。不要批量测。
4. **源缺失**：用显式环境变量把源配置指向不存在的路径，确认仍加载最近有效清单且条数不变。
5. **卸载独立性**：**新进程**重新换 token，再直接调云端；不要复用旧进程缓存。若换 token 失败，报阶段，不改走本地代理。
6. **自定义保留**：给 `custom` 加一个模型，再重新导入，确认它还在。

验证模板：`assets/verify-fresh-process.mjs`。

## 7. 交付格式

简洁给：

- 真实安装位置 / 数据目录 / 账号配置；
- 登录状态（账号名、到期时间，不报密钥）；
- 导入数量与可用模型 ID；
- 实测结果（HTTP 状态、目标模型）；
- 能否安全卸载目标应用，以及剩余依赖（token 到期、无 refresh、云端目录不可用等）；
- 改过的文件与备份位置。

## 8. 常见坑速查

- **只查 `%APPDATA%`**：Windows 应用也可能用 `~/.local/share`；macOS 用 `Application Support`；还要查 `desktop-app/` 子目录。
- **UUID ≠ 目录名**：`activeProfileId` 往往是 UUID，目录用 `bceAccountId`/`userId`；两个都试。
- **多账号拿第一个**：别按目录顺序取第一个 `App.jsonc`；先读 `current-owner.json` 的 `ownerDir`，再定位账号配置。
- **键 ≠ API id**：`glm-5` 可能只是 `model-text` 的别名；按 API id 去重，请求发 API id。
- **本地代理**：`127.0.0.1:<port>` 是桌面端本地代理，卸载后不存在；真正云端地址要从插件/应用常量里找。
- **占位 apiKey**：`apiKey: "<app>"` 只是占位，不是用户云密钥；不要导入、不要展示。
- **`config` 钩子 `??=`**：已有 provider 配置时不会更新模型；改成显式合并。
- **选 none / unlisted**：模型存在但不可见，先查 `noModels` 与 `unlisted`。
- **插件更新覆盖**：改 `node_modules` 会在 `plugin update` 后丢失；用本地路径 spec。
- **云端目录会变**：同一账号的模型数可能从 4 变 13；每次都重新拉取，报告以当次结果为准，别引用旧清单。
- **fresh process**：旧进程的 token 缓存会掩盖卸载问题；必须新进程重换。

## 9. 已验证案例（2026-11-18 试跑）

| 目标 | 结果 |
|---|---|
| DuMate（本地 opencode.json） | 43 键 → 过滤/去重后 **23** 个；`deepseek-v41-flash` 可用；源缺失保留 23 |
| TeleAgent（`.local/share` + `.config`） | 当前 owner 的配置解析出 **5** 个聊天模型；另 1 个 `tele-safety-answer` 排除；登录 token 在桌面数据目录，卸载即断 |
| QwenWork（纯云端） | 无桌面安装/本地配置；云端目录本次返回 **13** 个，magpie 已选 1 个；登录在 `plugin-auth.json` |
| 合成“新控件”夹具 | 别名 `glm-5→model-text` 正确合并；mock/OCR/向量/重排/安全/图片入口全部排除；源缺失/损坏保留 3 条；custom 模型保留 |

## 参考文件

- `references/path-matrix.md` — Windows/macOS/Linux 路径与探测命令
- `references/plugin-contract.md` — magpie 插件钩子与模型字段
- `references/extraction-rules.md` — 过滤、去重、别名、保留规则
- `assets/catalog-importer.mjs` — 独立清单模板
- `assets/verify-fresh-process.mjs` — 新进程独立性验证模板
