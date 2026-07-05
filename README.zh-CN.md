[English](./README.md) | **简体中文**

<p align="center">
  <img src="https://capsule-render.vercel.app/api?type=waving&color=0:4f46e5,100:0ea5e9&height=160&section=header&text=ctxprof&fontSize=64&fontColor=ffffff&fontAlignY=38&desc=Claude%20Code%20%E4%B8%8A%E4%B8%8B%E6%96%87%E7%AA%97%E5%8F%A3%E5%88%B0%E5%BA%95%E8%A2%AB%E8%B0%81%E5%90%83%E4%BA%86&descAlignY=62&descSize=14&animation=fadeIn" alt="ctxprof header" />
</p>

<p align="center">
  <a href="LICENSE"><img alt="license" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
  <a href="https://go.dev/dl/"><img alt="go" src="https://img.shields.io/badge/go-1.24-00ADD8?logo=go&logoColor=white"></a>
  <a href="https://github.com/SuperMarioYL/ctxprof/actions/workflows/ci.yml"><img alt="CI" src="https://img.shields.io/badge/CI-go%20build%20%7C%20vet%20%7C%20test-success"></a>
  <a href="https://github.com/SuperMarioYL/ctxprof/releases"><img alt="release" src="https://img.shields.io/badge/release-v0.5.0-orange"></a>
  <a href="#"><img alt="Claude Code" src="https://img.shields.io/badge/Claude%20Code-ready-7c3aed"></a>
  <a href="#"><img alt="MCP" src="https://img.shields.io/badge/MCP-aware-0ea5e9"></a>
</p>

<p align="center">
  <b>一个面向 Claude Code 会话的上下文预算 Profiler。</b><br/>
  看清是哪个 skill、哪个 MCP 工具描述符、哪份项目文件、还是模型自己的推理过程把你的 20 万 token 上下文窗口吃掉了 —— 在下一次会话再撞墙之前。
</p>

---

## 为什么是现在

