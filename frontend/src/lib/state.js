const STORAGE_KEY = "bili-vault-dashboard-v3";
const GIB = 1024 ** 3;

export function createDefaultState() {
  return {
    mode: "api",
    apiBase: "http://localhost:8080",
    creators: [
      { id: 101, uid: "123456", name: "示例博主", platform: "bilibili", status: "active" },
      { id: 102, uid: "987654", name: "科技区UP主", platform: "bilibili", status: "active" },
      { id: 103, uid: "456789", name: "收藏向频道", platform: "bilibili", status: "paused" }
    ],
    videos: [
      {
        id: 1,
        title: "年度合集",
        videoId: "BV1demo001",
        state: "OUT_OF_PRINT",
        publishTime: "",
        viewCount: 188000,
        favoriteCount: 2100
      },
      {
        id: 2,
        title: "幕后花絮",
        videoId: "BV1demo002",
        state: "STABLE",
        publishTime: "",
        viewCount: 86000,
        favoriteCount: 980
      },
      {
        id: 3,
        title: "新作预告",
        videoId: "BV1demo003",
        state: "DOWNLOADING",
        publishTime: "",
        viewCount: 56000,
        favoriteCount: 520
      }
    ],
    jobs: [
      { id: 1, type: "fetch", status: "success", origin: "scheduler" },
      { id: 2, type: "download", status: "running", origin: "worker" },
      { id: 3, type: "check", status: "queued", origin: "manual" }
    ],
    logs: [
      makeLog("系统已加载默认面板数据"),
      makeLog("已启用本地 / API 双模式切换"),
      makeLog("等待下一次后端同步")
    ],
    storage: {
      usedBytes: 268 * GIB,
      limitBytes: 2048 * GIB,
      safeBytes: 1843 * GIB,
      hottestBucket: "绝版视频",
      cleanupRule: "绝版优先 -> 粉丝量 -> 播放量 -> 收藏量",
      fileCount: 426,
      usagePercent: 13,
      rareVideos: 1,
      rootDir: "/data/archive"
    },
    limits: {
      globalQps: 2,
      perCreatorQps: 1,
      downloadConcurrency: 4,
      checkConcurrency: 8
    },
    scheduler: {
      fetchInterval: "45m0s",
      checkInterval: "24h0m0s",
      cleanupInterval: "24h0m0s",
      stableDays: 30
    },
    system: {
      health: "online",
      lastSyncAt: "等待同步",
      activeJobs: 2,
      authEnabled: false,
      riskLevel: "低",
      riskActive: false,
      riskBackoffUntil: "",
      riskBackoffSeconds: 0,
      riskLastHitAt: "",
      riskLastReason: "",
      mysqlOK: true,
      cookieStatus: "not_configured",
      cookieConfigured: false,
      cookieSource: "",
      cookieUname: "",
      cookieMid: 0,
      cookieLastCheckAt: "",
      cookieLastCheckResult: "",
      cookieLastReloadAt: "",
      cookieLastReloadResult: "",
      cookieLastError: "",
      lastJobAt: "",
      storageRoot: "/data/archive",
      overview: {
        activeCreators: 3,
        pendingJobs: 2,
        rareVideos: 1
      }
    }
  };
}

export function loadState() {
  const defaults = createDefaultState();
  if (typeof window === "undefined") {
    return defaults;
  }

  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) {
      return defaults;
    }
    const parsed = JSON.parse(raw);
    return {
      ...defaults,
      ...parsed,
      creators: Array.isArray(parsed?.creators) ? parsed.creators : defaults.creators,
      videos: Array.isArray(parsed?.videos) ? parsed.videos : defaults.videos,
      jobs: Array.isArray(parsed?.jobs) ? parsed.jobs : defaults.jobs,
      logs: Array.isArray(parsed?.logs) ? parsed.logs : defaults.logs,
      storage: {
        ...defaults.storage,
        ...(parsed?.storage || {})
      },
      limits: {
        ...defaults.limits,
        ...(parsed?.limits || {})
      },
      scheduler: {
        ...defaults.scheduler,
        ...(parsed?.scheduler || {})
      },
      system: {
        ...defaults.system,
        ...(parsed?.system || {}),
        overview: {
          ...defaults.system.overview,
          ...(parsed?.system?.overview || {})
        }
      }
    };
  } catch (_error) {
    return defaults;
  }
}

