# Worker 消费端设计（当前实现）

## 1. 功能概述
- Worker 周期性从 `jobs` 表取出待执行任务。
- 默认实现是“拉取 1 条 → 执行 → 更新状态”。
- `fetch/check` 已接入 B 站真实接口（WBI 签名 + view 检查）。

## 2. 核心流程
1) `FetchQueued` 使用 `FOR UPDATE SKIP LOCKED` 取任务并标记为 `running`。
2) 执行 handler。
3) 成功则更新为 `success`，失败更新为 `failed` 并记录错误信息。

## 3. 并发与节流
- `workers` 数量由配置 `limits.download_concurrency` 决定（后续可拆分下载/检查并发）。
- `pollEvery` 默认为 2 秒。

## 4. 未来扩展
- 为不同任务类型使用独立 worker pool（download/check/cleanup）。
- 引入 backoff 与重试策略。
- 将 `handler` 拆分为具体任务处理器。
