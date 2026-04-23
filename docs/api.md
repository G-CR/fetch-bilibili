# API 文档（当前实现）

本文档只描述当前仓库已经实现并对外暴露的 HTTP / SSE 契约，不包含历史设想
或未来规划。

## 1. 通用约定

- 当前接口没有 `/api/v1` 前缀，直接挂在根路径下，例如 `/creators`、
  `/jobs`、`/events/stream`。
- 除 `GET /healthz`、`GET /readyz` 和 `GET /events/stream` 外，其余接口都返
  回 JSON。
- JSON 字段统一使用 `snake_case`。
- 时间字段统一使用 RFC 3339 字符串。HTTP 快照中，带 `omitempty` 的字段在零
  值时会被省略；未加 `omitempty` 的字段，以及当前用 `map` 手工构造的 SSE
  payload，零值仍可能编码为空字符串。
- 多数业务错误路径返回如下 JSON：

```json
{
  "error": "错误原因"
}
```

- 当前部分 handler 在方法不支持等路径上直接写状态码，响应可能没有 JSON
  body；当前典型包括部分 `405 Method Not Allowed`，以及 SSE 的非 JSON 错误
  路径。

- 当前 CORS 策略为：
  - `Access-Control-Allow-Origin: *`
  - `Access-Control-Allow-Methods: GET,POST,PUT,PATCH,DELETE,OPTIONS`
  - `Access-Control-Allow-Headers: Content-Type`
  - `OPTIONS` 预检返回 `204 No Content`

## 2. 健康检查

### 2.1 `GET /healthz`

- 用途：基础存活检查。
- 响应：

```text
ok
```

### 2.2 `GET /readyz`

- 用途：基础就绪检查。
- 响应：

```text
ready
```

## 3. 博主管理

### 3.1 `POST /creators`

- 用途：新增或更新一个正式追踪博主。
- 请求体字段：
  - `uid`：可选；与 `name` 至少提供一个。
  - `name`：可选；与 `uid` 至少提供一个。
  - `platform`：可选；默认 `bilibili`。
  - `status`：可选；默认 `active`。
- 说明：
  - 仅提供 `name` 时，依赖名称解析器补 UID；若运行时没有可用解析器，接口
    会失败。
  - 当前响应会同时返回 `local_video_count` 和 `storage_bytes`。

- 请求示例：

```json
{
  "uid": "123456",
  "name": "示例博主",
  "platform": "bilibili",
  "status": "active"
}
```

- 成功响应示例：

```json
{
  "id": 1,
  "uid": "123456",
  "name": "示例博主",
  "platform": "bilibili",
  "status": "active",
  "local_video_count": 0,
  "storage_bytes": 0
}
```

- 常见错误：
  - `400`：请求体不是合法 JSON，或 `uid`、`name` 都未提供。
  - `503`：博主服务未就绪。
  - `500`：名称解析失败、仓储失败或其他内部错误。

### 3.2 `GET /creators`

- 用途：查询当前活跃博主列表。
- 查询参数：
  - `limit`：可选；默认 `200`；必须大于 `0`。
- 说明：
  - 当前只返回 `status=active` 的博主，不返回 `paused` 或 `removed`。

- 响应示例：

```json
{
  "items": [
    {
      "id": 1,
      "uid": "123456",
      "name": "示例博主",
      "platform": "bilibili",
      "status": "active",
      "local_video_count": 4,
      "storage_bytes": 4096
    }
  ]
}
```

### 3.3 `PATCH /creators/{id}`

- 用途：更新单个博主的可运维字段。
- 请求体字段：
  - `name`：可选；提供时不能为空白字符串。
  - `status`：可选；提供时不能为空白字符串。
- 说明：
  - 两个字段至少提供一个。
  - 当前接口层只校验 `status` 非空；现有业务基线状态为
    `active`、`paused`、`removed`。

- 请求示例：

```json
{
  "name": "新的博主名称",
  "status": "paused"
}
```

- 成功响应示例：

```json
{
  "id": 1,
  "uid": "123456",
  "name": "新的博主名称",
  "platform": "bilibili",
  "status": "paused",
  "local_video_count": 4,
  "storage_bytes": 4096
}
```

