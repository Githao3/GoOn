# GoOn P2 (SQLite 后端 + 跨端发现) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]` checkboxes.

**Goal:** 让 opencode / zcode 两个 SQLite 客户端进入 GoOn 的 distill/salvage 主路径，并实现跨全部 4 个客户端的"会话自动发现"，去掉手动传会话路径的负担。

**Architecture:** 复用 P1 的 `extract.SessionModel` 契约。SQLite adapter 以只读方式打开各库，用统一查询（`message` 提供 role、`part`(type=text) 提供正文、`part`(type=tool) 提供文件改动、`todo` 提供待办）组装模型。`discover` 包按当前项目 cwd 匹配各客户端的最近会话，供 CLI 在省略参数时自动选取。

**Tech Stack:** Go 1.22+；`modernc.org/sqlite`（纯 Go、免 CGO、只读 URI）；stdlib。

**依赖 P1**：`goon/internal/extract`、`handoff`、`store`、`gitrepo`、`llm`、`distill`、`resume`、`cli`、`redact` 已就绪。设计见 `docs/superpowers/specs/2026-09-20-goon-cross-client-session-handoff-design.md`。

---

## 实测 schema（本机只读核实，权威）

**opencode.db**（`~/.local/share/opencode/opencode.db`）
- `session(id, directory, path, title, time_updated, …)`；`directory` 为**正斜杠**形如 `C:/Users/Admin/Desktop/tmp`；`time_updated` 为 epoch **毫秒**。
- `message(id, session_id, time_created, data)`；`data` = `{"role":"user|assistant", "time":{"created":…}, …}`，**正文不在 message**。
- `part(id, message_id, session_id, time_created, data)`；`data` = `{"type":"text","text":"…"}`（还有 tool 等类型）。
- `todo(session_id, content, status, priority, position, …)`。

**zcode db.sqlite**（`~/.zcode/cli/db/db.sqlite`）
- `session(id, directory, path, title, time_updated, …)`；`directory` 为**反斜杠**形如 `D:\Attempt\Zcode\local-api`。
- `message(id, session_id, time_created, data, sequence)`；`data.role` 同上。
- `part(id, message_id, session_id, time_created, data, sequence)`；`data.type` ∈ text / tool / step-start / step-finish / reasoning / timeline。text→`data.text`；tool→`data.tool`(名)+`data.state.input.file_path`。
- `session_entry`：仅 `runtime/model_selection`、`v4/fork_start_failure` 等**元数据，非转录，忽略**。
- `todo`：同上。

**统一提取查询**（两家通用，均按 `part.time_created` 排序）：
```sql
-- 正文事件（role + text）
SELECT json_extract(m.data,'$.role') role, json_extract(p.data,'$.text') text
FROM part p JOIN message m ON p.message_id = m.id
WHERE p.session_id = ? AND json_extract(p.data,'$.type') = 'text' AND text IS NOT NULL AND text <> ''
ORDER BY p.time_created;
-- 文件改动事件（tool + 文件路径）
SELECT json_extract(p.data,'$.tool') tool, json_extract(p.data,'$.state','$.input','$.file_path') fp
FROM part p WHERE p.session_id = ? AND json_extract(p.data,'$.type')='tool' AND fp IS NOT NULL
ORDER BY p.time_created;
-- 会话元信息
SELECT title, directory FROM session WHERE id = ?;
-- 待办
SELECT content FROM todo WHERE session_id = ? ORDER BY position;
```
（`json_extract` 路径写法以驱动为准，必要时用 `->`/`->>`；modernc.org/sqlite 内置 JSON1。）

**目录归一（匹配 cwd）**：`normalizeDir(s) = strings.Trim(strings.ToLower(filepath.ToSlash(s)),"/")`；比较时两侧都归一（opencode 已是斜杠、zcode 反斜杠经 ToSlash 转换）。

**安全**：SQLite 只读打开（`file:...?mode=ro`）；正文经 `distill`/`redact` 处理；**fixtures 一律合成**，绝不提交真实会话内容。

---

