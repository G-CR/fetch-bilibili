# B 站适配器（当前实现）

本文档只描述仓库当前已经接入并被实际调用的 B 站能力，不讨论未来平台扩展。

## 1. 适用范围

- 当前适配器实现位于 `internal/platform/bilibili`。
- 它是仓库内唯一的外部平台客户端，当前被以下链路复用：
  - 博主投稿拉取
  - 视频可访问性检查
  - 视频下载
  - Cookie 有效性检查
  - 名称解析与 UID 反查
  - 候选池关键词发现与一跳关系扩散
- 当前仓库只支持 B 站，不存在多平台适配层。

## 2. 当前能力

### 2.1 投稿拉取

- `ListVideos(uid)` 通过 `GET /x/space/wbi/arc/search` 拉取博主投稿列表。
- 请求会附带 WBI 签名；签名 key 通过 `GET /x/web-interface/nav` 获取，并在内存
  中缓存 12 小时。
- 当前只拉取第 1 页，并按发布时间倒序请求。
- 单次拉取条数由 `bilibili.fetch_page_size` 控制。
- 返回结果当前会映射为：
  - `video_id`
  - `title`
  - `description`
  - `publish_time`
  - `duration`
  - `cover_url`
  - `view_count`
  - `favorite_count`

### 2.2 视频可访问性检查

- `CheckAvailable(videoID)` 通过 `GET /x/web-interface/view` 判断视频是否仍可访问。
- 当前同时支持：
  - `BV...`
  - `av...`
  - 纯数字 aid
- 返回语义如下：
  - `code = 0`：返回可访问。
  - `code = -404` 或 `62002`：返回不可访问，不当作系统错误。
  - `code = -403` 或 `-412`：视为访问受限或触发风控，返回错误并记录退避状态。
  - 其他非 `0` 返回码：按检查失败处理。

### 2.3 视频下载

- `Download(videoID, dst)` 当前会先请求：
  1. `GET /x/web-interface/view` 获取 `cid`
  2. `GET /x/player/playurl` 获取播放地址
- 请求播放地址时当前固定携带：
  - `qn=80`
  - `fnver=0`
  - `fnval=4048`
  - `fourk=1`
- 若返回 DASH 音视频流，适配器会：
  1. 分别下载视频流与音频流
  2. 调用本机 `ffmpeg` 合并
- 若没有 DASH 流，则退回到 `durl` 直链下载。
- 当前下载链路只在同一播放计划内尝试切换备用 URL，不包含统一的 HTTP 重试器。
- 若本机缺少 `ffmpeg`，DASH 合并会直接失败。
- 当前以下播放地址错误码会被视为不可恢复错误，并返回 `PermanentError`：
  - `87008`
  - `-404`
  - `62002`
  - `-10403`

### 2.4 名称解析与 UID 反查

- `ResolveUID(name)` 通过 `GET /x/web-interface/search/type` 做作者搜索。
- 若传入值本身就是纯数字，当前直接视为 UID 返回，不再请求 B 站。
- 名称解析结果会按 `bilibili.resolve_name_cache_ttl` 做内存缓存。
- `ResolveName(uid)` 通过 `GET /x/space/wbi/acc/info` 反查博主昵称。
- UID 反查结果也会写入同一套内存缓存。

### 2.5 候选池搜索能力

- `SearchCreators(keyword, page, pageSize)` 通过
  `GET /x/web-interface/search/type?search_type=bili_user` 搜索作者。
- `SearchVideos(keyword, page, pageSize)` 通过
  `GET /x/web-interface/search/type?search_type=video` 搜索视频。
- `SearchRelatedVideos(...)` 当前直接复用 `SearchVideos(...)`。
- 搜索结果会做以下清洗：
  - 去掉高亮 HTML 标签
  - 反转义文本
  - 把 `//...` 头像或封面地址补成 `https://...`

### 2.6 认证状态与运行时快照

- 当前适配器支持两种认证输入：
  - `bilibili.cookie`
  - `bilibili.sessdata`
- 优先级为 `cookie` 高于 `sessdata`；若只提供 `sessdata`，会自动拼成
  `SESSDATA=...` Cookie 头。
- `CheckAuth()` 通过 `GET /x/web-interface/nav` 检查当前是否已登录，并更新运
  行时状态：
  - 最近检查时间
  - 最近检查结果
  - `mid`
  - `uname`
  - 最近错误摘要
- `RuntimeStatus()` 会暴露 Cookie 与风控的当前运行态，供驾驶舱汇总使用。
- 当前 `ReloadAuth()` 仍是空实现：会记录一次 `no_change`，但不会从外部文件
  或其他来源热更新 Cookie。
- 因此，当前 `bilibili.auth_reload_interval` 只驱动周期性“检查是否变化”的
  动作，不代表已经支持独立的 Cookie 热加载链路。

## 3. 请求行为与公共约束

- 所有请求当前都会带：
  - `User-Agent`
  - `Referer: https://www.bilibili.com`
  - 已配置的 `Cookie`（如有）
- 请求超时时间由 `bilibili.request_timeout` 控制。
- 当前没有额外代理池、账号池或多 Cookie 轮换逻辑。
- 当前请求失败后不会统一自动重试；能否继续主要取决于：
  - 上游是否返回可恢复错误
  - 风控退避是否结束
  - 下载链路是否还有备用 URL

## 4. 风控与退避

- 当前以下情况会被视为风控命中，并写入运行时风险状态：
  - HTTP `403`
  - HTTP `412`
  - 业务返回码 `-403`
  - 业务返回码 `-412`
- 风控命中后，适配器会按配置执行指数退避：
  - `bilibili.risk_backoff_base`
  - `bilibili.risk_backoff_max`
  - `bilibili.risk_backoff_jitter`
- 退避期间，新请求会先等待到 `risk_until` 再继续发出。
- 一旦请求成功，当前风险退避会被清零。
- 若已设置事件发布器，Cookie 或风险状态变化时会发布 `system.changed`
  增量事件。

## 5. 当前限制

- 当前只拉取投稿列表第 1 页，不会在适配器层主动翻完整历史页。
- 当前没有独立的 Cookie 文件加载、远程凭据中心或多来源轮换。
- 当前没有统一请求重试、熔断或代理切换。
- 风控状态当前只按“是否处于退避窗口”暴露高低，不细分更复杂等级。
- 适配器当前只服务仓库内 B 站场景，不抽象成通用多平台接口。