- 常见错误：
  - `400`：`id` 非法、请求体不是合法 JSON，或没有提供任何更新字段。
  - `404`：博主不存在。
  - `503`：博主服务未就绪。
  - `500`：内部错误。

### 3.4 `DELETE /creators/{id}`

- 用途：手工移除一个博主。
- 说明：
  - 当前实现是把博主状态改成 `removed`，不是物理删除。
  - 已经是 `removed` 的博主再次删除时，仍返回成功。
  - 本地归档数据不会因这个接口被立即删除。

- 成功响应：

```text
204 No Content
```

- 常见错误：
  - `400`：`id` 非法。
  - `404`：博主不存在。
  - `503`：博主服务未就绪。
  - `500`：内部错误。

## 4. 候选池

- 当前只支持 `bilibili`。
- 当前候选池主流程状态基线为
  `reviewing`、`approved`、`ignored`、`blocked`；底层 schema 仍保留 `new`。
- 手动触发 discover 的入口是 `POST /candidate-creators/discover`，不是
  `POST /jobs`。

### 4.1 `GET /candidate-creators`

- 用途：查询候选博主列表。
- 查询参数：
  - `status`：可选；按候选状态精确过滤。
  - `min_score`：可选；最小分数，必须大于等于 `0`。
  - `keyword`：可选；按名称、UID 或来源标签模糊过滤。
  - `page`：可选；默认 `1`；必须大于 `0`。
  - `page_size`：可选；默认 `20`；必须大于 `0`。

- 响应示例：

```json
{
  "items": [
    {
      "id": 301,
      "platform": "bilibili",
      "uid": "9001",
      "name": "候选补档站",
      "avatar_url": "",
      "profile_url": "https://space.bilibili.com/9001",
      "follower_count": 321000,
      "status": "reviewing",
      "score": 88,
      "score_version": "v1",
      "last_discovered_at": "2026-04-20T12:00:00Z",
      "last_scored_at": "2026-04-20T12:00:00Z",
      "approved_at": "",
      "ignored_at": "",
      "blocked_at": "",
      "created_at": "2026-04-20T12:00:00Z",
      "updated_at": "2026-04-20T12:00:00Z",
      "sources": [
        {
          "id": 1,
          "source_type": "keyword",
          "source_value": "补档",
          "source_label": "关键词：补档",
          "weight": 15,
          "created_at": "2026-04-20T12:00:00Z"
        }
      ]
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 20
}
```

- 常见错误：
  - `400`：分页或 `min_score` 参数非法。
  - `503`：候选池服务未就绪。
  - `500`：内部错误。

### 4.2 `GET /candidate-creators/{id}`

- 用途：查询单个候选详情。
- 响应包含三部分：
  - `candidate`：候选主体。
  - `sources`：来源列表。
  - `score_details`：评分明细。

- 响应示例：

```json
{
  "candidate": {
    "id": 301,
    "platform": "bilibili",
    "uid": "9001",
    "name": "候选补档站",
    "avatar_url": "",
    "profile_url": "https://space.bilibili.com/9001",
    "follower_count": 321000,
    "status": "reviewing",
    "score": 88,
    "score_version": "v1",
    "last_discovered_at": "2026-04-20T12:00:00Z",
    "last_scored_at": "2026-04-20T12:00:00Z",
    "approved_at": "",
    "ignored_at": "",
    "blocked_at": "",
    "created_at": "2026-04-20T12:00:00Z",
    "updated_at": "2026-04-20T12:00:00Z"
  },
  "sources": [
    {
      "id": 1,
      "source_type": "keyword",
      "source_value": "补档",
      "source_label": "关键词：补档",
      "weight": 15,
      "detail_json": {
        "keyword": "补档"
      },
      "created_at": "2026-04-20T12:00:00Z"
    }
  ],
  "score_details": [
    {
      "id": 11,
      "factor_key": "keyword_risk",
      "factor_label": "命中高风险关键词",
      "score_delta": 15,
      "detail_json": {
        "keywords": [
          "补档"
        ]
      },
      "created_at": "2026-04-20T12:00:00Z"
    }
  ]
}
```

- 常见错误：
  - `400`：`id` 非法。
  - `404`：候选不存在。
  - `503`：候选池服务未就绪。
  - `500`：内部错误。

