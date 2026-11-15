# 错误串的界面翻译（error i18n）——设计

状态：设计稿，2026-11-16。本文只做设计，不改代码。
范围：`internal/gateway` 与 `internal/provider` 的 `fmt.Errorf`／`errors.New`／`writeError`
字面量串；GUI 显示层的翻译机制。CLI（`internal/cli`）是终端界面，另算，不在本期。

## 1 现状与口径

**总量（本次实测，工作树，2026-11-16）**：非测试 Go 代码里，三个错误构造模式共 1393 行
（gateway 541 = writeError 234 + fmt.Errorf 176 + errors.New 131；provider 852 =
fmt.Errorf 472 + errors.New 380，均为含该模式的行数）；其中**实参为字面量**的调用点
1049 处，去重后 **836 个串**（统计口径见文末附录）。
上一轮结论「约 1036 个、14 个有词条」：1036 与本次 1049 吻合；「14 个有词条」未能以
整串匹配复现——把 836 个串对 `i18n.zh.js` 的键做整串与前缀匹配，**7 个被覆盖**（4 整串 +
3 前缀，如 `"no model to try: type one the vendor serves"`）。量级一致（7–14），结论不变：
**约 99% 的错误串无翻译**，用户在界面看到英文原文。

**错误串到达用户的四条路**：

1. HTTP 错误体：`writeError`（`internal/gateway/gateway.go:9255`）按客户端形状写回。
2. 请求日志：同一串再交给日志，`c.Error` 进请求行（`internal/gateway/gateway.go:641`）。
3. GUI：`status(t(err.message), "err")` 等 **21 处**把整串过 `t()`（`app.js:398`、`1205`、
   `1873`、`1891`、`1907`、`1946`、`2369`…）。`t()` 未命中词条时**原样返回**
   （`i18n.js:280-283`），有词条才翻——这就是「有词条就翻、没有就透传」的现成机制。
4. 登录页：`signsheets.js:59/73` 直接显示 `flow.error`，连 `t()` 都没过。

**两条硬约束（决定方案走向）**：

- **错误体必须保持英文**：`writeError` 的文案被 AI 客户端按文本机器判读——
  `saysBusy` 用 `busyWords`/`spentWords` 正则识别忙态并追加
  `"(busy: you can retry your request)"`（`gateway.go:9236-9252`，#2372）；
  `tooLong` 命中时改写为 `prompt is too long`（`gateway.go:9302-9308`），pi 的
  `overflow.ts` 靠它触发压缩。**Go 侧翻译会破坏这套协议语义，禁止。**
- **厂商原话原样透传**：`writeVendorError`（`gateway.go:9264-9275`）保留厂商的
  msg/code/param；翻译只能发生在 GUI 显示层，且必须放过匹配不上的文本。

## 2 分类学：按「用户能行动的程度」分三类

判据（一条亮线）：**用户读完知道下一步做什么 = A；知道坏在哪但修不了 = B；
既无动作也无坏因可报（包装/管道/内部断言）= C。**
分类方式：随机抽 100 个去重串人工逐条判定（seed=42），按比例外推。
样本判定：A 42 / B 35 / C 23 → 外推 **A≈350、B≈290、C≈190**（n=100，95%CI 约 ±10 个
百分点，即 A∈[267,433]）。启发式正则预分（183/105/548）与人工抽检的偏差已计入区间。

### A 可行动（≈350 个）——翻译收益最高，第一期只做这类

| 串（真实原句） | 出处 |
| --- | --- |
| that address isn't from this sign-in: paste the one its browser tab ended on | `internal/provider/claude_signin.go:401` |
| this sign-in is over; start it again | `internal/provider/claude_signin.go:394` |
| the routing group %s is switched off in Magpie, so %q is not served; switch it on again on Magpie's Routing page to use it | `internal/gateway/decide_group.go:45` |
| magpie's web search is off: turn the Web search plugin on in magpie's Settings → Plugins | `internal/gateway/search.go:774` |
| paste the token `claude setup-token` printed | `internal/provider/claude_setup_token.go:79` |
| this is an Anthropic API key, not a setup-token: add it as an API key provider instead | `internal/provider/claude_setup_token.go:81` |
| DashScope's image models take no aspect ratio as size: give WIDTHxHEIGHT (…) | `internal/gateway/draw_dashscope.go:64` |
| /responses/compact is not supported for Magpie models; use a compaction_trigger on /responses | `internal/gateway/codex_backend.go:184` |
| the Codex app started no new app-server: quit it and open it again | `internal/provider/codex_app_server.go:674` |
| send the gateway key as Authorization: Bearer \<key\> or x-api-key, never in the address | `internal/gateway/key_page.go:63` |