## File Structure
```
goon/internal/sqliteopen/sqliteopen.go   # 只读打开 sqlite *sql.DB 的小封装（新）
goon/internal/extract/model.go           # 增加 SessionSummary{ID,Title,Modified}（改）
goon/internal/extract/sqlite.go          # 共享 loader：loadSQLite(dbPath, sessionID, source) → SessionModel（新）
goon/internal/extract/opencode.go        # ParseOpenCodeDB + ListOpenCodeSessions（新）
goon/internal/extract/zcode.go           # ParseZcodeDB + ListZcodeSessions（新）
goon/internal/extract/*_test.go          # 用临时 sqlite 造 fixture 单测（新）
goon/internal/discover/discover.go       # 跨端发现：Recent(cwd) / All(cwd, days)（新）
goon/internal/discover/discover_test.go  # 临时目录 + 临时库/文件 fixture（新）
goon/internal/config/config.go           # 增加各端 sqlite/会话根路径的默认与覆盖（改）
goon/internal/cli/cli.go                 # parseSession 扩展 sqlite；自动选会话；--all（改）
```

---

## Task 1: sqlite 只读打开 + 模型扩展 + 依赖

**Files:** Create `goon/internal/sqliteopen/sqliteopen.go`(+test); Modify `goon/internal/extract/model.go`, `goon/go.mod`.

- [ ] **Step 1** 加依赖：在 `goon/` 跑 `go get modernc.org/sqlite@latest`（若离线失败即 BLOCKED 上报，不要卡住）。
- [ ] **Step 2** 写 `sqliteopen.Open(path string) (*sql.DB, error)`：用 `sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro&_pragma=busy_timeout(2000)")`，`db.Ping()` 校验。失败返回 error。测试（临时建一个内存/文件库，插入一行，Open 后能读到；对不存在文件 Open 不 panic）。
- [ ] **Step 3** `extract/model.go` 增加：
```go
// SessionSummary 用于发现列表：一个可比对的会话条目。
type SessionSummary struct {
	ID       string
	Title    string
	Modified time.Time // 归一到 UTC
}
```
- [ ] **Step 4** gofmt -w . ; go build ./... ; go test ./internal/sqliteopen/ ; 全绿。
- [ ] **Step 5** commit `feat(p2): sqlite readonly opener + SessionSummary + modernc dep`。

## Task 2: 共享 SQLite loader + opencode adapter

**Files:** Create `goon/internal/extract/sqlite.go`, `opencode.go`, `opencode_test.go`.

- [ ] **Step 1** 写失败测试 `opencode_test.go`：测试内用 `modernc.org/sqlite` 现造一个临时 opencode 形状库（建 `session`/`message`/`part`/`todo` 表，插 2 条 message(user/assistant)+对应 text part + 1 个 tool part(带 file_path) + 1 条 todo；`session.directory` 用正斜杠）。断言 `ParseOpenCodeDB` 得到 `Source=="opencode"`、按顺序的 user/assistant 文本事件、含一个 tool FileEdit(Path 命中)、Todos 含该条。`ListOpenCodeSessions(dbPath, cwd)` 用归一后的 cwd 命中该 session、返回按 time_updated 新→旧。
- [ ] **Step 2** 实现 `sqlite.go`：`func loadSQLite(dbPath, sessionID, source string) (SessionModel, error)` 用上面统一查询填充 Events(文本)、tool→FileEdit 事件、Todos、Project.CWD(session.directory)、Title。`func listSQLite(dbPath, source, cwd, dirCol string) ([]SessionSummary, error)`：`SELECT id,title,time_updated FROM session`，Go 侧按 `normalizeDir(directory)==normalizeDir(cwd)` 过滤并按 Modified 降序（epoch ms → time）。
- [ ] **Step 3** `opencode.go`：`ParseOpenCodeDB(dbPath, sessionID string) (SessionModel, error)`（source="opencode"）；`ListOpenCodeSessions(dbPath, cwd string) ([]SessionSummary, error)`。
- [ ] **Step 4** 运行测试通过；`gofmt -l .` 空；`go build ./...`；commit `feat(p2): shared sqlite loader + opencode adapter`。

## Task 3: zcode adapter

**Files:** Create `goon/internal/extract/zcode.go`, `zcode_test.go`.

- [ ] **Step 1** 失败测试：造临时 zcode 形状库（`session.directory` 用**反斜杠**；`part` 含 text 与 tool；额外塞一条 `session_entry`(type='runtime/model_selection') 以证明被忽略；message/part 带 `sequence`）。断言 `ParseZcodeDB` 正确取文本/工具/待办且忽略 session_entry；`ListZcodeSessions(dbPath, cwd)` 归一后命中反斜杠目录。
- [ ] **Step 2** `zcode.go`：`ParseZcodeDB`（source="zcode"，复用 `loadSQLite`）；`ListZcodeSessions`（复用 `listSQLite`）。若 zcode 需按 `sequence` 排序，可在 loadSQLite 里 `ORDER BY time_created` 已足够（等价），否则加参数化。
- [ ] **Step 3** 测试通过 + 门禁；commit `feat(p2): zcode sqlite adapter`。

