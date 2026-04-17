# 种子池自动发现实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 为当前系统补齐“B 站候选博主自动发现 -> 人工审核 -> 转正进入正式追踪”的完整闭环，且不影响现有抓取、下载、检查与清理主链路。

**架构：** 后端新增 `candidate_*` 持久化模型、`internal/discovery` 领域服务和 `discover` 任务类型；候选来源先接入关键词发现，再在后续任务中补充基于已追踪博主的一跳关系扩散。Web 端新增候选池页面，直接联动候选查询、审核动作与手动触发发现接口。

**技术栈：** Go、MySQL、net/http、React、Vite、单元测试、前端脚本测试、现有 SSE/HTTP 驱动驾驶舱

---

## 文件边界

### 数据与仓储

- 创建：`migrations/00003_candidate_discovery.sql`
  - 创建候选池 3 张表，并扩展 `jobs.type` 支持 `discover`
- 修改：`internal/repo/models.go`
  - 增加候选模型、来源模型、评分明细模型、查询过滤器
- 修改：`internal/repo/repo.go`
  - 增加候选仓储接口定义，并把候选仓储挂到 `Repositories`
- 修改：`internal/repo/mysql/repo.go`
  - 暴露 `Candidates()` 仓储构造函数
- 创建：`internal/repo/mysql/candidate_repo.go`
  - MySQL 候选仓储实现
- 创建：`internal/repo/mysql/candidate_repo_test.go`
  - 仓储 SQL、状态流转、列表筛选、幂等更新测试

### 配置与任务调度

- 修改：`internal/config/config.go`
  - 增加 `DiscoveryConfig`、默认值、校验与解析
- 修改：`internal/config/config_test.go`
  - 覆盖 discovery 配置解析、默认值与非法配置
- 修改：`configs/config.example.yaml`
  - 增加 discovery 配置模板
- 修改：`internal/jobs/types.go`
  - 增加 `TypeDiscover`
- 修改：`internal/jobs/service.go`
  - 增加 `EnqueueDiscover` 与“按博主定向拉取”入队能力
- 修改：`internal/jobs/service_test.go`
  - 覆盖 discover 入队、定向拉取入队、幂等与事件发布
- 修改：`internal/scheduler/scheduler.go`
  - 增加 discover 定时触发逻辑
- 修改：`internal/scheduler/scheduler_test.go`
  - 覆盖 discovery enabled/disabled 与触发行为

### 发现领域服务

- 创建：`internal/discovery/types.go`
  - 领域输入输出结构、评分因子键、状态常量
- 创建：`internal/discovery/scorer.go`
  - 候选评分器
- 创建：`internal/discovery/scorer_test.go`
  - 覆盖关键词分、活跃度分、体量修正、人工反馈惩罚、分数上限
- 创建：`internal/discovery/service.go`
  - 候选聚合服务：列表、详情、审核动作、批准转正、触发发现
- 创建：`internal/discovery/service_test.go`
  - 覆盖候选审核状态机、批准幂等、来源/评分明细重建
- 创建：`internal/discovery/keyword_discoverer.go`
  - 关键词发现入口
- 创建：`internal/discovery/keyword_discoverer_test.go`
  - 覆盖去重、命中来源聚合、忽略/拉黑行为
- 创建：`internal/discovery/related_discoverer.go`
  - 一跳关系扩散发现器
- 创建：`internal/discovery/related_discoverer_test.go`
  - 覆盖基于已追踪池的一跳扩散规则与上限控制

### B 站发现数据源

- 创建：`internal/platform/bilibili/discovery.go`
  - 搜索关键词发现候选作者、拉取公开视频摘要、轻量关系扩散 API
- 创建：`internal/platform/bilibili/discovery_test.go`
  - 覆盖请求参数、响应解析、风控错误处理
- 修改：`internal/platform/bilibili/types.go`
  - 增加候选发现所需的作者/公开视频元数据结构
- 修改：`internal/platform/bilibili/client.go`
  - 暴露 discovery 所需公共方法与复用已有登录/风控/限流逻辑

### Worker / 应用装配

- 修改：`internal/worker/handler.go`
  - 接入 `discover` 任务处理分支
