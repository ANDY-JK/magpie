# 模型提取、过滤与保留规则

## 1. 只取必要字段

从源配置的每个模型里取：

| 源字段 | 目标字段 | 说明 |
|---|---|---|
| key | magpie 模型键 | 显示/路由名，可能不是 API id |
| `id` | `id` / 实际 API id | 请求体里的 `model`；缺失时用 key |
| `name` | `name` | 没有就用 key，不要编造 |
| `limit.context` 或 `limit.input` | `limit.context` | 上下文窗口 |
| `limit.output` | `limit.output` | 输出上限；0 表示未知 |
| `modalities.input` | `modalities.input` | 只留 text/image/video/audio |
| `modalities.output` | `modalities.output` | 只留 text/image/video/audio |
| 能力标记 | `tool_call` / `reasoning` / `attachment` | 有明确依据才置 true |

不要取：`apiKey`、`baseURL`、`headers`、`options`（除非是模型自身的白名单字段）、`plugins`、`commands`、`permission`、`skills`。

## 2. 键与 API id 的区别

配置键可能是别名：

```json
"glm-5": { "id": "model-text", "limit": { "context": 1000000, "output": 128000 } },
"model-text": { "id": "model-text", "limit": { "context": 1000000, "output": 384000 } }
```

规则：

- magpie 的模型键用 **API id**（`model-text`），请求才发得对。
- 同一 API id 只留一条；`key === id` 的条目优先（它是规范项）。
- 别名只用来补显示名/别名记录，不单独生成模型。
- 去重后再排序（按 id 字典序），保证每次导入结果稳定、可 diff。

## 3. 排除内部入口

排除这些（大小写不敏感，按 key 与 id 都匹配）：

- 内部任务：`model-aiassist`、`model-artifact-validate`、`model-coding-routing`、`model-followup`、`model-gui-use`、`model-meeting-notes`、`model-ppt-validate`、`model-reason-summary`、`model-routing`、`model-routing2`、`model-tm-*`、`model-wiki`、`*-fallback`（内部回退路由）
- mock：`llm-mock`、`*mock*`
- OCR：`qianfan-ocr`、`*-ocr`、`*ocr*`
- 向量：`*embedding*`、`*embed*`
- 重排：`*reranker*`、`*rerank*`
- 校验/安全：`*validate*`、`tele-safety-*`、`*safety*`

**保留**：通用多模态网关（`model-mm`）、自动路由别名（`model-text`、`dm-auto-model/*`）、真实聊天模型（即使名字里有 `vl`）。

判断标准：能被用户当聊天模型直接调用就保留；只服务内部流程、评测、检索、重排、审核的排除。拿不准时保留并在报告里注明。

## 4. 去重与合并

```text
for each (key, raw):
  if excluded(key) or excluded(raw.id): skip
  apiId = raw.id || key
  if !seen[apiId] or key === apiId:
     seen[apiId] = { key, model }
```

别名（`key !== apiId`）只在没有规范项时临时占位；规范项出现后替换。

## 5. models.json 结构

```json
{
  "version": 1,
  "importedAt": "2026-01-01T00:00:00.000Z",
  "sourceKind": "Provider.models",
  "models": {
    "<api-id>": {
      "id": "<api-id>",
      "name": "显示名",
      "limit": { "context": 1000000, "output": 384000 },
      "modalities": { "input": ["text", "image"], "output": ["text"] },
      "tool_call": true,
      "reasoning": false,
      "attachment": true
    }
  },
  "custom": {}
}
```

- `models`：最近一次成功导入；源缺失/损坏时原样保留。
- `custom`：用户手动添加或覆盖；每次导入都保留，且最后覆盖同名项。
- 原子写：写 `.tmp` 再 rename，避免中途损坏。
- 文件里不出现任何凭据、账号 ID、端口、用户名。

## 6. 重新导入策略

| 源状态 | 行为 |
|---|---|
| 存在且可解析，至少 1 个有效模型 | 覆盖 `models`，更新 `importedAt`，保留 `custom` |
| 存在但解析失败/0 个有效模型 | 保留 `models`，记录一次失败原因（日志，不写敏感信息） |
| 不存在 | 保留 `models`，不报错 |
| 首次运行且从未成功导入 | 用最小内置清单兜底，标记 `imported: false` |

## 7. 用户模型保留

用户手动加的模型可能：

- 写在 magpie 插件配置的 `provider.<id>.models` 里；
- 或通过插件自己的 `custom` 保存。

合并顺序：

```js
defs = { ...importedModels, ...customModels, ...userConfigModels }  // 后者覆盖前者
```

但不要把插件自己上次生成的 `defs` 当成用户模型再存回 `custom`，否则永远无法跟随源更新。只把“源里没有的键”或“用户显式覆盖的键”当 custom。

## 8. 验证样例

```text
导入：43 个键 → 排除 19 个内部/mock/OCR/向量/重排 → 24 条
去重：glm-5（id=model-text）与 model-text 合并 → 23 条
保留：deepseek-v41-flash、model-text、dm-auto-model/*、model-mm
排除：llm-mock、qianfan-ocr、qwen3-embedding-4b、qwen3-reranker-4b、
      model-routing、model-followup、model-ppt-validate、tele-safety-answer
```

最后用 `magpie provider <id>` 与 `/v1/models` 核对到同一组 id。
