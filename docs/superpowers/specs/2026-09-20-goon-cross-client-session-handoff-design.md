# GoOn：跨 Agent 客户端会话交接 —— 设计文档

- **日期**：2026-09-20
- **状态**：已评审通过，待生成实现计划
- **代号**：GoOn（"继续"，也双关用 Go 语言实现 CLI）

---

## 1. 问题与目标

日常开发同时使用多个 agent 客户端（Claude Code、Codex/CLI、opencode、zcode）。**换客户端的常见原因是当前客户端的 LLM 出问题、无法继续使用**（本机实测证据：zcode 会话里就有 "Provider rejected the model request" 的失败记录）。此时新客户端不了解前情，要重讲需求、重新理解代码，非常费劲。

**目标**：做一个工具，让"会话"能跨客户端延续。经讨论确定走 **结构化交接（蒸馏）而非逐字回放**——因为不同客户端会话格式私有且随版本变化，逐字节迁移最脆；而"目标/已完成/关键决策/下一步/坑"这类语义快照，任何 agent 拿到都能接手。

**非目标（v1）**：不做逐字完整回放；不做实时双向同步；不做团队共享（个人、同机为主）；不做"无客户端在场"的独立浓缩（除非配置了 LLM 端点）。

## 2. 五条设计支柱

1. **只读、零耦合** —— GoOn 永不写任何客户端的会话库，只读。客户端升级/换实现都不影响它。
2. **确定性归 CLI，叙述性归 LLM/agent** —— 目录、id 链、git 快照、结构校验、密钥脱敏全是代码；只有"讲清楚这次干了啥"交给模型。
3. **蒸馏而非回放** —— 正式交接控制在几百行内，是对已裁剪抽取文本的提炼，不是录像，防稀释新 agent 上下文。
4. **会话内容一律当"不可信数据"** —— 只抽取、转义，绝不执行；写盘前脱敏（因为 handoff 要进 git）。
5. **处处降级** —— LLM 挂了→纯代码出 raw；格式没见过→通用提取器；没 git→跳过对账；客户端不能脚本→剪贴板一行提示词。

## 3. 本机实测事实（会话存储 & 打包格式）

> 以下为在本机 `C:\Users\Admin` 实际探查所得，作为 adapter 设计依据。

| 客户端 | 会话存储位置 | 格式 | 项目匹配字段 |
|--------|--------------|------|--------------|
| Claude Code | `~/.claude/projects/<slug>/<uuid>.jsonl` | JSONL（type: user/assistant/mode/…；带 `cwd`、`gitBranch`、`timestamp`、`uuid`/`parentUuid`、`isSidechain`） | `<slug>` 由路径转义（`D:\Attempt\ClaudeCode\Test` → `D--Attempt-ClaudeCode-Test`）+ 记录内 `cwd` |
| Codex CLI | `~/.codex/sessions/YYYY/MM/DD/rollout-<时间>-<uuid>.jsonl`，索引 `~/.codex/session_index.jsonl` | JSONL（`session_meta`、`response_item`(role+content[].text)、`turn_context`、`world_state`） | `session_meta.payload.cwd` / `turn_context.payload.cwd` |
| opencode | `~/.local/share/opencode/opencode.db` | **SQLite**（`session`→`message`→`part`+`todo`；`summary_additions/deletions/files`） | `session.directory` / `session.project_id` |
| zcode | `~/.zcode/cli/db/db.sqlite`（~244MB；rollout 仅 model-io 调试日志，单文件可达 23MB） | **SQLite**（与 opencode 同源 schema，另多 `model_usage` 表记录失败原因、`dwf_*`/`workflow_*` 多代理编排） | `session.directory` |

**关键结论**：四家 = **两族后端**（JSONL：Claude/Codex；SQLite：opencode/zcode 同源）。工作量减半。

**打包格式（实测）**：Claude 与 Codex 均原生支持 Agent Skills —— `<客户端>/skills/<name>/SKILL.md`，YAML frontmatter（`name`+`description`）+ markdown 正文。用户本机两家装的是同一批 skill，印证"跨客户端同步能力"是真实诉求。

## 4. 架构与数据流