export function saveState(state) {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
}

export function applyRemoteSnapshot(previous, snapshot, lastSyncAt = formatNow()) {
  const creators = normalizeCreators(snapshot?.creators);
  const jobs = normalizeJobs(snapshot?.jobs);
  const videos = normalizeVideos(snapshot?.videos);
  const storage = normalizeStorage(snapshot?.storage, snapshot?.system, previous?.storage);
  const limits = normalizeLimits(snapshot?.system?.limits, previous?.limits);
  const scheduler = normalizeScheduler(snapshot?.system?.scheduler, previous?.scheduler);
  const overview = normalizeOverview(snapshot?.system?.overview, creators, jobs, videos, storage, previous?.system?.overview);

  return {
    ...previous,
    creators,
    jobs,
    videos,
    storage,
    limits,
    scheduler,
    system: {
      ...previous.system,
      health: stringOr(snapshot?.system?.health, previous?.system?.health, "unknown"),
      lastSyncAt,
      activeJobs: numberOr(snapshot?.system?.active_jobs, overview.pendingJobs),
      authEnabled: booleanOr(snapshot?.system?.auth_enabled, previous?.system?.authEnabled, false),
      riskLevel: stringOr(snapshot?.system?.risk_level, previous?.system?.riskLevel, "未知"),
      riskActive: booleanOr(snapshot?.system?.risk?.active, previous?.system?.riskActive, false),
      riskBackoffUntil: stringOr(snapshot?.system?.risk?.backoff_until, previous?.system?.riskBackoffUntil, ""),
      riskBackoffSeconds: numberOr(snapshot?.system?.risk?.backoff_seconds, previous?.system?.riskBackoffSeconds, 0),
      riskLastHitAt: stringOr(snapshot?.system?.risk?.last_hit_at, previous?.system?.riskLastHitAt, ""),
      riskLastReason: stringOr(snapshot?.system?.risk?.last_reason, previous?.system?.riskLastReason, ""),
      mysqlOK: booleanOr(snapshot?.system?.mysql_ok, previous?.system?.mysqlOK, false),
      cookieStatus: stringOr(snapshot?.system?.cookie?.status, previous?.system?.cookieStatus, "unknown"),
      cookieConfigured: booleanOr(snapshot?.system?.cookie?.configured, previous?.system?.cookieConfigured, false),
      cookieSource: stringOr(snapshot?.system?.cookie?.source, previous?.system?.cookieSource, ""),
      cookieUname: stringOr(snapshot?.system?.cookie?.uname, previous?.system?.cookieUname, ""),
      cookieMid: numberOr(snapshot?.system?.cookie?.mid, previous?.system?.cookieMid, 0),
      cookieLastCheckAt: stringOr(snapshot?.system?.cookie?.last_check_at, previous?.system?.cookieLastCheckAt, ""),
      cookieLastCheckResult: stringOr(snapshot?.system?.cookie?.last_check_result, previous?.system?.cookieLastCheckResult, ""),
      cookieLastReloadAt: stringOr(snapshot?.system?.cookie?.last_reload_at, previous?.system?.cookieLastReloadAt, ""),
      cookieLastReloadResult: stringOr(snapshot?.system?.cookie?.last_reload_result, previous?.system?.cookieLastReloadResult, ""),
      cookieLastError: stringOr(snapshot?.system?.cookie?.last_error, previous?.system?.cookieLastError, ""),
      lastJobAt: stringOr(snapshot?.system?.last_job_at, previous?.system?.lastJobAt, ""),
      storageRoot: stringOr(snapshot?.system?.storage_root, storage.rootDir, previous?.system?.storageRoot, "-"),
      overview
    }
  };
}

