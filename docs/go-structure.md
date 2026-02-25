# Go 工程结构与并发设计（worker pool）

## 1. 目录结构建议
```
fetch-bilibili/
  cmd/
    server/
      main.go
  internal/
    app/
      app.go              # 组装依赖、启动组件
    config/
      config.go           # 配置加载与校验
    api/
      http/               # HTTP 路由与 handler
    domain/
      creators.go
      videos.go
      jobs.go
    repo/
      mysql/              # MySQL 实现
    scheduler/
      scheduler.go        # 任务调度与编排
    worker/
      queue.go            # 任务队列（内存/持久化）
      pool.go             # worker pool
      handlers.go         # 任务处理器
    platform/
      bilibili/
        client.go         # API 调用与限速
        fetcher.go         # 拉取视频列表
        downloader.go      # 下载器
        checker.go         # 下架检测
    storage/
      manager.go          # 容量评估与清理策略
    observability/
      logger.go
      metrics.go
  migrations/
    0001_init.sql
  configs/
    config.example.yaml
  scripts/
    run_local.sh
  docs/
  README.md
```

## 2. 模块职责
- `cmd/server`：服务入口（仅做初始化与启动）。
- `internal/app`：装配依赖（DB/Repo/Client/Scheduler/Worker）。
- `internal/domain`：核心业务模型与状态机逻辑（避免耦合基础设施）。
- `internal/repo/mysql`：数据持久化实现。
- `internal/platform/bilibili`：B 站适配器实现。
- `internal/scheduler`：周期性调度器（拉取/检查/清理）。
- `internal/worker`：统一任务队列与 worker pool。
- `internal/storage`：清理策略与评分模型。
- `internal/observability`：日志与指标封装。

## 3. worker pool 设计（核心）

### 3.1 设计目标
- 高并发 I/O（下载/检查）与可控速率（QPS 限制）。
- 任务幂等：同一视频不会重复执行。
- 可重试：失败任务可自动退避重试。
- 可扩展：后续多平台使用统一任务类型。

### 3.2 任务模型
```go
// 任务类型
const (
  JobFetch   = "fetch"
  JobDownload= "download"
  JobCheck   = "check"
  JobCleanup = "cleanup"
)

type Job struct {
  ID         int64
  Type       string
  Payload    map[string]any
  RetryCount int
  MaxRetry   int
  NotBefore  time.Time
  CreatedAt  time.Time
}
```

### 3.3 队列模型（简化）
- 默认实现：内存队列 + MySQL 任务表（持久化）。
- 拉取方式：`SELECT ... FOR UPDATE SKIP LOCKED` 取 `queued` 任务。
- 去重键：
  - fetch：creator_id + schedule_window
  - download：video_id
  - check：video_id + scheduled_at
  - cleanup：schedule_window

### 3.4 Worker Pool
- `N` 个 worker goroutine 从队列消费任务。
- 各类型任务可配置并发上限（下载/检查独立）。
- 任务超时由 `context.WithTimeout` 控制。

```go
type WorkerPool struct {
  workers int
  queue   JobQueue
  handler JobHandler
  limiter *rate.Limiter // 全局限速
}

func (p *WorkerPool) Start(ctx context.Context) {
  for i := 0; i < p.workers; i++ {
    go func() {
      for {
        job, ok := p.queue.Pop(ctx)
        if !ok { return }
        p.handleOne(ctx, job)
      }
    }()
  }
}
```

### 3.5 限速与并发
- 全局限速：`rate.Limiter` 控制 QPS。
- 按博主限速：在 B 站 client 中使用 `map[creator_id]*rate.Limiter`。
- 并发控制：下载/检查分别使用 `semaphore`。

### 3.6 重试与退避
- 失败后：写入 `failed` 并计算 `NotBefore` 重新入队（指数退避）。
- 重试次数达到上限：标记为 `failed`，并记录错误详情。

### 3.7 幂等性
- 下载任务：
  - 检查 `videos.state` 是否已是 `DOWNLOADED`/`OUT_OF_PRINT`。
  - 文件存在且校验通过时直接成功返回。
- 检查任务：
  - 仅当 `last_check_at` < `scheduled_at` 才执行。

## 4. Scheduler 设计
- 使用 ticker 或 cron-like 触发：
  - fetch: 30-60min
  - check: 24h
  - cleanup: 24h
- 生成 job 写入 `jobs` 表，由 worker pool 消费。

## 5. 平台适配器接口（抽象）
```go
type Platform interface {
  ResolveCreator(name string) (uid string, err error)
  ListVideos(uid string) ([]VideoMeta, error)
  DownloadVideo(videoID string, dest string) (VideoFile, error)
  CheckAvailable(videoID string) (bool, error)
}
```

## 6. 线程安全与上下文
- 所有任务处理使用 `context`，支持取消与超时。
- DB/HTTP 客户端复用连接池。
- 共享 map 必须加锁或使用 sync.Map。

## 7. 本地部署建议
- MySQL 使用本地实例或 Docker。
- 配置文件 `configs/config.yaml`。
- 日志默认 JSON 输出到 stdout。