```
        [只读] 各客户端会话存储
  ┌───────────────────────────────────────────────────────────┐
  │ Claude Code  ~/.claude/projects/<slug>/<uuid>.jsonl  (JSONL)│
  │ Codex CLI    ~/.codex/sessions/…/rollout-*.jsonl     (JSONL)│
  │ opencode     ~/.local/share/opencode/opencode.db     (SQLite)│
  │ zcode        ~/.zcode/cli/db/db.sqlite               (SQLite)│
  └───────────────┬───────────────────────┬───────────────────┘
        JSONL 后端│                       │SQLite 后端（同源 schema）
                  ▼                        ▼
            ┌──────────────────────────────────┐
            │  Adapters → 归一化 SessionModel   │  有序事件{text/tool/文件±行}
            │  + 项目元信息{cwd,branch,commit}  │  + todo + diffstat
            └───────────────────┬──────────────┘
                    ┌───────────┴───────────┐
                    ▼                       ▼
             (主路径) distill          (兜底) salvage
             调 GoOn 自配 LLM          纯代码裁剪，不碰 LLM
             蒸馏成 handoff 正文        出 raw 草稿到新 agent 消化
                    │                       │
                    └───────────┬───────────┘
                                ▼
                   ┌─────────────────────────┐    写盘前：结构校验 + 密钥脱敏
                   │  Store  .goon/           │
                   │  handoffs/ 链 + index.md │
                   └───────────┬─────────────┘
                               │  goon resume [id]
                               ▼
                   ┌─────────────────────────┐
                   │ Reconciler: git 对账     │ handoff.commit ↔ HEAD
                   │ → Drift Report           │
                   └───────────┬─────────────┘
                               ▼
                 Resume Prompt = Drift Report + handoff 正文 + 收尾指令
                               │
                               ▼   （各端 goon SKILL.md 只负责调 goon CLI）
                     新客户端 agent 接手（白嫖其模型消化）
```

**组件职责**：
- **Adapters**（只读）：定位本项目最近会话 → 抽取为归一化模型。两族各一实现 + 通用兜底提取器。
- **Normalizer / SessionModel**：统一的 `有序事件 + 项目元信息 + todo + diffstat` 内部结构，隔离各端格式差异。
- **Distiller**：由归一化模型构造裁剪提示词 → 调配置的 LLM 产出 handoff 正文；frontmatter/git/id 链等确定部分由 CLI 填。
- **Store（`.goon/`）**：链式存放、`index.md`、写盘前校验与脱敏。
- **Reconciler**：resume 时比对 handoff 记录的 commit 与当前 HEAD，产出 Drift Report。
- **Resume Composer**：拼装 `Drift Report + handoff + 收尾指令` 到 stdout。
- **Skill/Plugin 层**：各端 `goon` SKILL.md，仅引导 agent 用其 shell 调 `goon`。
- **Config**：全局 `~/.goon/config.yaml` + 项目 `.goon/config.yaml` 覆盖。

## 5. 交接文档结构

Markdown + YAML frontmatter，人可读/机器可解析/git 可 diff/agent 能直接吃：

```markdown
---
goon: 1
id: 2026-09-20T21-48-03-claude          # 时间戳(无冒号)+来源，唯一且可排序
source: claude-code                     # claude-code | codex | opencode | zcode | …
project: GoOn
git:
  branch: main
  commit: a1b2c3d
  dirty: true
  dirty_files:
    - src/auth/token.ts
supersedes: 2026-09-20T18-10-22-codex   # 指向上一份，形成链
---

## 目标
## 当前状态   （✅ 已完成 / 🔄 进行中 / ⛔ 卡点）
## 关键决策   （选 X 不选 Y 及原因，防新 agent 推翻重来）
## 改动文件
## 下一步     （可执行、带优先级）
## 坑与约定
## 开放问题
```

- **链式而非覆盖**：每次 distill/save 生成新文件，`supersedes` 串起；`index.md` 为方便人浏览的目录表。
- **蒸馏纪律**：正式 handoff 目标 ≤ ~200 行；salvage raw 可长但不进 git。

## 6. `.goon/` 目录布局

```
<project-root>/.goon/
├── config.yaml     # 可选覆盖：provider、salvage 路径、drift 详细度、salvage 保留天数
├── index.md        # CLI 自动生成的链索引（人不手写）
├── .gitignore      # GoOn 自动创建：排除 salvage/
├── handoffs/       # 正式交接（committed，随 git 版本化）
└── salvage/        # 抢救 raw 草稿（默认 gitignored，临时）
```

