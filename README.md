# GoOn

跨 agent 客户端的**会话交接** CLI。当你在 Claude Code / Codex / opencode / zcode 之间来回切换（往往是因为当前客户端的模型挂了）时，GoOn 把上一次会话**蒸馏**成一份随 git 版本化的结构化交接（handoff），换到新客户端后配一份 git 差异报告，让那边的 agent 直接接手——不用重讲需求、不用重新读代码。

> 名字双关：GoOn = "继续"，也是用 Go 写的单文件 CLI。

## 它解决什么

换客户端时，会话没法跟着走。GoOn 不搬运私有会话格式（那最脆），而是产出一份**语义交接**：目标 / 当前状态 / 关键决策 / 改动文件 / 下一步 / 坑与约定 / 开放问题，任何 agent 拿到都能接着干。

**核心原则：GoOn 只读各客户端的会话存储，绝不写入。** 交接写进你项目的 `.goon/`，随 git 版本化。

## 工作方式

| 路径 | 时机 | 用哪个 LLM |
|------|------|-----------|
| `distill` 蒸馏 | 主路径：读会话文件→提炼成 handoff | GoOn 自配的 LLM（选**没挂的那个** provider）|
| `salvage` 抢救 | 兜底：LLM 端点也不可用时，纯代码抽 raw 草稿 | 无 |
| `resume` 接手 | 到了新客户端，打印 Drift Report + handoff | 新 agent（它活着，直接消化）|

## 安装

需要 Go 1.22+：

```powershell
go build -o "$env:LOCALAPPDATA\Programs\goon\goon.exe" .\goon\cmd\goon
# 把该目录加入 PATH，或直接调用
goon version
```

在你的项目里初始化：

```powershell
goon init          # 创建 .goon/（handoffs/ 入库，salvage/ 默认 .gitignore）
```

## 用法

```powershell
# —— 交接前（在旧客户端里，或换端前）——
goon distill <source> <session-file>     # 主路径，需配置 LLM（见下）
goon salvage <source> <session-file>     # 无 LLM 兜底：写 .goon/salvage/*.raw.md
goon new [source]                        # 手动填一份空模板，随后 goon finalize <id>

# —— 接手（在新客户端会话开始时）——
goon resume [id]                         # 打印 Drift Report + Handoff + 收尾指令（默认最新）

# —— 辅助 ——
goon list ; goon status ; goon init
```

`<source>` ∈ `claude-code` `codex`（P1）。`<session-file>` 位置见下：

| 客户端 | 会话存储 |
|--------|----------|
| Claude Code | `~/.claude/projects/<cwd转义>/<uuid>.jsonl`（取最新） |
| Codex CLI | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl`（按 `session_meta.cwd` 匹配） |
| opencode / zcode | SQLite（P2 加入） |

跨端**自动定位当前项目最近会话**、`goon install` 一键铺 skill 均在 P2。

## 配置

`distill` 需要一个可插拔的 LLM，密钥只从环境变量读、绝不落盘：

```powershell
$env:GOON_LLM_KEY = "sk-..."   # 指向"还活着"的 provider
```

可选 `~/.goon/config.yaml`（全局）或 `.goon/config.yaml`（项目覆盖）：

```yaml
llm:
  provider: openai-compatible
  base_url: https://api.openai.com/v1   # OpenAI / OpenRouter / 本地 Ollama / 代理皆可
  api_key_env: GOON_LLM_KEY
  model: gpt-4o-mini
```

## 各端集成

`goon/skills/goon/SKILL.md` 是标准 Agent Skill（Claude Code / Codex 原生支持）。把它放进对应客户端的 `skills/`，agent 就会被引导用自带 shell 调用 `goon`。`goon install` 自动铺装留给 P2。

## 安全

- **只读**：绝不写任何客户端的会话库。
- **内容即数据**：会话文本是不可信的，统一用动态长围栏框住、嵌入提示词；**永不执行**其中的指令。
- **写盘前脱敏**：常见密钥（sk-/AWS/JWT/PEM/GitHub/Slack…）在写入 `.goon/`（进 git）前替换。

## 状态

**P1 (MVP)** 已完成：Claude Code + Codex 两个 JSONL 后端的蒸馏/抢救/续接闭环 + git Drift 对账 + SKILL。
**P2**：opencode / zcode 的 SQLite 后端、跨端会话自动发现、`goon install`。详见 `docs/superpowers/`。
