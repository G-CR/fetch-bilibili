# API 文档

当前后端已经实现的 HTTP 接口如下。

## 1. 健康检查

### 1.1 `GET /healthz`
- 用途：基础存活检查
- 响应：

```text
ok
```

### 1.2 `GET /readyz`
- 用途：基础就绪检查
- 响应：

```text
ready
```

## 2. 博主管理

### 2.1 `POST /creators`
- 用途：新增或更新博主
- 请求体：

```json
{
  "uid": "123456",
  "name": "示例博主",
  "platform": "bilibili",
  "status": "active"
}
```

- 说明：
  - `uid` 和 `name` 至少提供一个
  - `platform` 当前建议使用 `bilibili`
  - `status` 默认 `active`

- 响应示例：

```json
{
  "id": 1,
  "uid": "123456",
  "name": "示例博主",
  "platform": "bilibili",
  "status": "active"
}
```

### 2.2 `GET /creators?limit=200`
- 用途：获取当前启用中的博主列表
- 参数：
  - `limit`：返回条数，默认 `200`

- 响应示例：

```json
{
  "items": [
    {
      "id": 1,
      "uid": "123456",
      "name": "示例博主",
      "platform": "bilibili",
      "status": "active"
    }
  ]
}
```

### 2.3 `PATCH /creators/{id}`
- 用途：更新单个博主的可运维字段
- 当前支持字段：
  - `name`
  - `status`

- 请求示例：

```json
{
  "name": "新的博主名称",
  "status": "paused"
}
```

- 响应示例：

```json
{
  "id": 1,
  "uid": "123456",
  "name": "新的博主名称",
  "platform": "bilibili",
  "status": "paused"
}
```

### 2.4 `DELETE /creators/{id}`
- 用途：手工移除单个博主
- 说明：
  - 该接口会将博主标记为 `removed`，不会直接物理删除归档数据。
  - 被手工移除的博主不会再被 `creators.file` 自动同步恢复。
  - 如需恢复，可再次调用 `POST /creators` 添加相同 UID。

- 成功响应：

```text
204 No Content
```

- 失败响应示例：

```json
{
  "error": "博主不存在"
}
```

## 3. 候选池（自动发现 + 人工审核）

说明：
- 当前一期只支持 `bilibili`
- 候选池不会自动转正，必须人工审核
- `approve` 成功后，如开启配置项，系统只会为该博主创建一次定向 fetch，不会触发全量 fetch

### 3.1 `GET /candidate-creators`
- 用途：查询候选博主列表
- 参数：
  - `status`：按候选状态过滤，可选 `reviewing / ignored / blocked / approved`
  - `min_score`：按最低评分过滤
  - `keyword`：按名称 / UID / 来源标签模糊过滤
  - `page`：页码，默认 `1`
  - `page_size`：每页条数，默认 `20`

- 示例：

```text
GET /candidate-creators?status=reviewing&min_score=80&keyword=补档&page=1&page_size=20
```

- 响应示例：

```json
{
  "items": [
    {
      "id": 301,
      "platform": "bilibili",
      "uid": "9001",
      "name": "候选补档站",
      "profile_url": "https://space.bilibili.com/9001",
      "follower_count": 321000,
      "status": "reviewing",
      "score": 88,
      "score_version": "v1",
      "last_discovered_at": "2026-04-20T12:00:00Z",
      "sources": [
        {
          "id": 1,
          "source_type": "keyword",
          "source_value": "补档",
          "source_label": "关键词：补档",
          "weight": 15
        }
      ]
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 20
}
```

### 3.2 `GET /candidate-creators/{id}`
- 用途：查询单个候选详情，返回来源列表与评分明细

- 响应示例：