export function deriveMetrics(state) {
  const creators = Array.isArray(state?.creators) ? state.creators : [];
  const videos = Array.isArray(state?.videos) ? state.videos : [];
  const jobs = Array.isArray(state?.jobs) ? state.jobs : [];
  const storage = state?.storage || {};
  const overview = state?.system?.overview || {};

  return {
    creators: numberOr(overview.activeCreators, creators.length),
    pendingJobs: numberOr(
      overview.pendingJobs,
      jobs.filter((job) => job.status === "queued" || job.status === "running").length
    ),
    rareVideos: numberOr(
      overview.rareVideos,
      videos.filter((video) => video.state === "OUT_OF_PRINT").length
    ),
    storagePercent: numberOr(
      storage.usagePercent,
      Math.min(100, Math.round((numberOr(storage.usedBytes, 0) * 100) / Math.max(numberOr(storage.limitBytes, 1), 1)))
    )
  };
}

export function deriveTaskDiagnostics(state) {
  const jobs = Array.isArray(state?.jobs) ? state.jobs : [];
  const failed = jobs.filter((job) => job?.status === "failed");

  return {
    queuedCount: jobs.filter((job) => job?.status === "queued").length,
    runningCount: jobs.filter((job) => job?.status === "running").length,
    failedCount: failed.length,
    latestFailure: [...failed].sort((left, right) => sortByTime(right) - sortByTime(left))[0] || null
  };
}

export function deriveCleanupPreview(state, limit = 5) {
  const videos = Array.isArray(state?.videos) ? state.videos : [];

  return videos
    .filter((video) => isCleanupPreviewCandidate(video))
    .map((video) => ({
      ...video,
      protected: video?.state === "OUT_OF_PRINT",
      reasons: buildCleanupReasons(video),
      sortKey: [
        cleanupProtectedRank(video),
        Number(video?.viewCount) || 0,
        Number(video?.favoriteCount) || 0
      ]
    }))
    .sort((left, right) => compareSortKey(left.sortKey, right.sortKey))
    .slice(0, Math.max(Number(limit) || 0, 0))
    .map(({ sortKey, ...video }) => video);
}

export function makeLog(message) {
  return {
    id: Date.now() + Math.random(),
    at: new Date().toLocaleString("zh-CN", { hour12: false }),
    message
  };
}

export function nextJobId(jobs) {
  const max = (jobs || []).reduce((current, job) => Math.max(current, Number(job.id) || 0), 0);
  return max + 1;
}

function normalizeCreators(items) {
  if (!Array.isArray(items)) {
    return [];
  }
  return items.map((item) => ({
    id: numberOr(item?.id, 0),
    uid: stringOr(item?.uid, ""),
    name: stringOr(item?.name, ""),
    platform: stringOr(item?.platform, "bilibili"),
    status: stringOr(item?.status, "active")
  }));
}

function normalizeJobs(items) {
  if (!Array.isArray(items)) {
    return [];
  }
  return items.map((item) => ({
    id: numberOr(item?.id, 0),
    type: stringOr(item?.type, ""),
    status: stringOr(item?.status, "queued"),
    payload: objectOr(item?.payload, {}),
    errorMsg: stringOr(item?.error_msg, ""),
    createdAt: stringOr(item?.created_at, ""),
    updatedAt: stringOr(item?.updated_at, ""),
    startedAt: stringOr(item?.started_at, ""),
    finishedAt: stringOr(item?.finished_at, ""),
    origin: stringOr(item?.payload?.origin, item?.payload?.source, "remote")
  }));
}

function normalizeVideos(items) {
  if (!Array.isArray(items)) {
    return [];
  }
  return items.map((item) => ({
    id: numberOr(item?.id, 0),
    platform: stringOr(item?.platform, "bilibili"),
    videoId: stringOr(item?.video_id, ""),
    creatorId: numberOr(item?.creator_id, 0),
    title: stringOr(item?.title, ""),
    description: stringOr(item?.description, ""),
    publishTime: stringOr(item?.publish_time, ""),
    duration: numberOr(item?.duration, 0),
    coverUrl: stringOr(item?.cover_url, ""),
    viewCount: numberOr(item?.view_count, 0),
    favoriteCount: numberOr(item?.favorite_count, 0),
    state: stringOr(item?.state, "UNKNOWN"),
    outOfPrintAt: stringOr(item?.out_of_print_at, ""),
    stableAt: stringOr(item?.stable_at, ""),
    lastCheckAt: stringOr(item?.last_check_at, "")
  }));
}

