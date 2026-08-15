# mu fleet 模块计划（远程批量执行与部署）

## 一、不同场景的完整工作流

### 场景 A：临时快速操作（单条命令，无需文件）

```bash
mu fleet run --hosts h1,h2 --command "systemctl restart nginx && systemctl status nginx"
```

完整流程：

1. 控制器（MacBook）读取 `~/.config/mu/fleet-config.json`，获得 dispatcher URL 与 token。
2. 控制器以一次 **multipart** 请求提交 job：`POST /api/fleet/jobs`，携带 JSON 元数据（`command` 文本、`targets: ["h1","h2"]`，无文件）。
3. dispatcher 创建 job，并为每个目标主机生成一条 `run`（状态 `pending`）入队；返回 job id。
4. h1、h2 上的 agent（`mu fleet agent`）各自轮询 `POST /api/fleet/agents/<name>/poll`，领取属于自己的 `pending` run。
5. agent 复用 `internal/core/runner`（headless plain 模式）执行 command，边执行边分块回传输出：`POST /api/fleet/agents/<name>/runs/<id>/output`。
6. 执行结束 agent 上报结果：`POST .../complete`（成功/失败 + 退出码 + 耗时）。
7. 控制器带 `--watch` 时，以约 1s 间隔轮询 `GET /api/fleet/jobs/<id>`，实时刷新各主机输出；无 `--watch` 则一次性返回当前状态。
8. 结束打印每主机汇总（✓/✗ + 耗时），任一失败返回非零退出码。

### 场景 B：带归档的部署（文件传输 + 自动解压）

```bash
./build.sh                                    # 本地构建产出 dist/app.tar.gz
mu fleet run --hosts h1,h2 --file deploy.yaml \
  --files dist/app.tar.gz --var version=1.2
```

完整流程：

1. 控制器把本地 `dist/app.tar.gz` 作为 multipart 文件段随 job 一次上传到 dispatcher；dispatcher 计算并记录 sha256，落盘到 `~/.cache/mu/fleet/jobs/<id>/files/`。
2. agent 轮询领取 run 后，先下载任务文件：`GET /api/fleet/agents/<name>/runs/<id>/files/app.tar.gz`，校验 sha256。
3. agent 建立任务工作目录，把文件放入并**自动解压 `.tar.gz`**（`.zip` 同理）。
4. agent 以该工作目录为 recipe 的 workdir 执行 `deploy.yaml`，相对路径天然落在任务目录内：
   ```yaml
   tasks:
     install:
       command: cp app /usr/local/bin/app
     restart:
       depends: [install]
       command: systemctl restart app
   ```
5. 输出分块回传、结果上报，与场景 A 一致；任务目录用完即弃，不污染主机。
6. 汇总打印各主机部署结果。

### 场景 C：批量 + 多变量 + 部分失败

```bash
mu fleet run --hosts prod-web-1,prod-web-2,prod-db-1 --file rollout.yaml \
  --var version=2.0 --var registry=ghcr.io/me/app --watch
```

完整流程：

1. 三台主机各自并行领取 run（互不阻塞）。
2. `{{.version}}`、`{{.registry}}` 等模板变量在**每台 agent** 上解析（复用 `core/runner` 的 recipe 模板能力）。
3. 某台主机任务失败时，**该主机**按 recipe 的失败策略停止（默认即停，`continue_on_error`/`--keep-going` 例外），其余主机继续执行。
4. dispatcher 记录每台 run 的状态；控制器 `--watch` 下汇总表按主机列出，标出部分失败。

### 场景 D：离线主机补跑（poll 模型的核心价值）

1. 提交 job 时 `prod-db-1` 掉线 → dispatcher 仍为它保留 `pending` run（不丢失）。
2. `prod-db-1` 上线后，agent 轮询领取该 run → 下载文件 → 执行 → 上报。
3. 之后控制器执行 `mu fleet status <job-id>`，仍能看到该主机的完整结果与历史输出（BoltDB 持久化）。

### 场景 E：状态与运维

```bash
mu fleet hosts            # 在线 agent 列表（心跳超时自动判离线）
mu fleet status <job-id>  # 各主机任务状态与输出（含历史）
mu fleet jobs             # 最近任务列表
```

## 二、背景与动机

`mu run --file <recipe.yaml>` 把单机任务编排做成了「本地 mini-CI」，但部署通常需要**批量在多个主机上执行**。本模块提供一个类 ansible 的能力：由一台机器（如 MacBook）发起远程批量执行与部署。