- 修改：`internal/worker/handler_test.go`
  - 覆盖 discover 任务成功、失败、边界状态
- 修改：`internal/app/app.go`
  - 装配候选仓储、发现服务、discover worker 依赖与 HTTP service
- 修改：`internal/app/app_test.go`
  - 覆盖装配链路与依赖注入

### HTTP API

- 创建：`internal/api/http/candidate_creators.go`
  - 候选列表、详情、审核动作、手动 discover handler
- 修改：`internal/api/http/router.go`
  - 注册候选池路由
- 修改：`internal/api/http/router_test.go`
  - 覆盖候选池接口、错误处理、方法约束

### 前端候选池

- 修改：`frontend/src/lib/api.js`
  - 增加候选池查询与审核接口
- 修改：`frontend/src/lib/state.js`
  - 增加候选池状态归一化与派生统计
- 修改：`frontend/src/App.jsx`
  - 增加候选池页面、筛选区、详情抽屉、审核动作
- 修改：`frontend/src/styles.css`
  - 增加候选池列表、分数标签、详情面板样式
- 修改：`frontend/scripts/test-dashboard-state.mjs`
  - 覆盖候选池状态归一化与统计派生
- 修改：`frontend/scripts/smoke-render.mjs`
  - 覆盖候选池页面基础渲染
- 修改：`frontend/scripts/e2e/mock-api.mjs`
  - 增加候选池 mock 数据与审核接口
- 修改：`frontend/e2e/dashboard.spec.js`
  - 覆盖候选池展示、批准/忽略/拉黑交互

### 文档

- 修改：`README.md`
  - 增加候选池发现链路说明
- 修改：`docs/api.md`
  - 增加候选池 API 文档
- 修改：`docs/config.md`
  - 增加 discovery 配置说明
- 修改：`docs/todo.md`
  - 更新种子池发现进度与后续 TODO

---

### 任务 1：建立候选池表结构与仓储接口

**文件：**
- 创建：`migrations/00003_candidate_discovery.sql`
- 修改：`internal/repo/models.go`
- 修改：`internal/repo/repo.go`
- 修改：`internal/repo/mysql/repo.go`
- 创建：`internal/repo/mysql/candidate_repo.go`
- 创建：`internal/repo/mysql/candidate_repo_test.go`

- [ ] **步骤 1：先写失败测试，锁定候选仓储边界**

覆盖用例：
- 创建或更新候选时按 `platform + uid` 去重
- 同一候选的来源按 `(candidate_creator_id, source_type, source_value)` 去重
- 替换评分明细时旧明细被清空
- 列表支持 `status / min_score / keyword / page / page_size`
- 状态流转拒绝非法更新

运行：`go test ./internal/repo/mysql -run 'TestCandidateRepo' -count=1`
预期：FAIL，提示缺少候选仓储实现

- [ ] **步骤 2：编写迁移与领域模型**

迁移至少包含：

```sql
CREATE TABLE candidate_creators (...);
CREATE TABLE candidate_creator_sources (...);
CREATE TABLE candidate_creator_score_details (...);
ALTER TABLE jobs MODIFY COLUMN type ENUM('fetch','download','check','cleanup','discover') NOT NULL;
```

新增模型至少包含：

```go
type CandidateCreator struct {
    ID               int64
    Platform         string
    UID              string
    Name             string
    AvatarURL        string
    ProfileURL       string
    FollowerCount    int64
    Status           string
    Score            int
    ScoreVersion     string
    LastDiscoveredAt time.Time
    LastScoredAt     time.Time
    ApprovedAt       time.Time
    IgnoredAt        time.Time
    BlockedAt        time.Time
    CreatedAt        time.Time
    UpdatedAt        time.Time
}
```

- [ ] **步骤 3：为仓储接口定义候选查询与审核原语**

`internal/repo/repo.go` 需要新增接口，建议形态：

