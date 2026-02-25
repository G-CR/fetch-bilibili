# API 设计（草案）

## 1. 博主
### 1.1 添加博主
- POST /creators
- Body(JSON):
  - uid | name（二选一）
  - platform (default: bilibili)
  - status (default: active)
- Response(JSON):
```json
{
  "id": 1,
  "uid": "123456",
  "name": "示例博主",
  "platform": "bilibili",
  "status": "active"
}
```

### 1.2 列表查询
- GET /creators?status=active&platform=bilibili

### 1.3 更新/停用
- PATCH /creators/{id}
- Body: status, name

## 2. 视频
### 2.1 查询视频
- GET /videos?creator_id=&state=&from=&to=

### 2.2 视频详情
- GET /videos/{id}

### 2.3 手动触发下载
- POST /videos/{id}/download

### 2.4 手动触发检查
- POST /videos/{id}/check

## 3. 任务
### 3.1 查询任务
- GET /jobs?status=failed&type=download

## 4. 系统与存储
### 4.1 系统状态
- GET /system/status

### 4.2 存储统计
- GET /storage/stats

### 4.3 触发清理
- POST /storage/cleanup

## 5. 例子（返回结构建议）
```json
{
  "id": "v_123",
  "title": "示例视频",
  "state": "DOWNLOADED",
  "creator": {"id": "c_1", "name": "示例博主"},
  "files": [{"path": "/data/bilibili/2025/abc.mp4", "size": 123456789}]
}
```