function normalizeStorage(storage, system, previous) {
  return {
    usedBytes: numberOr(storage?.used_bytes, previous?.usedBytes, 0),
    limitBytes: numberOr(storage?.max_bytes, previous?.limitBytes, 1),
    safeBytes: numberOr(storage?.safe_bytes, previous?.safeBytes, 0),
    hottestBucket: stringOr(storage?.hottest_bucket, previous?.hottestBucket, "-"),
    cleanupRule: stringOr(storage?.cleanup_rule, previous?.cleanupRule, "绝版优先 -> 粉丝量 -> 播放量 -> 收藏量"),
    fileCount: numberOr(storage?.file_count, previous?.fileCount, 0),
    usagePercent: numberOr(storage?.usage_percent, previous?.usagePercent, 0),
    rareVideos: numberOr(storage?.rare_videos, system?.overview?.rare_videos, previous?.rareVideos, 0),
    rootDir: stringOr(storage?.root_dir, system?.storage_root, previous?.rootDir, "-")
  };
}

function normalizeLimits(limits, previous) {
  return {
    globalQps: numberOr(limits?.global_qps, previous?.globalQps, 0),
    perCreatorQps: numberOr(limits?.per_creator_qps, previous?.perCreatorQps, 0),
    downloadConcurrency: numberOr(limits?.download_concurrency, previous?.downloadConcurrency, 0),
    checkConcurrency: numberOr(limits?.check_concurrency, previous?.checkConcurrency, 0)
  };
}

function normalizeScheduler(scheduler, previous) {
  return {
    fetchInterval: stringOr(scheduler?.fetch_interval, previous?.fetchInterval, "-"),
    checkInterval: stringOr(scheduler?.check_interval, previous?.checkInterval, "-"),
    cleanupInterval: stringOr(scheduler?.cleanup_interval, previous?.cleanupInterval, "-"),
    stableDays: numberOr(scheduler?.check_stable_days, previous?.stableDays, 30)
  };
}

function normalizeOverview(overview, creators, jobs, videos, storage, previous) {
  return {
    activeCreators: numberOr(overview?.active_creators, creators.length, previous?.activeCreators, 0),
    pendingJobs: numberOr(
      overview?.pending_jobs,
      jobs.filter((job) => job.status === "queued" || job.status === "running").length,
      previous?.pendingJobs,
      0
    ),
    rareVideos: numberOr(
      overview?.rare_videos,
      storage?.rareVideos,
      videos.filter((video) => video.state === "OUT_OF_PRINT").length,
      previous?.rareVideos,
      0
    )
  };
}

function numberOr(...values) {
  for (const value of values) {
    if (typeof value === "number" && Number.isFinite(value)) {
      return value;
    }
  }
  return 0;
}

function stringOr(...values) {
  for (const value of values) {
    if (typeof value === "string" && value.trim() !== "") {
      return value;
    }
  }
  return "";
}

function booleanOr(...values) {
  for (const value of values) {
    if (typeof value === "boolean") {
      return value;
    }
  }
  return false;
}

function objectOr(value, fallback) {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    return value;
  }
  return fallback;
}

function isCleanupPreviewCandidate(video) {
  const state = String(video?.state || "").toUpperCase();
  if (!state || state === "OUT_OF_PRINT") {
    return false;
  }
  return state !== "DOWNLOADING";
}

function buildCleanupReasons(video) {
  return [
    video?.state === "OUT_OF_PRINT" ? "绝版保护" : "非绝版",
    `播放 ${numberOr(video?.viewCount, 0)}`,
    `收藏 ${numberOr(video?.favoriteCount, 0)}`
  ];
}

function cleanupProtectedRank(video) {
  return String(video?.state || "").toUpperCase() === "OUT_OF_PRINT" ? 1 : 0;
}

function compareSortKey(left, right) {
  for (let index = 0; index < left.length; index += 1) {
    const diff = (left[index] || 0) - (right[index] || 0);
    if (diff !== 0) {
      return diff;
    }
  }
  return 0;
}

function sortByTime(job) {
  return (
    Date.parse(job?.updatedAt || "") ||
    Date.parse(job?.finishedAt || "") ||
    Date.parse(job?.startedAt || "") ||
    Date.parse(job?.createdAt || "") ||
    0
  );
}

function formatNow() {
  return new Date().toLocaleString("zh-CN", { hour12: false });
}