- 文件名 `YYYY-MM-DDTHH-MM-SS-<client-slug>.md`（跨 OS 安全、天然排序）。
- 版本化：handoffs 入库；salvage 默认不入库（原始提取可达几百 KB～MB）。
- `config.yaml` 全部有默认值，文件可完全不存在：
```yaml
handoff_dir: .goon/handoffs
salvage_dir: .goon/salvage
salvage_keep_days: 7
drift_verbosity: summary   # summary | full | off
```

## 7. Git 对账机制

解决"交接写的是过去时、新 agent 面对的是现在时"的断裂。

- **Save 时记录**：`git rev-parse HEAD`→commit；`--abbrev-ref HEAD`→branch；`git status --porcelain`→dirty/dirty_files。写入 frontmatter。
- **Resume 时比对**：读 handoff 的 `git.commit` → `git cat-file -t <commit>`（探测是否被 rebase/squash 掉）→ `git diff --stat/--name-status <commit> HEAD` → 当前 branch → `git status --porcelain`。
- **Drift Report 注入**（置于 handoff 正文之前）：分支变化、此后新提交、变更文件清单、当前工作区状态，末尾附一句"handoff 描述的是 N 个 commit 前的状态，请先消化差异再接手"。

**边界处理**：

| 情况 | 处理 |
|------|------|
| commit 不存在（rebase/squash） | 降级为"找不到基准 commit"，用 `git log --since=<timestamp>` 兜底 |
| 跨机器 / commit 未 push | 同上；save 时若 dirty 且有未推 commit → 提示先 push/stash |
| 无 drift（同分支同状态） | Drift Report 缩为一行"✓ 代码状态与交接一致" |
| 非 git 仓库 | 整段跳过，frontmatter git 字段为 null，GoOn 仍正常 |

## 8. CLI 命令面

| 命令 | 作用 | 用谁的模型 |
|------|------|-----------|
| `goon init` | 初始化 `.goon/`、写 provider 配置 | 无 |
| **`goon distill`** | **主路径**：定位最近会话→抽取→调 LLM 蒸馏→落 `.goon/handoffs/<id>.md`。`--client` 指定源、`--pick` 列候选 | GoOn 自配 LLM |
| `goon new` / `goon save` | 可预见切换时，健康 agent 主动写交接的脚手架 | 当前 agent |
| `goon finalize <id>` | 校验正文结构 + 刷新 `index.md` | 无 |
| `goon resume [id]` | 打印 Drift Report + handoff + 收尾指令，喂新 agent | 新 agent（白嫖）|
| `goon salvage [--pick]` | 只抽取成 raw、不蒸馏 | 无 |
| `goon list` / `status` / `link <client> <path>` | 看链 / 看当前态 / 记会话目录 | 无 |
| `goon export --clipboard` | 无脚本能力客户端的降级：一行提示词进剪贴板 | 无 |
| `goon install <client>` | 一键把 goon SKILL.md 铺到该端配置目录 | 无 |

命令壳不依赖各端私有注入语法，一律**引导 agent 用它自己的 shell 调 `goon …`**（四端都能执行命令，移植性最好）。

## 9. LLM 蒸馏配置

**前提修正**：因换端常因"当前 agent 的 LLM 挂了"，主路径 distill **不依赖那个坏掉的 agent**，故 GoOn 需自带可配置 LLM。

- **基线**：OpenAI 兼容（`base_url` + `api_key_env` + `model`），一个配置覆盖 OpenAI / OpenRouter / 本地 Ollama·vLLM / 各类代理；Anthropic 原生作可选 override。
- **默认选"和挂掉那个不同"的 provider**。
- **安全**：key 只从环境变量读，绝不写盘、不入 git。
- **成本**：蒸馏是对裁剪文本调一次；仅超大会话才分块 map-reduce。

```yaml
# ~/.goon/config.yaml（全局），.goon/config.yaml 项目级覆盖
llm:
  provider: openai-compatible
  base_url: https://api.openai.com/v1
  api_key_env: GOON_LLM_KEY
  model: gpt-4o-mini
```

## 10. Skill / Plugin 打包分发

一个 `goon` 包内含：① 各端命令定义（`save`/`resume`/`distill` 的提示词模板，引导 agent 调 `goon` CLI）；② 一份 `goon` SKILL.md（何时 distill、何时 resume）。`goon install <client>` 铺到各端 `skills/`；**Qoder 版做成自用 skill/plugin（本环境即可 dogfood）**。

