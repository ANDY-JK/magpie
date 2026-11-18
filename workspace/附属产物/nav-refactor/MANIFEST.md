# 原型文件清单（唯一来源）

最后整理：2026-11-18。阅读顺序：① 骨架 → ② 层级 → ③ 应用 → ④ 浮层 → ⑤ 对照。入口 `index.html`；完整入口 `complete.html`。

## 当前（会继续改）

| 文件 | 是什么 | 规模 | 备注 |
|---|---|---|---|
| `skeleton-components.html` | 骨架与通用组件 | 外壳 + 4 导航组件 + 11 内容组件 | 骨架宽/窄、明暗 |
| `all-pages.html` | 全部页面与页内视图 | 32 个视图 + 每页元素表 + 32 行总索引 | 顶部可开「标注」 |
| `overlays.html` | 弹窗与浮层 | 30 个 + 33 行全界面清单 | 含托盘、手机、外部流程说明 |
| `complete.html` | 完整入口 | 应用 + 浮层 + 骨架 + 层级 | 顶部四标签，先看这个 |
| `index.html` | 总览入口 | 4 个当前入口 + 版本关系 + 文件清单 | 本页 |
| `AUDIT.md` | 检查与遗漏清单 | 缺口优先级 + 布局方案 A–E | 2026-11-16 |
| `COMPARE.md` | 与现行版对照 | 30 个界面的外观/行为/状态 | 2026-11-18 |

## 定稿（不再改）

| 文件 | 是什么 | 说明 |
|---|---|---|
| `framework-layers.html` | 四层导航框架 | 一级 5 分区 / 二级页面 / 三级标签 / 四级工具栏；外壳已定 |

## 截图

只保留 8 张：`complete.png`、`index-light.png`、`page-providers.png`、`prov-inline.png`、`overlays-light.png`、`skeleton-wide.png`、`layers-narrow.png`、`state-empty.png`。其余已清理，运行原型为准。

## 规则

1. 新增原型先在 `index.html` 和本文件登记，再开工。
2. 被取代的文件已清理，git 历史可查；新增先登记。
3. 描述改动统一用：`页面 · 区域号 · 区域名`，组件名见 `all-pages.html` 的 C01–C16 索引。