你打开 Claude Code，挂了上周装好的三四个 skill，连了两个 MCP server，开始干一个真活，结果窗口用到 82% 的时候助手让你新开一个 chat。问题是：你装的那七样东西，到底是谁吃掉了预算？厂商的 meter 只会丢给你一个汇总数字；[JuliusBrussee/caveman](https://github.com/JuliusBrussee/caveman)（68k★、单日 1,138 颗星）让模型说话像穴居人省 token，[rtk-ai/rtk](https://github.com/rtk-ai/rtk)（58k★）做命令代理省 token —— 两个都是聪明的 workaround，但都不告诉你*钱花在了哪*。另一边 Uber 刚把单座 AI 支出锁在 $1,500/月（[Simon Willison 的 TIL](https://simonwillison.net/2026/Jun/3/uber-caps-usage/)），"上下文里到底装了什么"已经从手艺活变成了财务问题。`ctxprof` 就是缺失的那台观测器：一个静态 Go 二进制，把 Claude Code 的扁平 token 总数拆成六个桶 —— **system / skill / MCP / file / reasoning / output**，让你能决定卸哪个，而不是靠猜。

## <img src="https://api.iconify.design/tabler:topology-star-3.svg?color=%230071E3&width=24" height="22" align="absmiddle" alt=""> 架构

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="./assets/atlas-dark.svg">
    <source media="(prefers-color-scheme: light)" srcset="./assets/atlas-light.svg">
    <img src="./assets/atlas-light.svg" width="880" alt="架构：一份已完成的 Claude Code JSONL 会话流经 parser，对每个 block 做 chars/4 估算，再按真实的逐 turn message.usage 总数做对齐，归类进六个桶，最后渲染成火焰树或 allocation_v1.json">
  </picture>
</p>

一份已完成的 Claude Code JSONL 会话流经四个纯函数包 —— **parser → estimate → attribute → render** —— 全程零网络调用、链路里没有任何模型。源数据带有真实的逐 turn `message.usage` 总数，但**没有逐 block 的 token 字段**，所以每个 block 先拿到一个本地 `chars/4` 估算值，再由 `attribute` 把这些权重缩放到 turn 的真实总数：会话级总和保持精确，逐桶拆分则成为对齐后的标定估算。每个对齐后的 block 落进**六个桶之一 —— system / skill / mcp / file / reasoning / output**，由 `render` 打印成火焰树或导出为 `allocation_v1.json`。

## 目录

- [架构](#架构)
- [你会看到什么](#你会看到什么)
- [安装](#安装)
- [30 秒上手](#30-秒上手)
- [它是怎么工作的](#它是怎么工作的)
- [对比 caveman —— 互补而非竞争](#对比-caveman--互补而非竞争)
- [配置](#配置)
- [Schema：`allocation_v1.json`](#schemaallocation_v1json)
- [Roadmap](#roadmap)
- [Kill criteria（诚实条款）](#kill-criteria诚实条款)
- [参与贡献](#参与贡献)
- [License](#license)
- [传播话术](#传播话术)

## 你会看到什么

```
session 2026-06-04 14:22 — 184,512 / 200,000 tokens (92%)
├── skills        ████████████░░  47,210  (25.6%)
│   ├── caveman              19,840
│   ├── code-review          14,520
│   └── frontend-design       12,850
├── mcp           █████░░░░░░░░░  18,403  (10.0%)
│   ├── pencil                11,201
│   └── shadcn-ui              7,202
├── system prompt █░░░░░░░░░░░░░   4,128  ( 2.2%)
├── files         ████████░░░░░░  31,990  (17.3%)
├── reasoning     ██████████░░░░  62,540  (33.9%)
└── output        ███░░░░░░░░░░░  20,241  (11.0%)
```

## <img src="https://api.iconify.design/tabler:photo.svg?color=%230071E3&width=24" height="22" align="absmiddle" alt=""> 演示

<p align="center">
  <img src="assets/demo.gif" width="820" alt="ctxprof 先渲染火焰树，再把 allocation_v1.json 管道给 jq">
</p>

> 由 CI 通过 [`docs/demo.tape`](./docs/demo.tape)（vhs）渲染 —— 重新录制方式见 [`assets/README.md`](./assets/README.md)。

## 安装

```bash
go install github.com/SuperMarioYL/ctxprof/cmd/ctxprof@latest
```

或者直接从 [Releases](https://github.com/SuperMarioYL/ctxprof/releases) 下载静态二进制。

国内开发者拉不下来可以通过 Gitee 镜像同步（v0.1.0 后会配置），或者 `GOPROXY=https://goproxy.cn,direct go install …` 走七牛代理。

## 30 秒上手

```bash
# 1. 自动找最近一次 Claude Code 会话（扫描 ~/.claude/projects/）
ctxprof

# 2. 指定具体的 session 文件
ctxprof --session ~/.claude/projects/myproj/abc123.jsonl

# 3. 把结构化结果管给其他工具
ctxprof --json | jq '.buckets'

# 4. 直接看该砍什么——跨所有桶、最大的单个占用项（只读）
ctxprof --cut-candidates 10

# 5. 看预算随时间的漂移——多个会话间每个桶的变化
ctxprof trend --since 7d
ctxprof trend session-a.jsonl session-b.jsonl session-c.jsonl --json

# 6. 只对比两个会话——这次 run 相比上次到底变了什么？
ctxprof compare old.jsonl new.jsonl
ctxprof compare old.jsonl new.jsonl --json | jq '.bucket_deltas'
```

> **`--cut-candidates N`** 在 tree 之后追加一个排序列表：跨所有桶、最大的 N 个具名占用项（某个 skill、某个 MCP server、某个文件路径），并标注它各自占窗口的比例——让你知道该砍哪个。它**只做诊断**：ctxprof 永远不会去编辑或改写一个 session。
>
> **`ctxprof trend`** 一次分析多个会话，按时间（旧→新）打印每个桶占用的变化，让悄悄上涨的 system/mcp/file 预算一眼可见。可传具体路径，或用 `--since 7d` 从 `~/.claude/projects/` 里挑最近的会话。显式传入的路径会按文件 mtime 从旧到新排序，所以像 `ctxprof trend *.jsonl` 这样的 shell 通配也能按正确方向读。`--json` 产出一个有序的 `allocation_v1` 数组。
>
> **`ctxprof compare`** 只对比*两个*会话：对每个桶给出两边的 reconciled token 数以及带符号的差值（新 − 旧），再列出这一对之间变化最大的具名占用项——让你一眼定位两次 run 之间到底动了什么。第一个参数传旧会话、第二个传新会话。同样只读。`--json` 产出两个 `allocation_v1` 对象外加一个 `bucket_deltas` 数组。

<details>
<summary><code>--json</code> 输出示例</summary>

```json
{
  "schema_version": "allocation/v1",
  "session_id": "abc123",
  "window_max": 200000,
  "window_occupancy": 184512,
  "cumulative_tokens": 312880,
  "estimated": true,
  "buckets": {
    "skill":     { "tokens": 47210, "items": [{"name":"caveman","tokens":19840}] },
    "mcp":       { "tokens": 18403, "items": [{"name":"pencil","tokens":11201}] },
    "system":    { "tokens": 4128 },
    "file":      { "tokens": 31990 },
    "reasoning": { "tokens": 62540 },
    "output":    { "tokens": 20241 }
  }
}
```

> 从 v0.2 起，窗口占用百分比按 `window_occupancy`（单轮峰值占用）计算，而不是跨轮累加（累加会把缓存前缀每轮重复计数）。`cumulative_tokens` 作为真实吞吐量单独列出。

</details>

## 它是怎么工作的

Claude Code 的 JSONL 会话日志给你的是**每轮真实的总数**（`message.usage.input_tokens` / `cache_read_input_tokens` / `cache_creation_input_tokens` / `output_tokens`），但**没有每个 content block 自己的 token 字段**。所以桶的数字读不出来，只能先估再校准。ctxprof 每次会话做三件事：

1. **估算** —— 对每个 content block 本地分词（v0.2 起用一个真正的、内置的字节级 BPE tokenizer，取代 v0.1 的 `chars/4` 启发式）。
2. **校准** —— 对每个 assistant 轮次，把这一轮所有估算权重等比缩放，使之精确等于这一轮 `message.usage` 的真实总和。会话级总数因此是精确的，桶级拆分是被校准过的估算。
3. **归类** —— 把校准后的每块权重，按一套**确定性的、不用 LLM 在 loop 里的**分类器折进六个桶之一：

| Block | → 桶 | 含义 |
| --- | --- | --- |
| `type:thinking` | `reasoning` | 模型内部推理痕迹 |
| `type:text`（assistant） | `output` | 给用户看的可见输出 |
| `tool_use` name = `Read` | `file` | 被拉进上下文的项目文件 |
| `tool_use` name = `Skill` | `skill` | `input.command` 即 skill 名 |
| `tool_use` name 以 `mcp__` 开头 | `mcp` | `mcp__<server>__<tool>` |
| `tool_use` 其他（`Bash`、`Edit`…） | `output` | 模型的动作面 |
| `tool_result` | `file` | 工具回带的内容 |
| *（无 per-block 信号）* | `system` | 用首轮 `cache_creation_input_tokens` 近似 |

四个 package，一个静态二进制，零网络调用：

```
cmd/ctxprof → internal/parser → internal/estimate → internal/attribute → internal/render
  (cobra)      (JSONL 流式)      (chars/4 权重)       (分类+校准)            (tree | json)
```

## 对比 caveman —— 互补而非竞争

[`caveman`](https://github.com/JuliusBrussee/caveman) 和 [`rtk`](https://github.com/rtk-ai/rtk) 是 token *省略者*，ctxprof 是 token *观测者*，两者互补：

| | ctxprof | caveman | claude `--stats` |
| --- | :---: | :---: | :---: |
| 显示窗口总占用 | ✓ | 部分 | ✓ |
| 拆到每个 skill | ✓ | — | — |
| 拆到每个 MCP 描述符 | ✓ | — | — |
| 拆到推理痕迹 | ✓ | — | — |
| 主动削减 token | — | ✓ | — |
| 重写模型输出风格 | — | ✓ | — |
| 单一静态二进制、不挂 agent | ✓ | — | ✓ |
| 开放 schema 供其他工具产出 | ✓ | — | — |

如果你已经在用 caveman，ctxprof 告诉你*caveman 替你省了什么*。把 unload 一个 skill 前后的 session 都跑一遍，"装这个 skill 到底值不值"就变成了一个可证伪的问题。

## 配置

v0.1 没有配置文件，所有开关都是 flag：

| Flag | 类型 | 默认 | 含义 |
| --- | --- | --- | --- |
| *（位置参数）* | path | — | 要分析的 JSONL session 文件 |
| `--session` | path | — | 同位置参数，写脚本时更清晰 |
| `--json` | bool | `false` | 输出 `allocation_v1.json` 而不是 tree |
| `--no-color` | bool | `false` | 关掉 ANSI 颜色（管给 `less` 时用） |
| `--window-max` | int | `200000` | 算百分比用的窗口大小 |

不加任何参数时 ctxprof 会扫 `~/.claude/projects/`，挑改动时间最新的 `.jsonl`。

## Schema：`allocation_v1.json`

JSON 结构本身就是护城河。如果有一天 Codex / Aider / Cursor 也想发布"我的上下文里有什么"，它们可以发同一份 schema —— ctxprof 就从"Claude Code 专用工具"退化成"通用 schema 的渲染器"。schema 在 [`internal/schema/allocation_v1.json`](./internal/schema/allocation_v1.json)，五分钟可以读完。

特别欢迎能加第二个 harness 解析器的 PR —— 这是它跳出单厂商工具的唯一办法。

## Roadmap

- [x] **m1 — parse_session.** 流式读 JSONL，按轮次产出真实 `message.usage` 总数 + 类型化 content blocks + 本地估算权重。
- [x] **m2 — attribute_buckets.** 六桶分类器 + 每轮校准；会话级总数精确，桶级拆分是被校准过的估算。
- [x] **m3 — render_treemap.** 真彩色终端 flame-graph tree，以及 `--json` 产出 `allocation_v1.json`。
- [x] **v0.2 — BPE tokenizer.** 用真正的、内置的字节级 BPE tokenizer 替掉 `chars/4`，让校准前的估算更紧。
- [x] **v0.3 — 多会话 trend.** `ctxprof trend` 展示多个会话间每个桶的预算漂移——"这次 run 比上周多/少了什么？"
- [x] **v0.3 — cut-candidates.** `--cut-candidates N` 跨所有桶把最大的单个占用项排序出来，让你知道该砍什么（只读诊断，绝不自动改写）。
- [x] **v0.4 — 两会话 compare.** `ctxprof compare old.jsonl new.jsonl` 只对比两个会话——每个桶 旧→新→Δ，外加变化最大的具名占用项——让定位一次回归只差一条命令。
- [x] **v0.4 — trend 排序修复.** 显式的 `trend` 路径参数（以及 shell 通配展开）按 mtime 从旧到新排序，漂移轴和 `Δ first→last` 列不会再反向。
- [x] **v0.5 — `tool_result` 内容终于被计入.** 读回的文件/工具内容（一次会话里最大的输入）存在于没有 `message.usage` 的*用户*轮里，之前被吞进了 `output`/`reasoning`。现在它会折叠进下一个 assistant 轮的对账池，按读取路径落到 **file** 桶——面向文件密集会话，"我的 token 到底去哪了" 这个核心答案终于是对的。
- [x] **v0.5 — schema 与 flag 修复.** `--json --cut-candidates` 现在能通过 `allocation_v1.json` 校验（schema 认识 `cut_candidates` 了）；`--cut-candidates` 在 `trend`/`compare` 上会报 `unknown flag` 而不再被静默忽略。
- [ ] **v0.x — 第二个 harness 解析器.** Codex 或 Aider，看哪边维护者先点头。
- [ ] **v0.x — CI 模式.** 上下文超预算时 fail 构建。靠真实团队需求驱动，不预先做。

明确**不在** v0.1 范围：Web UI、实时 tail、token→人民币换算、自动给削减建议、托管 SaaS。完整 out-of-scope 列表见 MVP plan §6。

## Kill criteria（诚实条款）

v0.1 上线 30 天后如果同时满足以下三条，**项目终止**：

- 仓库 < 250 stars，且
- 启动渠道之外的有机用户开 < 5 个 issue，且
- 没有外部贡献者加进第二个 harness 解析器。

如果 Anthropic 先一步在 Claude Code 状态栏里内置"你的上下文里有什么"面板，ctxprof 转型成纯 schema 仓库，二进制归档。比起留个僵尸项目，承认它更体面。

## 参与贡献

Issue 和 PR 都欢迎，最有价值的三类：

1. **分类器规则** —— 哪些 tool name 应该归到别的桶？开 issue 附 fixture。
2. **第二个 harness 解析器**（Codex / Aider / Cursor）。对齐 `allocation_v1.json`，tree 渲染白送。
3. **真实 session fixture**（脱敏后）—— 让 test suite 提前抓到别人会踩的坑。

分类器在 [`internal/attribute/classifier.go`](./internal/attribute/classifier.go)，测试是表驱动的，加一条规则就是一个 PR + 一行 fixture。

国内同学的讨论也可以丢到 V2EX 程序员节点或者掘金 AI 标签，开 issue 时贴下链接，我会同步过来。

## License

[MIT](./LICENSE) —— 想干啥干啥，欢迎署名，没有担保。

## 点个 Star

如果你 `go install` 了 ctxprof，而它帮你揪出了本不知道存在的 token 占用，那么在
GitHub 上点个 star 就是在告诉我：真的有人在用。安装都直接走 release 产物，star 是这个
项目值不值得继续做下去的唯一信号。

## 传播话术

```
ctxprof —— Claude Code 的上下文预算 Profiler。
看清是哪个 skill、哪个 MCP 工具、哪份文件、还是推理把你的 20 万 token 窗口吃掉了。
单一静态 Go 二进制，MIT 开源。 https://github.com/SuperMarioYL/ctxprof
```

推送仓库后给它打几个话题，方便被搜到：

```bash
gh repo edit --add-topic claude-code --add-topic mcp --add-topic profiler \
             --add-topic context-window --add-topic developer-tools
```
