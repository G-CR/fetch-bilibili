import assert from "node:assert/strict";
import {
  applyRemoteSnapshot,
  createDefaultState,
  deriveCleanupPreview,
  deriveMetrics,
  deriveTaskDiagnostics
} from "../src/lib/state.js";

const previous = createDefaultState();

const next = applyRemoteSnapshot(previous, {
  creators: [
    { id: 1, uid: "123", name: "测试 UP", platform: "bilibili", status: "active" }
  ],
  jobs: [
    {
      id: 9,
      type: "fetch",
      status: "running",
      created_at: "2026-04-13T12:00:00Z"
    }
  ],
  videos: [
    {
      id: 21,
      video_id: "BV1xx411c7mD",
      title: "稀有投稿",
      state: "OUT_OF_PRINT",
      publish_time: "2026-04-12T09:30:00Z",
      view_count: 1024,
      favorite_count: 88
    }
  ],
  system: {
    health: "online",
    mysql_ok: true,
    auth_enabled: true,
    active_jobs: 1,
    risk_level: "低",
    risk: {
      level: "低",
      active: false,
      backoff_seconds: 0,
      backoff_until: "",
      last_hit_at: "2026-04-13T11:58:00Z",
      last_reason: "/x/web-interface/view 返回风控码 -412"
    },
    last_job_at: "2026-04-13T12:00:00Z",
    storage_root: "/data/archive",
    cookie: {
      configured: true,
      status: "valid",
      uname: "tester",
      source: "cookie_file",
      last_check_at: "2026-04-13T11:59:00Z",
      last_reload_at: "2026-04-13T11:50:00Z",
      last_check_result: "valid",
      last_reload_result: "success",
      last_error: "上次刷新失败"
    },
    overview: {
      active_creators: 1,
      pending_jobs: 1,
      rare_videos: 1
    },
    limits: {
      global_qps: 2,
      per_creator_qps: 1,
      download_concurrency: 4,
      check_concurrency: 8
    },
    scheduler: {
      fetch_interval: "45m0s",
      check_interval: "24h0m0s",
      cleanup_interval: "24h0m0s",
      check_stable_days: 30
    }
  },
  storage: {
    root_dir: "/data/archive",
    used_bytes: 1073741824,
    max_bytes: 2147483648,
    safe_bytes: 1610612736,
    usage_percent: 50,
    file_count: 12,
    hottest_bucket: "bilibili",
    rare_videos: 1,
    cleanup_rule: "绝版优先 -> 粉丝量 -> 播放量 -> 收藏量"
  }
}, "2026-04-13 20:00:00");

assert.equal(next.creators.length, 1);
assert.equal(next.jobs[0].status, "running");
assert.equal(next.jobs[0].createdAt, "2026-04-13T12:00:00Z");
assert.equal(next.videos[0].videoId, "BV1xx411c7mD");
assert.equal(next.system.health, "online");
assert.equal(next.system.cookieStatus, "valid");
assert.equal(next.system.authEnabled, true);
assert.equal(next.system.cookieSource, "cookie_file");
assert.equal(next.system.cookieLastCheckAt, "2026-04-13T11:59:00Z");
assert.equal(next.system.cookieLastReloadResult, "success");
assert.equal(next.system.riskActive, false);
assert.equal(next.system.riskLastReason, "/x/web-interface/view 返回风控码 -412");
assert.equal(next.system.overview.rareVideos, 1);
assert.equal(next.system.lastSyncAt, "2026-04-13 20:00:00");
assert.equal(next.storage.usedBytes, 1073741824);
assert.equal(next.storage.usagePercent, 50);
assert.equal(next.limits.downloadConcurrency, 4);
assert.equal(next.scheduler.stableDays, 30);

const metrics = deriveMetrics(next);
assert.equal(metrics.creators, 1);
assert.equal(metrics.pendingJobs, 1);
assert.equal(metrics.rareVideos, 1);
assert.equal(metrics.storagePercent, 50);

const preview = deriveCleanupPreview({
  ...createDefaultState(),
  videos: [
    { id: 1, title: "绝版保留", videoId: "BV1", state: "OUT_OF_PRINT", viewCount: 10, favoriteCount: 1 },
    { id: 2, title: "低价值视频", videoId: "BV2", state: "DOWNLOADED", viewCount: 20, favoriteCount: 2 },
    { id: 3, title: "高价值视频", videoId: "BV3", state: "DOWNLOADED", viewCount: 3000, favoriteCount: 500 }
  ]
});

assert.equal(preview.length, 2);
assert.equal(preview[0].id, 2);
assert.equal(preview[0].protected, false);
assert.equal(preview[0].reasons[0], "非绝版");
assert.equal(preview[1].id, 3);

const diagnostics = deriveTaskDiagnostics({
  ...createDefaultState(),
  jobs: [
    {
      id: 1,
      type: "check",
      status: "failed",
      errorMsg: "视频接口返回 412",
      updatedAt: "2026-04-13T12:31:00Z"
    },
    {
      id: 2,
      type: "download",
      status: "running",
      updatedAt: "2026-04-13T12:32:00Z"
    },
    {
      id: 3,
      type: "fetch",
      status: "queued",
      updatedAt: "2026-04-13T12:33:00Z"
    }
  ]
});

assert.equal(diagnostics.failedCount, 1);
assert.equal(diagnostics.runningCount, 1);
assert.equal(diagnostics.queuedCount, 1);
assert.equal(diagnostics.latestFailure?.id, 1);
assert.equal(diagnostics.latestFailure?.errorMsg, "视频接口返回 412");

const defaults = createDefaultState();
assert.equal(defaults.creators.length, 0);
assert.equal(defaults.videos.length, 0);
assert.equal(defaults.jobs.length, 0);

console.log("dashboard state ok");