### B 需上报（≈290 个）——用户修不了，价值在原样贴报告；第二期

| 串（真实原句） | 出处 |
| --- | --- |
| no balance in the reply | `internal/provider/balance.go:345` |
| ChatGPT sign-in: OpenAI sent no ID token | `internal/provider/chatgpt_api.go:234` |
| claude auth login gave no link to open: … | `internal/provider/claude_signin.go:309` |
| not a System One answer | `internal/gateway/decide.go:253` |
| Codex reset: an answer that says nothing | `internal/provider/codex_resets.go:371` |
| ended: the reply was stuck in a loop | `internal/gateway/fallback.go:2284` |
| the reply ended before it was complete | `internal/gateway/continuation.go:98` |
| OpenAI sent an unreadable ID token | `internal/provider/chatgpt_api.go:276` |
| classifier did not finish | `internal/gateway/classify.go:165` |
| Cline told neither limits nor a balance | `internal/provider/cline_usage.go:79` |

这类串里常带厂商侧证据（下游 %w、HTTP 码），翻译时**证据段必须原样保留**（见 §3 规则④）。

### C 纯调试（≈190 个）——不翻译

| 串（真实原句） | 出处 |
| --- | --- |
| invalid request: model must be a nonempty string | `internal/gateway/gateway.go:8515` |
| invalid request: expected a JSON object | `internal/gateway/gateway.go:8504` |
| decoded by encoding/json | `internal/gateway/fastjson.go:220` |
| struct not decoded field by field | `internal/gateway/fastjson.go:228` |
| incomplete model stream | `internal/gateway/gateway_hooks.go:84` |
| gave way to one with room | `internal/gateway/concurrency.go:122` |
| request body read timed out | `internal/gateway/request_bounds.go:186` |
| unsupported Content-Encoding | `internal/gateway/request_bounds.go:91` |
| the JSON schema isn't valid JSON: %v | `internal/gateway/claude_tui_schema.go:57` |
| unreadable token answer: %w | `internal/provider/chatgpt_api.go:366` |

C 类两个亚型都不翻：**给 AI 客户端的请求校验**（读者是写代码的人/agent，参数名必须原样）；
**纯格式包装**（`"%s: %s"` ×18、`"%s: %w"` ×15——最大的两个字面量，没有可翻的语义）。

## 3 方案选型

| 方案 | 做法 | 优点 | 缺点 | 判 |
| --- | --- | --- | --- | --- |
| 甲：错误码表 | Go 每个调用点换成 `errNoSuchKey` 等码，GUI `t(code)`，码→译文 | GUI 端查找简单、永不失配 | **要改 836 个调用点**；日志与 HTTP 体也变码，丢了英文可读性（`gateway.go:641` 的日志、agent 判读全坏）；和 agent 协议冲突 | ✗ |
| 乙：串键表 | 键=整串英文原文，GUI `t()` 直接命中 | 与现有 `t()` 完全一致（`i18n.js:280`） | 只能覆盖**无变量的串**；带 `%s/%q/%w` 的串（约 60%）键永远对不上 | 只够无变量串 |
| 丙：模板回配（推荐） | 键=英文**模板**，变量写成 `{slot}`；GUI 新增 `tErr(s)`：先整串查，再用槽位正则回配（`tNotice` 已验证的机制，`i18n.js:288-330`） | **零 Go 改动**；厂商原话与尾部 `%w` 原因天然透传（正则匹配不到就原样返回）；日志/HTTP 体保持英文 | 键与 Go 原文**弱耦合**：改词即失配（用测试钉住，§4） | ✓ |

**推荐：丙**。核心理由：

1. 仓库已有同构先例：notice 的 Go 模板 + GUI 正则回配 + 全语言词条测试
   （`internal/agent/notice.go:22-35`、`i18n.js:288-330`、`TestEveryNoticeIsTranslated`
   `internal/agent/notice_test.go:64`）。错误串照抄这套，不发明新机制。
2. 甲案与两条硬约束正面冲突（§1）；乙案覆盖不了带变量的串。
3. 第一期不碰 Go，任何一期都可独立回滚（词条删了即回到透传）。

**四条规则（写进词条表的使用约定）**：