```go
type CandidateRepository interface {
    Upsert(ctx context.Context, candidate CandidateCreator) (CandidateCreator, error)
    FindByID(ctx context.Context, id int64) (CandidateCreator, error)
    FindByPlatformUID(ctx context.Context, platform, uid string) (CandidateCreator, error)
    List(ctx context.Context, filter CandidateListFilter) ([]CandidateCreator, int64, error)
    ReplaceSources(ctx context.Context, candidateID int64, sources []CandidateCreatorSource) error
    ReplaceScoreDetails(ctx context.Context, candidateID int64, details []CandidateCreatorScoreDetail) error
    UpdateReviewStatus(ctx context.Context, id int64, from []string, to string, at time.Time) error
}
```

- [ ] **步骤 4：实现 MySQL 候选仓储**

要求：
- `Upsert` 返回最新候选记录
- `ReplaceSources` / `ReplaceScoreDetails` 在事务内执行
- 列表查询按 `score DESC, last_discovered_at DESC, id DESC`
- `keyword` 过滤优先匹配名称、UID、来源标签
- `candidate_creator_sources.detail_json` 要允许存储“命中视频标题、BV 号、发布时间”等最近公开视频摘要，供详情页直接读取

- [ ] **步骤 5：运行仓储测试确认通过**

运行：`go test ./internal/repo/mysql -run 'TestCandidateRepo' -count=1`
预期：PASS

- [ ] **步骤 6：Commit**

```bash
git add migrations/00003_candidate_discovery.sql internal/repo/models.go internal/repo/repo.go internal/repo/mysql/repo.go internal/repo/mysql/candidate_repo.go internal/repo/mysql/candidate_repo_test.go
git commit -m "feat(发现): 增加候选池仓储与迁移"
```

### 任务 2：补齐 discovery 配置与评分器

**文件：**
- 修改：`internal/config/config.go`
- 修改：`internal/config/config_test.go`
- 创建：`internal/discovery/types.go`
- 创建：`internal/discovery/scorer.go`
- 创建：`internal/discovery/scorer_test.go`
- 修改：`configs/config.example.yaml`

- [ ] **步骤 1：先写 discovery 配置解析失败测试**

覆盖用例：
- 默认值补齐 `enabled / interval / max_keywords_per_run / score_version`
- 非法配置（如负数页数、空关键词数组但 enabled=true）被拒绝
- 权重配置能正确解析到结构体

运行：`go test ./internal/config -run 'TestParse.*Discovery' -count=1`
预期：FAIL，提示缺少 discovery 配置结构

- [ ] **步骤 2：增加 `DiscoveryConfig` 与默认值**

建议结构：

```go
type DiscoveryConfig struct {
    Enabled                  bool
    Interval                 time.Duration
    MaxKeywordsPerRun        int
    MaxPagesPerKeyword       int
    MaxCandidatesPerRun      int
    MaxRelatedPerCreator     int
    AutoEnqueueFetchOnApprove bool
    ScoreVersion             string
    Keywords                 []string
    ScoreWeights             DiscoveryScoreWeights
}
```

- [ ] **步骤 3：先写评分器失败测试**

覆盖用例：
- 关键词风险分可累计但不超过上限
- 活跃度分按 `1-2 / 3-5 / 6+` 三档生效
- 过大账号触发负向修正
- 忽略次数带来惩罚
- `score_version` 被原样带入输出

运行：`go test ./internal/discovery -run 'TestScorer' -count=1`
预期：FAIL，提示缺少评分器实现

- [ ] **步骤 4：实现最小评分器**

输出建议形态：

```go
type ScoreResult struct {
    Total        int
    ScoreVersion string
    Details      []repo.CandidateCreatorScoreDetail
}
```

要求：
- 每个得分因子必须生成一条 `score_detail`
- 无命中因子时不要生成空明细
- 总分不做隐式魔法修正，所有变化都走显式 detail

- [ ] **步骤 5：运行配置与评分测试确认通过**

运行：`go test ./internal/config ./internal/discovery -run 'Test(Parse.*Discovery|Scorer)' -count=1`
预期：PASS

- [ ] **步骤 6：Commit**

```bash
git add internal/config/config.go internal/config/config_test.go internal/discovery/types.go internal/discovery/scorer.go internal/discovery/scorer_test.go configs/config.example.yaml
git commit -m "feat(发现): 增加 discovery 配置与评分器"
```

