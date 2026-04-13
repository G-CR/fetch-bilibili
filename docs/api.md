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

## 3. 任务

### 3.1 `POST /jobs`
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

- 响应示例：

```json
{
  "status": "queued",
  "type": "fetch"
}
```

### 3.2 `GET /jobs`
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

## 4. 视频

### 4.1 `GET /videos`
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

### 4.2 `GET /videos/{id}`
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

### 4.3 `POST /videos/{id}/download`
- 用途：手动为指定视频创建下载任务

- 响应示例：

```json
{
  "status": "queued",
  "type": "download",
  "video_id": 21
}
```

### 4.4 `POST /videos/{id}/check`
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

## 5. 系统状态

### 5.1 `GET /system/status`
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
    "source": "cookie_file",
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

## 6. 存储统计

### 6.1 `GET /storage/stats`
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

### 6.2 `POST /storage/cleanup`
- 用途：手动触发一次 cleanup 任务

- 响应示例：

```json
{
  "status": "queued",
  "type": "cleanup"
}
```

## 7. 错误响应

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

## 8. 说明

- 当前前端驾驶舱已真实对接：
  - `GET /creators`
  - `POST /creators`
  - `POST /jobs`
  - `GET /jobs`
  - `GET /videos`
  - `GET /system/status`
  - `GET /storage/stats`
- 当前后端已额外实现：
  - `PATCH /creators/{id}`
  - `GET /videos/{id}`
  - `POST /videos/{id}/download`
  - `POST /videos/{id}/check`
  - `POST /storage/cleanup`