```json
{
  "candidate": {
    "id": 301,
    "platform": "bilibili",
    "uid": "9001",
    "name": "候选补档站",
    "status": "reviewing",
    "score": 88
  },
  "sources": [
    {
      "id": 1,
      "source_type": "keyword",
      "source_value": "补档",
      "source_label": "关键词：补档",
      "weight": 15,
      "detail_json": {
        "keyword": "补档",
        "videos": []
      }
    },
    {
      "id": 2,
      "source_type": "related_creator",
      "source_value": "1001",
      "source_label": "关联博主：已追踪 A",
      "weight": 10
    }
  ],
  "score_details": [
    {
      "id": 11,
      "factor_key": "keyword_risk",
      "factor_label": "命中高风险关键词",
      "score_delta": 30
    },
    {
      "id": 12,
      "factor_key": "similarity",
      "factor_label": "与已追踪池内容相似",
      "score_delta": 10
    }
  ]
}
```

### 3.3 `POST /candidate-creators/discover`
- 用途：手动触发一次 discover 任务

- 响应示例：

```json
{
  "status": "queued",
  "type": "discover"
}
```

### 3.4 `POST /candidate-creators/{id}/approve`
- 用途：批准候选并转正为正式追踪博主

- 响应示例：

```json
{
  "id": 21,
  "uid": "9001",
  "name": "候选补档站",
  "platform": "bilibili",
  "status": "active"
}
```

### 3.5 `POST /candidate-creators/{id}/ignore`
### 3.6 `POST /candidate-creators/{id}/block`
### 3.7 `POST /candidate-creators/{id}/review`
- 用途：
  - `ignore`：将候选标记为已忽略
  - `block`：将候选标记为已拉黑
  - `review`：将已忽略候选恢复为审核中

- 响应示例：

```json
{
  "status": "ok",
  "action": "ignore",
  "candidate_id": 301
}
```

## 4. 任务

### 4.1 `POST /jobs`
- 用途：手动触发任务
- 请求体：

```json
{
  "type": "fetch"
}
```

- `type` 支持：
  - `fetch`
  - `check`
  - `cleanup`
  - `discover`

- 响应示例：

```json
{
  "status": "queued",
  "type": "fetch"
}
```

### 4.2 `GET /jobs`
- 用途：查询最近任务
- 参数：
  - `limit`：返回条数，默认 `20`
  - `status`：按状态过滤
  - `type`：按任务类型过滤

- 示例：

```text
GET /jobs?limit=10&status=queued&type=fetch
```

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

## 5. 视频

### 5.1 `GET /videos`
- 用途：查询最近视频
- 参数：
  - `limit`：返回条数，默认 `20`
  - `creator_id`：按博主 ID 过滤
  - `state`：按视频状态过滤

- 示例：

```text
GET /videos?limit=10&creator_id=1&state=OUT_OF_PRINT
```

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

### 5.2 `GET /videos/{id}`
- 用途：查询单个视频详情

- 响应示例：

```json
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
```

### 5.3 `POST /videos/{id}/download`
- 用途：手动为指定视频创建下载任务

- 响应示例：

```json
{
  "status": "queued",
  "type": "download",
  "video_id": 21
}
```

### 5.4 `POST /videos/{id}/check`
- 用途：手动为指定视频创建检查任务
- 说明：
  - 当前实现会复用 `check` 任务类型，并附带 `video_id` payload
  - worker 收到该任务后只检查目标视频

- 响应示例：

```json
{
  "status": "queued",
  "type": "check",
  "video_id": 21
}
```

## 6. 系统状态

### 6.1 `GET /system/status`
- 用途：提供驾驶舱总览状态
- 包含：
  - MySQL 连通状态
  - Cookie 状态、来源、最近检查/刷新结果
  - 认证监控是否启用
  - 风控退避是否生效、剩余秒数、最近命中原因
  - 活跃博主数 / 待处理任务数 / 绝版视频数
  - 当前限速配置
  - 调度周期
  - 存储根目录

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
    "last_reload_result": "success",
    "last_error": "上次刷新失败"
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
  "risk_level": "低",
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

## 7. 存储统计