### 任务 3：实现候选审核服务与批准转正逻辑

**文件：**
- 创建：`internal/discovery/service.go`
- 创建：`internal/discovery/service_test.go`
- 修改：`internal/repo/repo.go`
- 修改：`internal/creator/service.go`

- [ ] **步骤 1：先写候选服务失败测试**

覆盖用例：
- 列表返回候选基础信息 + 来源摘要
- `Approve` 幂等：重复批准只创建一个正式 `creator`
- `Ignore` / `Block` / `Review` 拒绝非法流转
- `Approve` 成功后按配置决定是否为该博主入队一次“定向 fetch”

运行：`go test ./internal/discovery -run 'TestService' -count=1`
预期：FAIL，提示缺少候选服务或依赖接口

- [ ] **步骤 2：定义服务接口与事务边界**

建议公开方法：

```go
type Service struct { ... }

func (s *Service) ListCandidates(ctx context.Context, filter repo.CandidateListFilter) ([]CandidateView, int64, error)
func (s *Service) GetCandidate(ctx context.Context, id int64) (CandidateDetailView, error)
func (s *Service) Approve(ctx context.Context, id int64) (repo.Creator, error)
func (s *Service) Ignore(ctx context.Context, id int64) error
func (s *Service) Block(ctx context.Context, id int64) error
func (s *Service) Review(ctx context.Context, id int64) error
```

- [ ] **步骤 3：实现批准转正逻辑**

要求：
- 先读取候选并校验状态
- 再 Upsert 到正式 `creators`
- 最后更新候选状态为 `approved`
- 如果配置开启，则只为当前批准的博主创建一条带 `creator_id` 的定向 `fetch` 任务，不能退化成“全量 fetch”
- 整个过程必须幂等，重复调用不重复插入正式追踪

- [ ] **步骤 4：运行候选服务测试确认通过**

运行：`go test ./internal/discovery -run 'TestService' -count=1`
预期：PASS

- [ ] **步骤 5：Commit**

```bash
git add internal/discovery/service.go internal/discovery/service_test.go internal/creator/service.go internal/repo/repo.go
git commit -m "feat(发现): 增加候选审核与批准转正服务"
```

### 任务 4：接入 B 站关键词发现数据源

**文件：**
- 创建：`internal/platform/bilibili/discovery.go`
- 创建：`internal/platform/bilibili/discovery_test.go`
- 修改：`internal/platform/bilibili/types.go`
- 修改：`internal/platform/bilibili/client.go`
- 创建：`internal/discovery/keyword_discoverer.go`
- 创建：`internal/discovery/keyword_discoverer_test.go`

- [ ] **步骤 1：先写 B 站 discovery API 失败测试**

覆盖用例：
- 关键词搜索请求参数正确拼装
- 能从响应中解析作者 UID、名称、粉丝量、公开视频摘要
- 遇到 `-412 / -403 / 未登录` 等风控或认证错误时，能返回可识别错误

运行：`go test ./internal/platform/bilibili -run 'Test(SearchCreators|SearchVideos)' -count=1`
预期：FAIL，提示缺少 discovery API

- [ ] **步骤 2：实现 discovery 数据源方法**

建议公开方法：

```go
func (c *Client) SearchCreators(ctx context.Context, keyword string, page, pageSize int) ([]CreatorHit, error)
func (c *Client) SearchVideos(ctx context.Context, keyword string, page, pageSize int) ([]VideoHit, error)
```

要求：
- 复用现有 cookie、风控退避、请求超时逻辑
- 不新增绕过风控的特殊逻辑
- 统一把 discovery 接口错误转换为中文、可定位日志
- 在 discovery 响应解析阶段就提取最近公开视频摘要，并写入 `detail_json`，避免详情页临时再次打远端接口

- [ ] **步骤 3：先写关键词发现器失败测试**

覆盖用例：
- 同一作者被多个关键词命中时只产出一条候选
- 多来源关键词被聚合为多条 `source`
- `blocked` 候选被过滤
- `ignored` 候选再次命中时保持 `ignored`，但刷新来源和发现时间

运行：`go test ./internal/discovery -run 'TestKeywordDiscoverer' -count=1`
预期：FAIL