### 4.3 `POST /candidate-creators/discover`

- 用途：手动触发一次 discover 任务。
- 请求体：无。

- 成功响应示例：

```json
{
  "status": "queued",
  "type": "discover"
}
```

### 4.4 `POST /candidate-creators/{id}/approve`

- 用途：批准候选并转成正式追踪博主。
- 说明：
  - 当前只允许从 `reviewing` 批准。
  - 已经是 `approved` 的候选再次批准时，会按幂等方式返回成功。
  - 若 `discovery.auto_enqueue_fetch_on_approve=true`，批准后会补一次该博主的定
    向 fetch。

- 成功响应示例：

```json
{
  "id": 21,
  "uid": "9001",
  "name": "候选补档站",
  "platform": "bilibili",
  "status": "active",
  "local_video_count": 0,
  "storage_bytes": 0
}
```

### 4.5 `POST /candidate-creators/{id}/ignore`

- 用途：把候选从 `reviewing` 标记为 `ignored`。

### 4.6 `POST /candidate-creators/{id}/block`

- 用途：把候选从 `reviewing` 标记为 `blocked`。

### 4.7 `POST /candidate-creators/{id}/review`

- 用途：把候选从 `ignored` 恢复为 `reviewing`。

- 4.5 到 4.7 的成功响应格式一致：

```json
{
  "status": "ok",
  "action": "ignore",
  "candidate_id": 301
}
```

- 常见错误：
  - `400`：`id` 非法，或命中非法状态流转。
  - `404`：候选不存在。
  - `503`：候选池服务未就绪。
  - `500`：内部错误。

## 5. 任务

- 所有入队接口当前都可能在“已有同类活动任务”时仍返回成功。
- 成功返回 `queued` 只表示“任务已入队，或已存在可复用的活动任务”，不表示
  一定新建了一条数据库记录。

### 5.1 `POST /jobs`

- 用途：手动触发全局任务。
- 请求体字段：
  - `type`：必填；当前只支持 `fetch`、`check`、`cleanup`。
- 说明：
  - 当前不支持通过这个接口触发 `discover`。

- 请求示例：

```json
{
  "type": "fetch"
}
```

- 成功响应示例：

```json
{
  "status": "queued",
  "type": "fetch"
}
```

- 常见错误：
  - `400`：请求体不是合法 JSON，`type` 为空，或 `type` 不在支持范围内。
  - `503`：任务服务未就绪。
  - `500`：内部错误。

### 5.2 `GET /jobs`

- 用途：查询最近任务。
- 查询参数：
  - `limit`：可选；默认 `20`；必须大于 `0`。
  - `status`：可选；按状态精确过滤。
  - `type`：可选；按类型精确过滤。
- 当前任务状态基线为：`queued`、`running`、`success`、`failed`。
- 当前 HTTP 响应里，`error_msg`、`not_before`、`started_at`、`finished_at`
  为空时会被省略，不会固定返回空字符串。

- 响应示例：

```json
{
  "items": [
    {
      "id": 11,
      "type": "fetch",
      "status": "queued",
      "payload": {
        "scope": "all"
      },
      "created_at": "2026-04-13T12:00:00Z",
      "updated_at": "2026-04-13T12:00:00Z"
    }
  ]
}
```

## 6. 视频

### 6.1 `GET /videos`

- 用途：查询最近视频。
- 查询参数：
  - `limit`：可选；默认 `20`；必须大于 `0`。
  - `creator_id`：可选；按博主 ID 过滤；必须大于 `0`。
  - `state`：可选；按视频状态精确过滤。
- 当前 `GET /videos` 的公开状态基线为：
  `NEW`、`DOWNLOADING`、`DOWNLOADED`、`OUT_OF_PRINT`、`STABLE`、`DELETED`。

- 响应示例：

```json
{
  "items": [
    {
      "id": 21,
      "platform": "bilibili",
      "video_id": "BV1xx411c7mD",
      "creator_id": 1,
      "title": "稀有投稿",
      "description": "",
      "publish_time": "2026-04-12T09:30:00Z",
      "duration": 360,
      "cover_url": "",
      "view_count": 1024,
      "favorite_count": 88,
      "state": "OUT_OF_PRINT",
      "out_of_print_at": "2026-04-13T00:00:00Z",
      "stable_at": "",
      "last_check_at": "2026-04-13T12:00:00Z"
    }
  ]
}
```