- ① 只收 A/B 类；C 类永不入表。
- ② 模板槽只装**名字、路径、型号、命令、HTTP 码**（notice 同款约束，`notice.go:9-11`）。
- ③ 正则只锚定 magpie 自己的固定前缀；**尾部原因段**（`%w`、厂商体）匹配后原样拼接，永不翻译。
- ④ 消费者是 agent 的 writeError 文案（如 `saysBusy` 追加的 busy 提示、`prompt is too long`）
  **不入表**——它们被 `gateway.go:9236-9348` 的正则按英文判读，翻译即破坏协议。

## 4 分阶段落地

| 阶段 | 内容 | 规模 | 依赖 |
| --- | --- | --- | --- |
| 一 | A 类高频 25 串 + `tErr()` 机制 + 钉住测试 | 25×4 语言词条 + JS ≈40 行 + 2 个测试 | 无（不碰 Go） |
| 二 | A 类其余中低频串；B 类做「固定前缀翻译、证据透传」 | 按反馈频率排，每批 ≤50 | 阶段一机制 |
| 三 | `signsheets.js` 的 `flow.error` 过 `tErr()`；CLI 串另议 | 小 | 阶段一机制 |

### 阶段一机制：`tErr()`（i18n.js，≈40 行，tNotice 的通用化）

```
tErr(s):  ① 整串查 I18N[locale]，命中即回；
          ② 逐模板（按固定长度降序）正则匹配：模板里 {slot} 变成捕获组，
             尾部原因段（"…: " 之后若有剩余）作为 {cause} 捕获；
          ③ 译文取自词条表，槽位回填，{cause} 原样拼接；
          ④ 全部失败 → 原样返回 s（厂商文本由此透传）。
词条表放 ERRORS[]（i18n.js，NOTICES 同款），各语言译文进 i18n.zh/.zh-TW/.ja/.de.js。
```

### 阶段一清单：A 类 25 个高频串（键=英文模板，`{slot}` 为变量）

键名约定：**键即英文模板本身**（t() 语义），不另造 id；下表「键（模板）」列即词条键。

| # | 键（英文模板） | 槽 | 现出处（代表行） | 次数 |
| --- | --- | --- | --- | --- |
| 1 | {provider} has no model {model} (magpie provider {provider} lists them) | provider, model | `internal/provider/modelprefs.go:90` | 11 |
| 2 | {provider} has no such key | provider | `internal/provider/keys.go:345` | 9 |
| 3 | that address isn't from this sign-in: paste the one its browser tab ended on | — | `internal/provider/claude_signin.go:401` | 5 |
| 4 | this sign-in is over; start it again | — | `internal/provider/claude_signin.go:394` | 4 |
| 5 | {provider} has no account {account} | provider, account | `internal/provider/account_caps.go:167` | 7 |
| 6 | no saved {agent} account {account} | agent, account | `internal/provider/keep_login_as.go:79` | 5 |
| 7 | no {agent} account {account} | agent, account | `internal/provider/google.go:295` | 4 |
| 8 | name a model as provider/model, not {got} | got | `internal/provider/modelprefs.go:55` | 4 |
| 9 | that is a routing group, not a provider's model | — | `internal/provider/modelspec.go:256` | 4 |
| 10 | no provider {id} | id | `internal/provider/localid.go:323`（全句 `provider.go:963`） | 4 |
| 11 | the routing group {name} is switched off in Magpie, so {model} is not served; switch it on again on Magpie's Routing page to use it | name, model | `internal/gateway/decide_group.go:45` | 1 |
| 12 | magpie has no routing group {name} | name | `internal/gateway/decide_group.go:47` | 1 |
| 13 | this video was started with a key of {key} that is no longer on: turn it back on to get the video | key | `internal/gateway/video.go:680` | 3 |
| 14 | no {vendor} search API is set up | vendor | `internal/provider/search_api.go:267` | 3 |
| 15 | Kiro's sign-in has expired; sign in again with `kiro-cli login` or the Kiro IDE | — | `internal/provider/kiro.go:362` | 3 |
| 16 | the sign-in was canceled | — | `internal/provider/claude_signin.go:433` | 3 |
| 17 | no balance path: where in the reply the amount is, e.g. data.balance | — | `internal/provider/balance.go:662` | 2 |
| 18 | the balance path has a [ without its ] | — | `internal/provider/balance.go:929` | 3 |
| 19 | turn this account on before moving it first | — | `internal/provider/account_order.go:36` | 2 |
| 20 | accounts changed; reopen the provider and try again | — | `internal/provider/account_order.go:86` | 2 |
| 21 | the picture is over 1 MB; pick a smaller one | — | `internal/provider/icon.go:54` | 2 |
| 22 | tokens {got} is not a length (200000, 200k, 1m) | got | `internal/provider/ruleparse.go:30` | 2 |
| 23 | Codex is signed out; run codex login | — | `internal/provider/account.go:1090` | 2 |
| 24 | the ChatGPT sign-in has expired — sign in again | — | `internal/provider/chatgpt_api.go:82` | 1 |
| 25 | magpie's web search is off: turn the Web search plugin on in magpie's Settings → Plugins | — | `internal/gateway/search.go:774` | 1 |