- [ ] **步骤 4：实现关键词发现器**

发现器输入建议：

```go
type KeywordDiscoverer struct {
    client CandidateSourceClient
    repo   repo.CandidateRepository
    scorer *Scorer
    cfg    config.DiscoveryConfig
}
```

要求：
- 单轮最多处理 `MaxKeywordsPerRun`
- 单关键词最多翻 `MaxPagesPerKeyword`
- 总候选数到达 `MaxCandidatesPerRun` 后立即停止
- 每轮写入前统一重算分数和明细

- [ ] **步骤 5：运行 discovery 相关测试确认通过**

运行：`go test ./internal/platform/bilibili ./internal/discovery -run 'Test(SearchCreators|SearchVideos|KeywordDiscoverer)' -count=1`
预期：PASS

- [ ] **步骤 6：Commit**

```bash
git add internal/platform/bilibili/discovery.go internal/platform/bilibili/discovery_test.go internal/platform/bilibili/types.go internal/platform/bilibili/client.go internal/discovery/keyword_discoverer.go internal/discovery/keyword_discoverer_test.go
git commit -m "feat(发现): 接入 B 站关键词候选发现"
```

### 任务 5：把 discover 任务接入作业系统与应用装配

**文件：**
- 修改：`internal/jobs/types.go`
- 修改：`internal/jobs/service.go`
- 修改：`internal/jobs/service_test.go`
- 修改：`internal/scheduler/scheduler.go`
- 修改：`internal/scheduler/scheduler_test.go`
- 修改：`internal/worker/handler.go`
- 修改：`internal/worker/handler_test.go`
- 修改：`internal/app/app.go`
- 修改：`internal/app/app_test.go`

- [ ] **步骤 1：先写 jobs/scheduler 失败测试**

覆盖用例：
- `EnqueueDiscover` 成功入队
- `discovery.enabled=false` 时调度器不创建 discover ticker
- `discovery.enabled=true` 时按 `interval` 触发 discover
- `EnqueueFetchCreator` 只为单个 `creator_id` 入队，不创建全量拉取任务

运行：`go test ./internal/jobs ./internal/scheduler -run 'Test(EnqueueDiscover|Scheduler.*Discover)' -count=1`
预期：FAIL

- [ ] **步骤 2：实现 discover 入队与调度逻辑**

要求：
- `jobs.TypeDiscover = "discover"`
- `jobs.Service` 新增 `EnqueueDiscover(ctx)`
- `jobs.Service` 新增 `EnqueueFetchCreator(ctx, creatorID int64)`，payload 形如 `{"creator_id": 123}`
- `scheduler.JobService` 扩展 discover 入队能力
- 调度器只在 `cfg.Discovery.Enabled` 且 `Interval > 0` 时创建 discover ticker

- [ ] **步骤 3：先写 worker discover 失败测试**

覆盖用例：
- `job.Type == discover` 时调用 discovery 服务
- `job.Type == fetch` 且 payload 带 `creator_id` 时只拉取该博主
- discovery 失败只标记 discover 任务失败，不影响其他任务分支
- discovery 成功不会误创建视频下载任务

运行：`go test ./internal/worker -run 'TestHandleDiscover' -count=1`
预期：FAIL，提示未知任务类型或依赖未注入

- [ ] **步骤 4：实现 discover 任务处理分支**

建议做法：
- 为 `DefaultHandler` 注入 discovery runner 接口，而不是直接依赖具体实现
- 在 `Handle()` 的 `switch job.Type` 中新增 `jobs.TypeDiscover`
- `discover` 分支只执行候选发现，不碰视频状态
- `fetch` 分支读取可选 `creator_id` payload；若存在则只处理该博主，否则沿用全量逻辑

- [ ] **步骤 5：完成 app 装配**

要求：
- `App.New()` 组装候选仓储与 discovery service
- `newRouter` 增加 candidate service 注入
- `newScheduler` 能拿到 discovery 配置
- 尽量沿用现有依赖注入风格，不引入全局单例

- [ ] **步骤 6：运行作业与装配测试确认通过**

