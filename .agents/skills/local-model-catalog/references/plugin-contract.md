# magpie 插件契约（模型清单相关）

## 一个 provider 插件的最小形状

```js
export const Plugin = async () => ({
  config: async (config) => {
    // 这里给 magpie 的模型清单；host 只加载时调用一次
  },
  provider: {
    id: "<provider-id>",
    async models(provider) {
      // provider.models 是 config 钩子给出的模型；可在这里补 rate/free
      return provider.models
    },
  },
  auth: {
    provider: "<provider-id>",
    methods: [ /* oauth / api 登录方式 */ ],
    async loader(getAuth) {
      // 每个请求：读 auth、换 token、返回 { baseURL, apiKey, fetch }
    },
    async usage(getAuth) { /* 余额/积分 */ },
    async checkin(getAuth) { /* 每日签到，可选 */ },
  },
})
```

## 模型字段（config 钩子）

`models` 是一个对象：键是 magpie 里显示的模型 id（`provider/<key>`），值：

```js
{
  name: "显示名",
  id: "<真实 API id>",          // 请求体里的 model 用它；不写则等于键
  limit: { context: 1000000, output: 384000 },
  modalities: { input: ["text", "image"], output: ["text"] },
  tool_call: true,
  reasoning: false,
  attachment: true,             // 是否接受图片/视频/音频附件
}
```

host 的归一化（节选）：

- `api.id = m.id ?? key` —— **键只是 magpie 的 id，`id` 才是发给上游的值**。
- `limit.context` 来自 `limit.context`；有些配置只给 `limit.input`，要转换。
- `modalities.input/output` 只保留 `text`/`image`/`video`/`audio`；`tool_result` 不是模态。
- `reasoning`、`tool_call`、`attachment` 映射到 capabilities。

## 别用 `??=` 锁死模型

```js
// 不推荐：已有 provider 配置时永远不会更新模型
config.provider[id] ??= { models: buildModels() }

// 推荐：保留用户设置，但每次重新给模型
const was = config.provider[id] ?? {}
config.provider[id] = { ...was, name, npm, api, models: catalog.defs }
```

`was.models` 是用户手动写在 magpie 配置里的模型/覆盖；把它作为 `custom` 保留，并最后覆盖同名项。

## 登录态存放

- magpie 把插件登录存在 `plugin-auth.json`，按 **provider id** 索引，不按 spec。
- 同一 provider id 换插件 spec（例如从 npm 包换成本地路径）不会丢登录。
- 桌面端登录态（`auth.json`、`token.json`）属于应用本身，卸载应用可能一起消失；插件若不在 magpie 侧保存可刷新凭据，就不能声称“卸载独立”。

## 本地插件路径

`plugins.json` 的 `spec` 可以是绝对路径：

```json
{ "spec": "C:\\Users\\<user>\\.config\\magpie\\plugin-local\\<name>" }
```

- 用 `magpie plugin add "<绝对路径>"` 添加；`plugin rm "<旧 spec>"` 移除旧包。
- 不要改 `plugins/node_modules`，`plugin update` 会覆盖。
- 本地目录至少要 `package.json` + `index.mjs`（或 `main`/`exports` 指向的文件）。

## 可见性开关

模型“存在但看不到”通常是这三层：

1. `provider.models` 没给（插件 config/models 钩子问题）。
2. `providers.json` 的 `noModels: true`（界面“Select none”）：`magpie provider models <id> all`。
3. `providers.json` 的 `unlisted: true`（“Only through routing groups”）：`magpie provider listed <id> yes`。

网关 `/v1/models` 是最终给 agent 的清单；`plugin-providers.json` 是插件最近一次列出的原始结果，两者都要看。

## 请求链路

- 插件 `auth.loader` 返回的 `fetch` 负责最终请求；它拿到的是 magpie 已转换好的 OpenAI/Anthropic 请求。
- 云端地址要用应用/插件常量里的真实地址；`127.0.0.1:<port>` 多半是桌面端本地代理。
- 验证时先直接调云端（确认地址与 token），再经 magpie 网关调一次（确认插件链路）。