## 11. 安全与隐私红线

- **只读**：绝不写任何客户端会话库。
- **内容即数据**：会话文件含不可信文本（实测 Codex `thread_name` 有 `<|endoftext|>` 注入残渣）。抽取后转义/引用嵌入蒸馏与 resume 提示词，**永不执行**。
- **写盘前脱敏**：会话常含密钥/token/工具输出；handoff 要进 git，故写入前对 handoff 与 salvage raw 做正则 + 熵值扫描脱敏并告警。
- **展示消毒**：`index.md`/`list` 里对会话标题等做转义，防间接注入与终端污染。

## 12. 分阶段实现

| 阶段 | 内容 |
|------|------|
| **P1 (MVP)** | `goon init` + 交接 schema + **JSONL 后端蒸馏**（Claude Code + Codex）+ git 对账 + `goon resume` + Claude/Codex 的 goon SKILL.md → 跑通"挂了就蒸馏、换端就续接"主闭环 |
| **P2** | **SQLite 后端**（opencode + zcode，解 `message.data`/`part.data`，白嫖 `todo` 与 `summary_*`）+ 主动 `goon new/save` |
| **P3** | provider 配置 UX、`goon install <client>` 一键铺 skill、剪贴板降级、`--pick` 选会话、超大会话分块蒸馏、`index` 回滚 |

## 13. 成功标准

1. **冷启动接手**：A 端干 N 件真活→切 B 端→敲一条命令→新 agent 准确复述目标/下一步/坑，无需重讲；期间改过文件能主动 flag drift。
2. **体积/成本**：正式 handoff ≤ ~200 行；蒸馏 20MB zcode 会话不炸上下文（流式抽取+裁剪）。
3. **零耦合**：GoOn 全程不写任何客户端会话库。
4. **降级**：LLM 端点也挂时，`goon salvage` 仍纯代码出 raw，`resume` 交新 agent 消化。

## 14. 测试策略

- **黄金 fixture**：四家各存一份脱敏真实会话快照（Claude jsonl / Codex rollout / opencode·zcode db 片段）。
- **单元**：`解析器 → 归一化 SessionModel → 蒸馏提示词构造 → handoff 校验`。
- **Drift**：临时 git 仓库（提交→改→断言变更文件清单/分支变化）。
- **鲁棒性**：未知/更新格式优雅降级到通用提取器，绝不崩。
- **安全回归**：脱敏命中、注入转义各有专门用例。
- **E2E smoke**：对 fixture 跑 `distill → resume`。

## 15. 技术栈与依赖

- **语言：Go**（单静态 exe、Windows 零运行时依赖、流式解析大 JSONL 省内存、跨平台）。
- SQLite 驱动：`modernc.org/sqlite`（纯 Go、免 CGO）。
- frontmatter/YAML：`gopkg.in/yaml.v3`。
- git 对账：直接 shell 调 `git`（复用用户已有 git；解析其文本输出）。

## 16. 开放问题 / 后续

- opencode/zcode 的 `message.data`/`part.data` 具体 JSON 结构待 P2 打开确认（本文档已定归一化模型，不阻塞 P1）。
- 是否未来加 MCP façade（曾列为方案 C，作为 P1/P2 之上的一层，暂不做）。
- zcode 的 `dwf_*`/`workflow_*` 多代理图是否要在交接里表达（P2+ 评估）。

## 附录 A：各端会话记录字段速查

- **Claude Code**：`type`(user/assistant/mode/permission-mode) · `message.{role,content}` · `uuid`/`parentUuid`（重建主线）· `isSidechain`（子代理）· `timestamp` · `cwd` · `gitBranch` · `sessionId`。
- **Codex**：`session_meta.payload.{cwd,session_id,cli_version,model_provider}` · `response_item.payload.{type,message/…,role,content[].text}` · `turn_context.payload.{cwd,workspace_roots}` · `world_state`。
- **opencode / zcode（同源）**：`session{id,project_id,directory,path,title,cost,tokens_*,summary_additions/deletions/files}` · `message{id,session_id,data}` · `part{id,message_id,data}` · `todo{session_id,content,status,priority}` · zcode 另有 `model_usage{provider_id,model_id,status,error_type,context_exceeded}`。