运行：`go test ./internal/jobs ./internal/scheduler ./internal/worker ./internal/app -count=1`
预期：PASS

- [ ] **步骤 7：Commit**

```bash
git add internal/jobs/types.go internal/jobs/service.go internal/jobs/service_test.go internal/scheduler/scheduler.go internal/scheduler/scheduler_test.go internal/worker/handler.go internal/worker/handler_test.go internal/app/app.go internal/app/app_test.go
git commit -m "feat(发现): 接入 discover 任务与应用装配"
```

### 任务 6：暴露候选池 HTTP API

**文件：**
- 创建：`internal/api/http/candidate_creators.go`
- 修改：`internal/api/http/router.go`
- 修改：`internal/api/http/router_test.go`

- [ ] **步骤 1：先写 API 失败测试**

覆盖用例：
- `GET /candidate-creators` 返回列表和分页元数据
- `GET /candidate-creators/:id` 返回详情、来源、评分明细
- `POST /candidate-creators/discover` 手动触发 discover
- `POST /candidate-creators/:id/approve|ignore|block|review` 正常工作
- 错误状态、非法方法、服务未注入时返回中文错误

运行：`go test ./internal/api/http -run 'TestCandidateCreators' -count=1`
预期：FAIL，提示路由或 handler 缺失

- [ ] **步骤 2：实现候选池 handler**

建议接口依赖：

```go
type CandidateService interface {
    ListCandidates(ctx context.Context, filter repo.CandidateListFilter) ([]discovery.CandidateView, int64, error)
    GetCandidate(ctx context.Context, id int64) (discovery.CandidateDetailView, error)
    TriggerDiscover(ctx context.Context) error
    Approve(ctx context.Context, id int64) (repo.Creator, error)
    Ignore(ctx context.Context, id int64) error
    Block(ctx context.Context, id int64) error
    Review(ctx context.Context, id int64) error
}
```

- [ ] **步骤 3：注册路由并统一返回结构**

建议路由：
- `GET /candidate-creators`
- `GET /candidate-creators/:id`
- `POST /candidate-creators/discover`
- `POST /candidate-creators/:id/approve`
- `POST /candidate-creators/:id/ignore`
- `POST /candidate-creators/:id/block`
- `POST /candidate-creators/:id/review`

- [ ] **步骤 4：运行 HTTP 测试确认通过**

运行：`go test ./internal/api/http -run 'TestCandidateCreators' -count=1`
预期：PASS

- [ ] **步骤 5：Commit**

```bash
git add internal/api/http/candidate_creators.go internal/api/http/router.go internal/api/http/router_test.go
git commit -m "feat(发现): 增加候选池 HTTP API"
```

### 任务 7：实现前端候选池页面与人工审核流

**文件：**
- 修改：`frontend/src/lib/api.js`
- 修改：`frontend/src/lib/state.js`
- 修改：`frontend/src/App.jsx`
- 修改：`frontend/src/styles.css`
- 修改：`frontend/scripts/test-dashboard-state.mjs`
- 修改：`frontend/scripts/smoke-render.mjs`
- 修改：`frontend/scripts/e2e/mock-api.mjs`
- 修改：`frontend/e2e/dashboard.spec.js`

- [ ] **步骤 1：先写前端状态失败测试**

覆盖用例：
- 候选池列表归一化
- 分数段统计
- 审核动作后状态变化
- 详情抽屉所需来源和评分明细结构映射

运行：`cd frontend && npm run test:state`
预期：FAIL，提示缺少 candidate state 处理

- [ ] **步骤 2：扩展前端 API 层**

新增方法：

```js
export async function listCandidateCreators(baseURL, filters = {}) {}
export async function getCandidateCreator(baseURL, id) {}
export async function triggerCandidateDiscover(baseURL) {}
export async function approveCandidateCreator(baseURL, id) {}
export async function ignoreCandidateCreator(baseURL, id) {}
export async function blockCandidateCreator(baseURL, id) {}
export async function reviewCandidateCreator(baseURL, id) {}
```

- [ ] **步骤 3：实现候选池页面**