选入标准：A 类 + GUI 面（模型编辑、账号/登录页、provider 卡片、会话错误）+ 出现次数
（高频优先）+ 模板稳定（版本号类如 "update it to 0.2.7 or later" 不选，键会随版本烂掉）。
`internal/cli` 的串（如 `no group %q`，`internal/cli/groups_cli.go:620`）留待阶段三。

### 钉住不回退（两个测试，照 notice 的现成形状）

1. **词条完整**（Go 测试，仿 `TestEveryNoticeIsTranslated`，`internal/agent/notice_test.go:64`
   ——它已在解析 i18n.js 的语言表，读表函数直接复用）：ERRORS 的每个模板在四种语言表
   都有译文、槽位集合一致；词条里的模板正则能配回它自己的英文原文。
2. **原文未漂移**（Go 测试，仿 `TestNoticesAreTemplates`，`notice_test.go:104` 的 AST 扫描）：
   对每个阶段一模板，扫描 gateway/provider 源码，断言其**固定前缀**（槽位前的英文部分）
   仍以字面量存在。英文改词 → 测试红 → 同一提交里同步改词条。GUI 侧套
   code-standards.md:200 的既有规矩（测试跑中英双语言）。

## 5 工作量与风险

**工作量**（人工判定的设计外推，非实测）：

| 项 | 估计 |
| --- | --- |
| 阶段一 `tErr()` + ERRORS[] + 2 个钉住测试 | 0.5–1 人日 |
| 阶段一 25 串 × 4 语言翻译与校对（zh 先行，zh-TW/ja/de 可晚一拍） | 1–1.5 人日 |
| 阶段二每批 50 串 | ≈2 人日/批（含校对） |
| C 类 | 不投入（0） |

**风险与对策**：

- **翻译质量**：错误串短、缺上下文，易翻出「字典腔」。对策：zh 先出、由中文用户实际反馈
  校对后再铺 ja/de；术语沿用 i18n.zh.js 已有译法（agent/provider/key/routing group）；
  槽位值（路径、命令、型号）永不进译文语序重排之外的位置。
- **原文漂移导致静默回退**：英文改词 → 正则失配 → GUI 又显示英文，**无报错**。
  对策：§4 的测试 2 钉住固定前缀；这是本设计里唯一的新增 Go 测试面。
- **误伤 agent 消费串**：把 writeError 的协议文案收进词条表会破坏 `saysBusy`/`tooLong`
  判读（§1、§3 规则④）。对策：词条表 Review 时对照 `gateway.go:9236-9348` 的正则清单，
  该清单里的词根（busy、retry、prompt is too long、context）禁止出现在任何模板正则的
  匹配范围里。
- **厂商文本漏进模板**：模板锚定过宽会把厂商变体吃进去翻错。对策：规则③（尾部原因段
  透传）+ 测试 1 里加「每个模板只允许 ≤1 个尾部通配段」的形状断言。
- **规模错觉**：836 个串里约 60% 带变量、约 23% 是纯包装；「全量翻译」既不可行也没必要。
  本设计全程只按「用户能行动」排序投入，B 类只做前缀，C 类永不做。

## 附：本次实测口径

- 统计对象：`internal/gateway/*.go`、`internal/provider/*.go` 非 `_test` 文件（515 个），
  `fmt.Errorf`／`errors.New` 的字面量首参 + `writeError*` 调用里的最后一个字面量实参。
  Python 脚本（临时目录，`python -I` 运行）正则提取；调用点 1049、去重 836。
- 「已有词条」复核：836 串对 `i18n.zh.js` 键做整串 + 前缀匹配（键长 >15 才算前缀），7 串
  被覆盖。上一轮「14」未能按此口径复现，差异可能来自子串匹配口径；两者都说明覆盖 <2%。
- 分类：`random.sample(seed=42)` 抽 100 个去重串人工逐条判定（A 42 / B 35 / C 23），
  二项 95%CI ≈ ±10 个百分点；外推总数时按比例计算。启发式正则预分（183/105/548）与
  人工抽检的偏差已计入区间。
- 本工作树含未提交改动（`git status` 见 gateway/provider 多文件 M），数字反映工作树而非
  HEAD；提交前后若再有增删，按同口径重跑即可。