### 6.2 `GET /videos/{id}`

- 用途：查询单个视频详情。
- 响应结构与 `GET /videos` 中的单个 `item` 相同。

- 常见错误：
  - `400`：`id` 非法。
  - `404`：视频不存在。
  - `503`：视频查询服务未就绪。
  - `500`：内部错误。

### 6.3 `POST /videos/{id}/download`

- 用途：为指定视频创建下载任务。
- 请求体：无。

- 成功响应示例：

```json
{
  "status": "queued",
  "type": "download",
  "video_id": 21
}
```

### 6.4 `POST /videos/{id}/check`

- 用途：为指定视频创建定向检查任务。
- 请求体：无。
- 说明：
  - 实际入队的任务类型仍是 `check`，并通过 payload 携带 `video_id`。

- 成功响应示例：

```json
{
  "status": "queued",
  "type": "check",
  "video_id": 21
}
```

- 6.3 与 6.4 的常见错误：
  - `400`：`id` 非法。
  - `404`：视频不存在。
  - `503`：视频运维服务未就绪。
  - `500`：查询或入队失败。

## 7. 系统与存储

### 7.1 `GET /system/status`

- 用途：提供驾驶舱总览状态。
- 主要字段：
  - `health`：当前为 `online` 或 `degraded`。
  - `mysql_ok`：MySQL 连通性。
  - `auth_enabled`：是否同时具备 Cookie 配置与认证检查能力。
  - `cookie.status`：当前可能为
    `not_configured`、`unknown`、`valid`、`invalid`、`error`。
  - `risk.level` / `risk_level`：当前可能为 `低`、`中`、`高`。
  - `limits`：当前生效的限速与并发配置。
  - `scheduler`：当前生效的调度周期。

- 响应示例：

```json
{
  "health": "online",
  "mysql_ok": true,
  "auth_enabled": true,
  "cookie": {
    "configured": true,
    "is_login": true,
    "mid": 10001,
    "uname": "tester",
    "status": "valid",
    "source": "config",
    "last_check_at": "2026-04-13T12:00:00Z",
    "last_check_result": "valid",
    "last_reload_at": "2026-04-13T11:50:00Z",
    "last_reload_result": "no_change",
    "last_error": ""
  },
  "risk": {
    "level": "高",
    "active": true,
    "backoff_until": "2026-04-13T12:01:00Z",
    "backoff_seconds": 18,
    "last_hit_at": "2026-04-13T11:59:42Z",
    "last_reason": "/x/web-interface/nav 返回风控码 -412"
  },
  "overview": {
    "active_creators": 0,
    "pending_jobs": 0,
    "rare_videos": 0
  },
  "active_jobs": 0,
  "last_job_at": "2026-04-13T02:43:35Z",
  "risk_level": "高",
  "limits": {
    "global_qps": 2,
    "per_creator_qps": 1,
    "download_concurrency": 4,
    "check_concurrency": 8
  },
  "scheduler": {
    "fetch_interval": "45m0s",
    "check_interval": "24h0m0s",
    "cleanup_interval": "24h0m0s",
    "check_stable_days": 30
  },
  "storage_root": "/data/bilibili"
}
```

### 7.2 `GET /storage/stats`

- 用途：提供驾驶舱存储面板数据。

- 响应示例：

```json
{
  "root_dir": "/data/bilibili",
  "used_bytes": 7340032,
  "max_bytes": 2199023255552,
  "safe_bytes": 1979120930000,
  "usage_percent": 0,
  "file_count": 2,
  "hottest_bucket": "bilibili",
  "rare_videos": 0,
  "cleanup_rule": "绝版优先 -> 粉丝量 -> 播放量 -> 收藏量"
}
```

### 7.3 `POST /storage/cleanup`

- 用途：手动触发一次 cleanup 任务。
- 请求体：无。

- 成功响应示例：

```json
{
  "status": "queued",
  "type": "cleanup"
}
```

### 7.4 `GET /system/config`

- 用途：读取当前运行配置文件路径与全文内容。