设计取舍：

- **模型 A（dispatcher + agent poll）**，而非 ansible 的 push 直连：
  - agent 无需开放入站端口（适合 homelab / NAT）；
  - 发出的任务**不依赖发起者在线**（发完可断开，agent 继续跑，回来查结果）；
  - 临时离线的主机上线后能补跑；
  - 单点鉴权。
- 大量复用已有资产：`core/runner`（recipe 执行引擎）、`core/wol/agent.go`（注册 + 退避重试模式）、`core/store`（BoltDB）、模块配置约定。

## 三、总体架构

```
MacBook (controller) ──HTTP──▶ mu fleet serve (dispatcher)
                                    │  └─ BoltDB: agents / jobs / runs / run_output
                                    │  └─ 文件目录: ~/.cache/mu/fleet/jobs/<id>/files/
                    ┌───────────────┼───────────────┐
                 h1 agent        h2 agent        h3 agent
              (mu fleet agent)  (poll 领取 + 本地执行)
```

数据流向：

- 控制器 → dispatcher：提交 job（multipart，含文件）、查询状态/输出。
- dispatcher → agent：poll 返回待执行 run（含 recipe/command、vars、文件清单）。
- agent → dispatcher：注册、输出分块、完成结果、心跳（poll 即心跳）。
- agent → dispatcher 文件下载：领取 run 后按需拉取任务文件。

## 四、目录结构

```
internal/core/fleet/
  ├── types.go        Agent / Job / JobRun / 状态常量
  ├── store.go        BoltDB 持久化（agents / jobs / runs / run_output）
  ├── dispatcher.go   dispatcher HTTP handlers + RegisterHandlers
  ├── auth.go         X-Auth-Token 中间件
  ├── agent.go        agent 循环（register → poll → 执行 → 上报）
  ├── client.go       controller / agent 的 HTTP 客户端
  ├── transfer.go     文件上传下载 + 归档解压（.tar.gz / .zip）
  └── *_test.go       单元 + 集成测试

internal/fleet/       CLI wrapper（仅解析 + 调用 core）
  ├── options.go      子命令与选项定义
  ├── command.go      Run() 实现
  └── config.go       fleet-config.json 加载/保存

cmd/mu/myutilities.go  注册 Fleet 顶级命令
```

核心逻辑全部在 `internal/core/fleet`，`internal/fleet` 只做 CLI 包装（遵循项目约定）。

## 五、CLI 设计

```
mu fleet serve [--port 8890]
mu fleet agent [--server URL] [--hostname NAME] [--groups prod] [--poll-interval 5s]
mu fleet run --hosts h1,h2 [--file x.yaml | --command "cmd"] [--var k=v] [--files path...] [--watch]
mu fleet hosts
mu fleet status <job-id> [--watch]
mu fleet jobs [--limit 20]
```

规则：

- `run` 的 `--file` 与 `--command` 互斥（必选其一）。
- `--hosts` 必填，逗号分隔或重复。
- `--files` 可重复；`--var` 可重复。
- 配置：`~/.config/mu/fleet-config.json`，字段：`server`、`token`、`hostname`、`groups`、`poll_interval`、`port`、`db_path`、`data_dir`；支持 `--config-dir` 覆盖目录；含密钥 → 文件权限 `0600`。

## 六、数据模型与存储

```
Agent   { hostname, groups, lastSeen }
Job     { id, recipe 文本 | command, vars, targets, files[] {name,size,sha256}, createdAt }
JobRun  { jobID+hostname, state(pending/running/succeeded/failed), startedAt, finishedAt, error }
```

BoltDB buckets：

| Bucket | Key → Value |
|---|---|
| `agents` | hostname → Agent 序列化 |
| `jobs` | job id → Job 序列化 |
| `runs` | `<jobID>/<hostname>` → JobRun 元数据 |
| `run_output` | `<jobID>/<hostname>` → 累积输出 |

- **输出持久化**：agent 分块回传（约每 100ms 或每 N 行一批），dispatcher 每批 append 到 `run_output`，避免逐行写库。每 run 输出默认截断 1MB（可配置），超限保留尾部并标记 `[truncated]`，防止无界膨胀。
- dispatcher 重启后：agents/jobs/runs/run_output 全部恢复，任务不丢、历史输出可查。
- job 文件落盘 `~/.cache/mu/fleet/jobs/<id>/files/`，按任务隔离。
- 离线 agent 的 pending run **保持等待**（供补跑），agent 的 Online 状态由 lastSeen 动态推导。

