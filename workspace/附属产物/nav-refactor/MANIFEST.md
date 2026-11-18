# 原型文件清单（唯一来源）

最后整理：2026-11-16。只从 `index.html` 进入；本文件是给人和 agent 看的台账。

## 当前（会继续改）

| 文件 | 是什么 | 规模 | 备注 |
|---|---|---|---|
| `skeleton-components.html` | 骨架与通用组件 | 外壳 + 4 导航组件 + 11 内容组件 | 骨架宽/窄、明暗 |
| `all-pages.html` | 全部页面与页内视图 | 32 个视图 + 每页元素表 + 32 行总索引 | 顶部可开「标注」 |
| `overlays.html` | 弹窗与浮层 | 30 个 + 33 行全界面清单 | 含托盘、手机、外部流程说明 |
| `index.html` | 总览入口 | 4 个当前入口 + 版本关系 + 文件清单 | 本页 |
| `AUDIT.md` | 检查与遗漏清单 | 缺口优先级 + 布局方案 A–E | 2026-11-16 |

## 定稿（不再改）

| 文件 | 是什么 | 说明 |
|---|---|---|
| `framework-layers.html` | 四层导航框架 | 一级 5 分区 / 二级页面 / 三级标签 / 四级工具栏；外壳已定 |

## 提案（等确认）

| 文件 | 是什么 | 说明 |
|---|---|---|
| `grouping-options.html` | 导航分组 A/B/C | 3 套方案，尚未选 |

## 历史（只读，不再改）

| 文件 | 是什么 | 被谁取代 |
|---|---|---|
| `layout-detail.html` | 六页细节版 | `all-pages.html` |
| `layout-frameworks.html` | 宽窄两套骨架 | `framework-layers.html` |
| `prototype.html` | 三方案（英文） | `framework-layers.html` |

## 截图前缀

- 当前：`skeleton-*`、`page-*`、`tab-*`、`spec-*`、`no-spec-*`、`overlays-*`
- 历史：`detail-*`、`frame-*`、`group-*`、`nav-*`

## 规则

1. 新增原型先在 `index.html` 和本文件登记，再开工。
2. 被取代的文件不删，移到“历史”，加“历史版本”角标。
3. 描述改动统一用：`页面 · 区域号 · 区域名`，组件名见 `all-pages.html` 的 C01–C16 索引。
