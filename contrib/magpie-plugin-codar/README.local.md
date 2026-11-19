# magpie-plugin-codar（本仓库维护版）

上游：<https://github.com/MisonL/magpie-plugin-codar>，基线版本 **0.3.0**（npm `magpie-plugin-codar@0.3.0` 就是它）。

这份是有本地补丁的副本，magpie 通过**本地路径**装它（`~/.config/magpie/plugins.json` 里的 spec 指向这个目录），
所以 `magpie plugin update` 不会用 npm 上的版本覆盖。

## 本仓库相对上游 0.3.0 的补丁

### 1. 修掉「所有 chat 模型 405」

上游把 `loaderBase` 的 `/v1` 剥掉了。magpie 拼请求时会**先去掉路径里的 `/v1`** 再接 base
（`provider/plugins.go`）：

```go
case strings.HasPrefix(rest, "/v1/"):
    url = base + strings.TrimPrefix(rest, "/v1")
```

于是 chat 请求被拼成 `https://api.claw.speedai.com/chat/completions`，而网关只认
`/v1/chat/completions`，其余路径由 nginx 回 `405 Not Allowed`（text/html）。

Claude 模型没暴露这个问题，因为 `pluginRequestBase` 会为 Anthropic 协议单独补回 `/v1`。

改动：

| 文件 | 改动 |
|---|---|
| `index.mjs` | `loaderBase = api`（保留 `/v1`）；初始值 `${API}/v1` |
| `lib/internal.mjs` | `modelOf` 让 chat 与 Anthropic 两种协议的 URL 都以 `/v1` 结尾 |
| `test/plugin.test.mjs` | 断言更新到新契约；两处错误提示接受"网页而非 API"文案 |

### 2. 请求落到非 API 站点时说明原因

`lib/internal.mjs` 新增 `landedOnWebSite()`：应答的 `Content-Type` 是 `text/html`、
体以标签开头、或 4xx 且正文是 nginx 的 `Method Not Allowed` / `405 Not Allowed` 时，
抛出「`<host>` 返回的是网页而不是 API 应答：请求发到了非 API 站点」，
而不是把 405 当成模型故障。

### 3. 地址钉扎

`lib/internal.mjs` 新增 `officialBase()`：官方域里不是网关的（例如 `codar-web.speedai.com`
是官网不是 API），一律纠正回 `https://api.claw.speedai.com`，保留用户自设的非官方地址。

### 4. 模型目录刷新

`lib/catalog.mjs` 从 15 个模型刷新到厂商现网 **20 个**（新增
`deepseek-v4.1-flash-economy`、`gemini-4-argon`、`gemini-4-argon-official`、
`gpt-6.2-astra`、`gpt-6.2-sol`，费率同步）。

### 5. 原生代理二进制

上游 0.3.0 把构建产物拆到了平台包 `magpie-plugin-codar-win32-x64`。
按路径安装拿不到那个包，所以这里把 `codar-proxy-win32-amd64.exe` 放回
`native/bin/`（`lib/native.mjs` 的 `binaryPath()` 会回落到这个位置），
保持与桌面端一致的原生传输。

## 验证

```powershell
cd "I:\开发文件\AI生成测试\magpie\contrib\magpie-plugin-codar"
node --test test/plugin.test.mjs      # 394 pass / 0 fail

# 经 magpie 网关实测（四种模型、两条协议）
# codar/deepseek-v4.1-flash 200 · codar/auto 200 · codar/glm-5.3 200 · codar/claude-sonnet-5-5 200
```

## 跟进上游

上游若修了 `/v1` 的拼接（`loaderBase` 与 `modelOf`），可以直接用上游新版本替换本目录，
再确认 `test/plugin.test.mjs` 里那几处断言是否仍成立。