- 响应示例：

```json
{
  "path": "/app/config.yaml",
  "content": "storage:\n  root_dir: /data\nmysql:\n  dsn: test\n"
}
```

### 7.5 `PUT /system/config`

- 用途：保存新的配置文件全文；内容变化时会在写回后安排重启。
- 请求体字段：
  - `content`：必填；完整配置文本。
- 说明：
  - 当前不支持局部字段补丁。
  - 服务端会先执行 YAML 解析与业务配置校验。
  - `changed=false` 表示内容未变化，也不会触发重启。

- 请求示例：

```json
{
  "content": "storage:\n  root_dir: /data\nmysql:\n  dsn: test\n"
}
```

- 响应示例：

```json
{
  "changed": true,
  "restart_scheduled": true,
  "path": "/app/config.yaml"
}
```

## 8. 实时事件流

### 8.1 `GET /events/stream`

- 用途：为前端驾驶舱提供实时增量事件。
- 协议：SSE（`text/event-stream`）。
- 服务端行为：
  - 建连后立即发送一次 `hello`。
  - 默认每 `15` 秒发送一次 `heartbeat`。
  - 事件 `data:` 后面直接是 JSON 对象，不再额外包一层统一 envelope。
- 推荐消费方式：
  - 首屏先请求快照接口：
    `GET /creators`、`GET /jobs`、`GET /videos`、`GET /system/status`、
    `GET /storage/stats`。
  - 再建立 `EventSource`。
  - 断线重连成功后，再补一次快照以修正漂移。

### 8.2 当前事件类型

#### `hello`

```text
event: hello
data: {"server_time":"2026-04-15T08:00:00Z"}
```

#### `heartbeat`

```text
event: heartbeat
data: {"server_time":"2026-04-15T08:00:15Z"}
```

#### `job.changed`

- `data` 直接是任务对象，不包 `job` 字段。
- 当前事件 payload 由 `map` 直接编码，空 `error_msg` 和空时间字段会保留为空
  字符串，而不是省略。

```text
event: job.changed
data: {"id":11,"type":"fetch","status":"running","payload":{"scope":"all"},"error_msg":"","not_before":"","started_at":"2026-04-15T08:00:00Z","finished_at":"","created_at":"2026-04-15T08:00:00Z","updated_at":"2026-04-15T08:00:00Z"}
```

#### `video.changed`

- `data` 直接是视频对象，字段与 `GET /videos` 单项基本一致。
- 当前存在一个实现例外：下载命中永久失败时，worker 仍可能发布
  `state = "FAILED"` 的 `video.changed` 事件。
- 这个 `FAILED` 值属于当前 SSE 运行时例外，不表示 `GET /videos` 的公开状态
  基线已经扩展。

#### `creator.changed`

- `data` 当前为博主摘要：

```json
{
  "id": 1,
  "uid": "123456",
  "name": "示例博主",
  "platform": "bilibili",
  "status": "active"
}
```

#### `storage.changed`

- `data` 当前为存储面板快照，字段与 `GET /storage/stats` 一致。

#### `system.changed`

- `data` 当前只推送 `cookie` 与 `risk` 两块增量：
- 当前事件 payload 同样由 `map` 直接编码；空时间字段可能保留为空字符串。

```json
{
  "cookie": {
    "configured": true,
    "is_login": true,
    "mid": 10001,
    "uname": "tester",
    "status": "valid",
    "source": "config",
    "last_check_at": "2026-04-15T08:00:00Z",
    "last_check_result": "valid",
    "last_reload_at": "2026-04-15T07:50:00Z",
    "last_reload_result": "no_change",
    "last_error": ""
  },
  "risk": {
    "level": "低",
    "active": false,
    "backoff_until": "",
    "backoff_seconds": 0,
    "last_hit_at": "",
    "last_reason": ""
  }
}
```

## 9. 常见状态码

- `200`：请求成功。
- `204`：无响应体的成功返回，例如删除与 `OPTIONS` 预检。
- `400`：参数错误、ID 非法、请求体非法或状态流转不允许。
- `404`：资源不存在。
- `405`：请求方法不支持。
- `500`：内部错误。
- `503`：依赖服务尚未就绪。
