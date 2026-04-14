# B 站适配器（当前实现）

## 1. 说明
当前适配器已接入 B 站公开接口，实现了真实拉取与可用性检查。
如需更高权限或更稳定的访问，可在后续加入 Cookie 与风控处理。

## 2. 当前行为
- `ListVideos(uid)`：调用 `x/space/wbi/arc/search` 获取投稿列表（带 WBI 签名）。
- `CheckAvailable(videoID)`：调用 `x/web-interface/view` 判断视频可访问性。
- `ResolveUID(name)`：调用 `x/web-interface/search/type` 解析名称并缓存。
- 内置 WBI key 缓存（默认 12 小时）。
- 如果配置 `cookie` / `sessdata`，会在请求头带上 Cookie。
- `cookie` 支持直接写完整 Cookie；`sessdata` 仅填写 token 时会自动拼成 `SESSDATA=...`。
- 内置 Cookie 有效性检查（由 `auth_check_interval` 控制）。

## 3. 接口说明
- 投稿列表：
  - `GET https://api.bilibili.com/x/space/wbi/arc/search`
  - 参数：`mid`、`pn`、`ps`、`order` + `wts` + `w_rid`
- 可用性检查：
  - `GET https://api.bilibili.com/x/web-interface/view?bvid=...` 或 `aid=...`
- 登录状态检查：
  - `GET https://api.bilibili.com/x/web-interface/nav`
- 名称解析：
  - `GET https://api.bilibili.com/x/web-interface/search/type?search_type=bili_user&keyword=...`

## 4. 风险与说明
- 若返回 -403/-412 可能是访问受限或触发风控，当前会作为错误返回，避免误判为下架。
- 若返回 -404/62002，视为视频不可访问（下架/不可见）。

## 5. 未来优化
- 支持分页拉取全量视频。
- 加入限速、重试与错误分类统计。