## Task 4: 跨端发现 discover

**Files:** Create `goon/internal/discover/discover.go`(+test); Modify `config.go`.

- [ ] **Step 1** config 增加各端路径（均有默认、可被覆盖）：`sqlite_opencode`, `sqlite_zcode`, `claude_projects`, `codex_sessions` 目录；`normalizeDir` 放到 discover 或 extract 共享。测试：覆盖默认生效。
- [ ] **Step 2** 定义
```go
type Kind int // KindJSONL, KindSQLite
type Candidate struct {
	Client   string // claude-code|codex|opencode|zcode
	Title    string
	Modified time.Time
	Kind     Kind
	Ref      string // JSONL: 文件绝对路径；SQLite: "dbPath#sessionID"
}
func Recent(cwd string, cfg config.Config) ([]Candidate, error) // 全 4 端合并、按 Modified 降序、跳过不可用来源
```
- [ ] **Step 3** 各端收集：claude（`claude_projects/<slug(cwd)>/*.jsonl`，mtime，标题取首条 user 文本摘要）、codex（`codex_sessions/**` 匹配 meta cwd）、opencode/zcode（`List*Sessions(dbPath, cwd)` → Ref=db#id）。**任一来源不存在/出错只跳过、不整体失败**（优雅降级，§5）。
- [ ] **Step 4** 测试：临时目录里造 1 个 claude jsonl + 1 个 opencode sqlite，指向同一 cwd，`Recent` 返回 2 个候选且按时间降序、Ref 形态正确；缺目录时返回可用子集不报错。
- [ ] **Step 5** commit `feat(p2): cross-client session discovery`。

## Task 5: CLI 接线 + 文档

**Files:** Modify `cli.go`, `cli_test.go`, `main.go`; Modify `goon/skills/goon/SKILL.md`, `README.md`.

- [ ] **Step 1** 扩展 `App`：`resolveSession(source, arg string)`：
    - source ∈ claude-code/codex：arg 为 jsonl 路径 → 现 P1 解析。
    - source ∈ opencode/zcode：arg 为 sessionID → `Parse{Opencode,Zcode}DB(defaultDbFor(source), sessionID)`。
    - source 为 `recent`/空 或 `--recent`：调 `discover.Recent(cwd,cfg)` 取最新 1 个（或 `--all N` 取 N 个合并）→ 按 Candidate.Kind/Ref 分派解析。
  `defaultDbFor` 从 config 读路径。`slug(cwd)` 复用 claude 规则（`[:\\/]`→`-`）。
- [ ] **Step 2** `main.go` dispatch：`distill/salvage [source] [arg]`，新增可选 `--recent`/`--all <n>`；缺省无 arg 时走 recent。保持旧的位置参数形态向后兼容（`distill claude-code <file>` 仍可用）。
- [ ] **Step 3** cli_test 增：用临时 opencode 库 + 一个合成 session，`App.SalvageBySource("opencode", sessionID)`（或经 discover）产出 raw 落盘、不 panic；`--recent` 路径至少覆盖"发现到则用之、发现不到则明确报错"。
- [ ] **Step 4** 更新 SKILL.md（源清单含 opencode/zcode、`goon distill`/`salvage` 可省参数走最近会话）与 README（P2 已支持 4 端）。
- [ ] **Step 5** 门禁全绿；commit `feat(p2): wire sqlite + discovery into CLI`。

---

## Self-Review
- 覆盖：SQLite 两端(Task2/3)、发现(Task4)、CLI(Task5)、config 路径(Task4)、文档(Task5)。SessionModel 契约复用、不破坏 P1。
- 只读、优雅降级、合成 fixture、脱敏：均有对应步骤；`normalizeDir` 与 slug 为关键正确性点，均有测试。
- 类型一致：`SessionSummary`/`Candidate`/`loadSQLite`/`listSQLite` 跨任务签名一致；`Parse{Opencode,Zcode}DB` 与既有 `ParseClaudeFile` 同为返回 `SessionModel`。

## 交接
执行方式同 P1（subagent-driven，逐任务 TDD + 评审）。完成后合并回 master 并推送。