## 七、Dispatcher API（`X-Auth-Token` 鉴权）

| Method | Path | 用途 |
|---|---|---|
| POST | `/api/fleet/jobs` | controller 提交 job（multipart：`job` JSON 字段 + 若干 `file` 段） |
| GET | `/api/fleet/jobs` | 最近任务列表 |
| GET | `/api/fleet/jobs/{id}` | 查 job 状态与各主机输出 |
| POST | `/api/fleet/register` | agent 注册（hostname/groups） |
| POST | `/api/fleet/agents/{name}/poll` | agent 领取 pending run + 心跳 |
| POST | `/api/fleet/agents/{name}/runs/{id}/output` | 上报输出块 |
| POST | `/api/fleet/agents/{name}/runs/{id}/complete` | 上报完成结果 |
| GET | `/api/fleet/agents/{name}/runs/{id}/files/{name}` | agent 下载任务文件 |
| GET | `/api/fleet/agents` | 在线 agent 列表 |

job 状态派生规则：全部 succeeded → succeeded；任一 failed → failed；否则 pending/running。

## 八、Agent 生命周期

1. **注册**：`POST /api/fleet/register`，带 hostname/groups；失败按退避重试（复用 `core/wol` 的 `agentMaxRetries` 模式）。
2. **循环**：
   - `poll`（即心跳，更新 dispatcher 侧 lastSeen）：有 `pending` run 则领取；无则 sleep `poll_interval`。
   - 领取后：下载任务文件 → 校验 sha256 → 解压归档 → 在任务工作目录执行（recipe 用 `RunRecipe`，command 用 `Run`，均走 core/runner plain 模式）→ 边跑边 POST 输出块 → 结束后 POST complete。
3. 断线重连与心跳超时：dispatcher 按 lastSeen 超时判离线（默认 3×poll_interval）。

## 九、文件传输与归档解压

- 控制器 `--files` 一次 multipart 上传；dispatcher 记录 sha256、落盘。
- agent 下载后校验 sha256，放入任务工作目录。
- `.tar.gz` / `.zip` 自动解压（类似 ansible `unarchive`）；其余文件原样放置。
- recipe 以任务工作目录为默认 workdir；任务目录用完即弃。

## 十、鉴权与安全

- 共享 token：所有请求带 `X-Auth-Token`（沿用 WOL 模式）；`fleet-config.json` 权限 `0600`。
- **风险说明**：agent 会执行控制器提交的任意命令 = RCE 面，仅适用于受信局域网；后续可升级 mTLS / 按 agent 独立凭据。

## 十一、core/runner 小改动

- plain 输出目前直接写 `os.Stdout`。给 `CommandRunner` 增加可注入的 `io.Writer`（默认 os.Stdout），供 agent 把输出 tee 到分块回传通道；TTY 显示路径不受影响。
- recipe 解析增加 `ParseRecipe([]byte)`，供 agent 直接解析下发的 recipe 文本（免临时文件）。
- `RunRecipe` 已返回 `[]TaskResult`，供 agent 汇总。

## 十二、测试计划

- 单元：`store`（CRUD/输出 append/截断/Online 推导）、dispatcher handlers（`httptest`，含鉴权与文件上传下载）、`transfer`（归档解压）、agent 循环（fake dispatcher）、runner OutputWriter。
- 集成：进程内起 dispatcher + agent + controller client，跑一个小 recipe 与一个带 `--files` 的任务，断言各主机结果、输出落库、文件落地与解压。
- 保持现有 264 个测试基线绿。

## 十三、文档更新

- `README.md` 新增准确的 `### fleet` 段（命令用法 + 工作流示例）。
- `AGENTS.md` 新增 core/fleet 与 command/fleet 条目。
- `CODEBASE.md` 目录树补充。

## 十四、分阶段

- **Phase 1（本计划）**：上述全部——serve/agent/run/hosts/status/jobs + 文件传输与自动解压 + token 鉴权 + BoltDB（含输出持久化）+ 轮询式输出（`--watch`）。
- **Phase 2**：SSE/WebSocket 实时输出流、inventory 分组别名（`--hosts 组名`）、gateway 任务/主机 Web 页、mTLS。