### 7.1 `GET /storage/stats`
- 用途：提供驾驶舱存储面板数据
- 包含：
  - 实际已用字节数
  - 容量上限与安全阈值
  - 当前文件数
  - 热点目录
  - 绝版视频数量
  - 当前清理规则说明

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

### 7.2 `POST /storage/cleanup`
- 用途：手动触发一次 cleanup 任务

- 响应示例：

```json
{
  "status": "queued",
  "type": "cleanup"
}
```

## 8. 系统配置

### 8.1 `GET /system/config`
- 用途：读取当前运行配置文件路径与全文内容

- 响应示例：

```text
{
  "path": "/app/config.yaml",
  "content": "server:\n  addr: \":8080\"\n"
}
```

### 8.2 `PUT /system/config`
- 用途：保存新的配置文件内容；如果内容发生变化，服务会在写回成功后安排重启
- 说明：
  - 请求体必须是完整配置文本，而不是局部字段补丁
  - 服务会先执行 YAML 解析与业务配置校验
  - `changed=false` 表示内容未变化，不会触发重启

- 请求示例：

```text
{
  "content": "server:\n  addr: \":8080\"\nstorage:\n  root_dir: /data/bilibili\nmysql:\n  dsn: test\n"
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

## 9. 实时事件流

### 9.1 `GET /events/stream`
- 用途：为前端驾驶舱提供实时增量事件
- 协议：SSE（`text/event-stream`）
- 建议接入方式：
  - 首屏先请求 `GET /creators`、`GET /jobs`、`GET /videos`、`GET /system/status`、`GET /storage/stats`
  - 再建立 `EventSource`
  - 断线重连成功后补一次快照，修正断线期间的状态漂移
- 服务端行为：
  - 建连后立即发送 `hello`
  - 默认每 15 秒发送一次 `heartbeat`
  - 状态变化时按事件类型推送 JSON 数据

- 当前事件类型：
  - `hello`
  - `heartbeat`
  - `job.changed`
  - `video.changed`
  - `creator.changed`
  - `storage.changed`
  - `system.changed`

- SSE 帧示例：

```text
event: job.changed
data: {"job":{"id":11,"type":"fetch","status":"running","updated_at":"2026-04-15T08:00:00Z"}}
```

- 说明：
  - `job.changed`：任务状态变化，通常包含 `job`
  - `video.changed`：视频状态或元数据变化，通常包含 `video`
  - `creator.changed`：博主新增、暂停、启用、停止追踪、名称更新
  - `storage.changed`：存储统计变化，通常包含 `storage`
  - `system.changed`：系统运行态增量，当前主要用于推送 `cookie` 和 `risk`；完整系统状态仍以 `GET /system/status` 为准

## 10. 错误响应

接口错误统一返回：

```json
{
  "error": "错误原因"
}
```

常见状态码：
- `400`：参数错误或请求体非法
- `404`：资源不存在
- `405`：请求方法不支持
- `500`：服务内部错误
- `503`：服务依赖尚未就绪

## 11. 说明

- 当前前端驾驶舱已真实对接：
  - `GET /creators`
  - `POST /creators`
  - `PATCH /creators/{id}`
  - `DELETE /creators/{id}`
  - `POST /jobs`
  - `GET /jobs`
  - `GET /videos`
  - `GET /videos/{id}`
  - `POST /videos/{id}/download`
  - `POST /videos/{id}/check`
  - `GET /system/status`
  - `GET /storage/stats`
  - `POST /storage/cleanup`
  - `GET /system/config`
  - `PUT /system/config`
  - `GET /events/stream`
  - `GET /candidate-creators`
  - `GET /candidate-creators/{id}`
  - `POST /candidate-creators/discover`
  - `POST /candidate-creators/{id}/approve`
  - `POST /candidate-creators/{id}/ignore`
  - `POST /candidate-creators/{id}/block`
  - `POST /candidate-creators/{id}/review`