要求：
- 左侧导航新增 `候选池`
- 顶部展示新候选数、高优候选数、今日发现数、已忽略数
- 列表支持状态、最低分、关键词筛选
- 行级操作：`加入追踪 / 忽略 / 拉黑 / 查看详情`
- 详情抽屉展示来源列表与评分明细

- [ ] **步骤 4：补 E2E / smoke 验证**

运行：
- `cd frontend && npm run test:smoke`
- `cd frontend && npm run test:e2e`

预期：先 FAIL，再 PASS

- [ ] **步骤 5：运行前端完整验证**

运行：`cd frontend && npm run test:state && npm run test:smoke && npm run build`
预期：PASS

- [ ] **步骤 6：Commit**

```bash
git add frontend/src/lib/api.js frontend/src/lib/state.js frontend/src/App.jsx frontend/src/styles.css frontend/scripts/test-dashboard-state.mjs frontend/scripts/smoke-render.mjs frontend/scripts/e2e/mock-api.mjs frontend/e2e/dashboard.spec.js
git commit -m "feat(发现): 增加候选池前端审核页面"
```

### 任务 8：补充一跳关系扩散与验收文档

**文件：**
- 创建：`internal/discovery/related_discoverer.go`
- 创建：`internal/discovery/related_discoverer_test.go`
- 修改：`internal/discovery/service.go`
- 修改：`internal/platform/bilibili/discovery.go`
- 修改：`internal/platform/bilibili/discovery_test.go`
- 修改：`README.md`
- 修改：`docs/api.md`
- 修改：`docs/config.md`
- 修改：`docs/todo.md`

- [ ] **步骤 1：先写关系扩散失败测试**

覆盖用例：
- 从已追踪博主出发只做一跳扩散
- 每个来源博主最多返回 `MaxRelatedPerCreator`
- 同一候选同时被关键词和关系扩散命中时来源会聚合

运行：`go test ./internal/discovery ./internal/platform/bilibili -run 'TestRelatedDiscoverer' -count=1`
预期：FAIL

- [ ] **步骤 2：实现轻量关系扩散**

建议优先顺序：
- 先复用关键词视频搜索结果中的作者共现
- 再补充已追踪博主公开视频标题词相似度
- 严格限制只做一跳，不做递归扩散

- [ ] **步骤 3：更新文档**

文档至少覆盖：
- 候选池使用方式
- discovery 配置说明
- 候选池 API
- 当前一期只支持 B 站、人工审核转正
- 批准后可选触发“定向拉取”，不会触发全量拉取

- [ ] **步骤 4：运行项目级验证**

运行：
- `go test ./... -count=1`
- `cd frontend && npm run test:state && npm run test:smoke && npm run build`

如环境允许，再运行：
- `docker compose build`

预期：PASS

- [ ] **步骤 5：Commit**

```bash
git add internal/discovery/related_discoverer.go internal/discovery/related_discoverer_test.go internal/discovery/service.go internal/platform/bilibili/discovery.go internal/platform/bilibili/discovery_test.go README.md docs/api.md docs/config.md docs/todo.md
git commit -m "feat(发现): 补齐关系扩散与交付文档"
```

## 执行注意事项

- 严格遵循 TDD：先写失败测试，再写最小实现，再运行通过。
- 不要在发现链路里顺手改下载、检查、清理的业务逻辑，除非测试明确证明需要。
- `discover` 任务与候选池状态必须做到幂等，否则会很快引入重复候选和重复正式追踪。
- Web 页面先追求“可解释和可用”，不要提前做复杂图表和批量操作。
- 如果关系扩散在真实 API 上风险过高，可以先只交付关键词发现，把关系扩散作为单独的后续 commit，但必须保留接口与测试边界。

## 建议的提交顺序

1. `feat(发现): 增加候选池仓储与迁移`
2. `feat(发现): 增加 discovery 配置与评分器`
3. `feat(发现): 增加候选审核与批准转正服务`
4. `feat(发现): 接入 B 站关键词候选发现`
5. `feat(发现): 接入 discover 任务与应用装配`
6. `feat(发现): 增加候选池 HTTP API`
7. `feat(发现): 增加候选池前端审核页面`
8. `feat(发现): 补齐关系扩散与交付文档`
