const STORAGE_KEY = "bili-vault-dashboard-v3";
const DEFAULT_LIST_PAGE_SIZE = 6;
const DEFAULT_CANDIDATE_PAGE_SIZE = 6;
const SUPPORTED_PAGE_SIZES = [6, 12, 20, 50];
const STORAGE_SCHEMA_VERSION = 5;
const CANDIDATE_PAGE_MIGRATION_VERSION = 5;
const CANDIDATE_FILTER_MIGRATION_VERSION = 5;

export function createDefaultState() {
  return {
    storageVersion: STORAGE_SCHEMA_VERSION,
    apiBase: "http://localhost:8080",
    connection: {
      status: "connecting",
      lastEventAt: "",
      lastEventType: "",
      lastError: ""
    },
    creators: [],
    videos: [],
    jobs: [],
    pagination: createDefaultPaginationState(),
    candidatePool: {
      items: [],
      total: 0,
      page: 1,
      pageSize: DEFAULT_CANDIDATE_PAGE_SIZE,
      lastSyncAt: "未同步",
      filters: {
        status: "reviewing",
        minScore: 0,
        keyword: ""
      },
      selectedID: 0,
      detail: null
    },
    logs: [
      makeLog("前端已切换为后端接口模式"),
      makeLog("等待首次同步")
    ],
    storage: {
      usedBytes: 0,
      limitBytes: 0,
      safeBytes: 0,
      hottestBucket: "",
      cleanupRule: "绝版优先 -> 粉丝量 -> 播放量 -> 收藏量",
      fileCount: 0,
      usagePercent: 0,
      rareVideos: 0,
      rootDir: ""
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
      health: "unknown",
      lastSyncAt: "未同步",
      activeJobs: 0,
      authEnabled: false,
      riskLevel: "未知",
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
      storageRoot: "",
      overview: {
        activeCreators: 0,
        pendingJobs: 0,
        rareVideos: 0
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
    const parsedVersion = Math.max(0, Math.floor(numberOr(parsed?.storageVersion, 0)));
    const { mode: _legacyMode, ...rest } = parsed || {};
    const isLegacyLocalMode = parsed?.mode === "local";
    const shouldResetCandidatePaging = parsedVersion < CANDIDATE_PAGE_MIGRATION_VERSION;
    const shouldResetCandidateFilter = parsedVersion < CANDIDATE_FILTER_MIGRATION_VERSION;
    return {
      ...defaults,
      ...rest,
      storageVersion: STORAGE_SCHEMA_VERSION,
      connection: {
        ...defaults.connection,
        ...(parsed?.connection || {})
      },
      creators: isLegacyLocalMode ? defaults.creators : Array.isArray(parsed?.creators) ? parsed.creators : defaults.creators,
      videos: isLegacyLocalMode ? defaults.videos : Array.isArray(parsed?.videos) ? parsed.videos : defaults.videos,
      jobs: isLegacyLocalMode ? defaults.jobs : Array.isArray(parsed?.jobs) ? parsed.jobs : defaults.jobs,
      pagination: normalizePaginationState(parsed?.pagination),
      candidatePool: normalizeCandidatePoolState(parsed?.candidatePool, defaults.candidatePool, {
        resetItems: isLegacyLocalMode,
        forceDefaultPageSize: shouldResetCandidatePaging,
        forceDefaultFilterStatus: shouldResetCandidateFilter
      }),
      logs: isLegacyLocalMode ? defaults.logs : Array.isArray(parsed?.logs) ? parsed.logs : defaults.logs,
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

export function applyLiveEvent(previous, event) {
  const safePrevious = previous || createDefaultState();
  const type = String(event?.type || "").trim();
  const data = event?.data || {};
  const eventAt = formatNow();

  if (!type) {
    return safePrevious;
  }

  switch (type) {
    case "hello":
    case "heartbeat":
    case "stream.live":
      return applyConnectionPatch(safePrevious, {
        status: "live",
        lastEventAt: eventAt,
        lastEventType: type,
        lastError: ""
      });
    case "stream.connecting":
      return applyConnectionPatch(safePrevious, {
        status: "connecting",
        lastEventAt: eventAt,
        lastEventType: type
      });
    case "stream.reconnecting":
      return applyConnectionPatch(safePrevious, {
        status: "reconnecting",
        lastEventAt: eventAt,
        lastEventType: type
      });
    case "stream.offline":
      return applyConnectionPatch(safePrevious, {
        status: "offline",
        lastEventAt: eventAt,
        lastEventType: type,
        lastError: stringOr(data?.message, data?.error, safePrevious?.connection?.lastError, "")
      });
    case "job.changed":
      return applyJobChanged(
        applyConnectionPatch(safePrevious, {
          status: "live",
          lastEventAt: eventAt,
          lastEventType: type
        }),
        data
      );
    case "video.changed":
      return applyVideoChanged(
        applyConnectionPatch(safePrevious, {
          status: "live",
          lastEventAt: eventAt,
          lastEventType: type
        }),
        data
      );
    case "creator.changed":
      return applyCreatorChanged(
        applyConnectionPatch(safePrevious, {
          status: "live",
          lastEventAt: eventAt,
          lastEventType: type
        }),
        data
      );
    case "storage.changed": {
      const storagePayload = data?.storage || data;
      return {
        ...applyConnectionPatch(safePrevious, {
          status: "live",
          lastEventAt: eventAt,
          lastEventType: type
        }),
        storage: normalizeStorage(storagePayload, safePrevious?.system, safePrevious?.storage),
        system: {
          ...safePrevious.system,
          storageRoot: stringOr(storagePayload?.root_dir, safePrevious?.system?.storageRoot, "-"),
          overview: {
            ...safePrevious.system.overview,
            rareVideos: numberOr(storagePayload?.rare_videos, safePrevious?.system?.overview?.rareVideos, 0)
          }
        }
      };
    }
    case "system.changed": {
      const systemPatch = data?.system || data;
      return {
        ...applyConnectionPatch(safePrevious, {
          status: "live",
          lastEventAt: eventAt,
          lastEventType: type
        }),
        limits: normalizeLimits(systemPatch?.limits, safePrevious?.limits),
        scheduler: normalizeScheduler(systemPatch?.scheduler, safePrevious?.scheduler),
        system: normalizeSystemPatch(safePrevious.system, systemPatch)
      };
    }
    default:
      return safePrevious;
  }
}

export function applySystemStatusSnapshot(previous, payload) {
  const safePrevious = previous || createDefaultState();
  const systemPatch = payload || {};

  return {
    ...safePrevious,
    limits: normalizeLimits(systemPatch?.limits, safePrevious?.limits),
    scheduler: normalizeScheduler(systemPatch?.scheduler, safePrevious?.scheduler),
    system: normalizeSystemPatch(safePrevious.system, systemPatch)
  };
}

export function applyCandidateListSnapshot(previous, payload, lastSyncAt = formatNow()) {
  const safePrevious = previous || createDefaultState();
  const items = normalizeCandidateItems(payload?.items);
  const selectedID = numberOr(safePrevious?.candidatePool?.selectedID, 0);
  const currentDetail = safePrevious?.candidatePool?.detail;
  const selectedFromList = items.find((item) => item.id === selectedID) || null;
  const nextDetail =
    currentDetail?.candidate?.id === selectedID && selectedFromList
      ? {
          ...currentDetail,
          candidate: {
            ...currentDetail.candidate,
            ...selectedFromList
          }
        }
      : currentDetail;

  return {
    ...safePrevious,
    candidatePool: {
      ...safePrevious.candidatePool,
      items,
      total: numberOr(payload?.total, items.length),
      page: numberOr(payload?.page, safePrevious?.candidatePool?.page, 1),
      pageSize: numberOr(payload?.page_size, safePrevious?.candidatePool?.pageSize, DEFAULT_CANDIDATE_PAGE_SIZE),
      lastSyncAt,
      detail: nextDetail
    }
  };
}

export function applyCandidateDetailSnapshot(previous, payload) {
  const safePrevious = previous || createDefaultState();
  const candidate = normalizeCandidateCore(payload?.candidate);
  if (!candidate.id) {
    return safePrevious;
  }

  const sources = normalizeCandidateSources(payload?.sources);
  const scoreDetails = normalizeCandidateScoreDetails(payload?.score_details);
  const nextItem = {
    ...candidate,
    sources
  };

  return {
    ...safePrevious,
    candidatePool: {
      ...safePrevious.candidatePool,
      items: mergeEntityByID(safePrevious?.candidatePool?.items, nextItem, {}, createEmptyCandidate),
      selectedID: candidate.id,
      detail: {
        candidate,
        sources,
        scoreDetails
      }
    }
  };
}

export function applyCandidateReviewAction(previous, candidateID, nextStatus, actedAt = new Date().toISOString()) {
  const safePrevious = previous || createDefaultState();
  const nextItems = (Array.isArray(safePrevious?.candidatePool?.items) ? safePrevious.candidatePool.items : []).map((item) =>
    item?.id === candidateID ? applyCandidateStatusPatch(item, nextStatus, actedAt) : item
  );
  const nextDetail =
    safePrevious?.candidatePool?.detail?.candidate?.id === candidateID
      ? {
          ...safePrevious.candidatePool.detail,
          candidate: applyCandidateStatusPatch(safePrevious.candidatePool.detail.candidate, nextStatus, actedAt)
        }
      : safePrevious?.candidatePool?.detail || null;

  return {
    ...safePrevious,
    candidatePool: {
      ...safePrevious.candidatePool,
      items: nextItems,
      detail: nextDetail
    }
  };
}

export function normalizePagerState(pager, defaultPageSize = DEFAULT_LIST_PAGE_SIZE) {
  const { page, pageSize } = resolvePagination(0, pager?.page, pager?.pageSize, defaultPageSize);
  return { page, pageSize };
}

export function resolvePagination(totalItems, page, pageSize, defaultPageSize = DEFAULT_LIST_PAGE_SIZE) {
  const total = Math.max(0, Math.floor(numberOr(totalItems, 0)));
  const safePageSize = clampPositiveInteger(pageSize, defaultPageSize);
  const totalPages = Math.max(1, Math.ceil(total / safePageSize) || 1);
  const safePage = Math.min(clampPositiveInteger(page, 1), totalPages);
  const startIndex = Math.min((safePage - 1) * safePageSize, total);
  const endIndex = Math.min(startIndex + safePageSize, total);

  return {
    total,
    page: safePage,
    pageSize: safePageSize,
    totalPages,
    startIndex,
    endIndex
  };
}

export function paginateItems(items, page, pageSize, defaultPageSize = DEFAULT_LIST_PAGE_SIZE) {
  const source = Array.isArray(items) ? items : [];
  const pagination = resolvePagination(source.length, page, pageSize, defaultPageSize);
  return {
    ...pagination,
    items: source.slice(pagination.startIndex, pagination.endIndex)
  };
}

export function deriveMetrics(state) {
  const creators = Array.isArray(state?.creators) ? state.creators : [];
  const videos = Array.isArray(state?.videos) ? state.videos : [];
  const jobs = Array.isArray(state?.jobs) ? state.jobs : [];
  const storage = state?.storage || {};
  const overview = state?.system?.overview || {};
  const preciseStoragePercent = calculateStoragePercent(storage);

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
    storagePercent: preciseStoragePercent
  };
}

export function deriveCandidateInsights(state, now = new Date()) {
  const items = Array.isArray(state?.candidatePool?.items) ? state.candidatePool.items : [];
  const start = new Date(now);
  start.setHours(0, 0, 0, 0);
  const end = new Date(start);
  end.setDate(end.getDate() + 1);

  const scoreBands = {
    high: 0,
    medium: 0,
    low: 0
  };

  items.forEach((item) => {
    const score = numberOr(item?.score, 0);
    if (score >= 80) {
      scoreBands.high += 1;
    } else if (score >= 60) {
      scoreBands.medium += 1;
    } else {
      scoreBands.low += 1;
    }
  });

  return {
    totalCount: items.length,
    reviewingCount: items.filter((item) => item?.status === "reviewing").length,
    highPriorityCount: items.filter((item) => item?.status === "reviewing" && numberOr(item?.score, 0) >= 80).length,
    discoveredTodayCount: items.filter((item) => isDateWithinRange(item?.lastDiscoveredAt, start, end)).length,
    ignoredCount: items.filter((item) => item?.status === "ignored").length,
    approvedCount: items.filter((item) => item?.status === "approved").length,
    blockedCount: items.filter((item) => item?.status === "blocked").length,
    scoreBands
  };
}

function calculateStoragePercent(storage) {
  const usedBytes = numberOr(storage?.usedBytes, 0);
  const limitBytes = numberOr(storage?.limitBytes, 0);
  if (limitBytes > 0) {
    if (usedBytes > 0) {
      const percent = (usedBytes * 100) / limitBytes;
      if (Number.isFinite(percent) && percent > 0) {
        return Math.min(100, Math.round(percent * 100) / 100);
      }
    }
    return Math.min(100, numberOr(storage?.usagePercent, 0));
  }
  return Math.min(100, numberOr(storage?.usagePercent, 0));
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

function normalizeCandidateItems(items) {
  if (!Array.isArray(items)) {
    return [];
  }
  return items.map((item) => ({
    ...normalizeCandidateCore(item),
    sources: normalizeCandidateSources(item?.sources)
  }));
}

function normalizeCandidateCore(item) {
  if (!item || typeof item !== "object") {
    return createEmptyCandidate(0);
  }
  return {
    id: numberOr(item?.id, 0),
    platform: stringOr(item?.platform, "bilibili"),
    uid: stringOr(item?.uid, ""),
    name: stringOr(item?.name, ""),
    avatarUrl: stringOr(item?.avatar_url, item?.avatarUrl, ""),
    profileUrl: stringOr(item?.profile_url, item?.profileUrl, ""),
    followerCount: numberOr(item?.follower_count, item?.followerCount, 0),
    status: stringOr(item?.status, "reviewing"),
    score: numberOr(item?.score, 0),
    scoreVersion: stringOr(item?.score_version, item?.scoreVersion, ""),
    lastDiscoveredAt: stringOr(item?.last_discovered_at, item?.lastDiscoveredAt, ""),
    lastScoredAt: stringOr(item?.last_scored_at, item?.lastScoredAt, ""),
    approvedAt: stringOr(item?.approved_at, item?.approvedAt, ""),
    ignoredAt: stringOr(item?.ignored_at, item?.ignoredAt, ""),
    blockedAt: stringOr(item?.blocked_at, item?.blockedAt, ""),
    createdAt: stringOr(item?.created_at, item?.createdAt, ""),
    updatedAt: stringOr(item?.updated_at, item?.updatedAt, "")
  };
}

function normalizeCandidateSources(items) {
  if (!Array.isArray(items)) {
    return [];
  }
  return items.map((item) => ({
    id: numberOr(item?.id, 0),
    sourceType: stringOr(item?.source_type, item?.sourceType, ""),
    sourceValue: stringOr(item?.source_value, item?.sourceValue, ""),
    sourceLabel: stringOr(item?.source_label, item?.sourceLabel, ""),
    weight: numberOr(item?.weight, 0),
    detail: normalizeCandidateSourceDetail(item?.detail_json ?? item?.detail),
    createdAt: stringOr(item?.created_at, item?.createdAt, "")
  }));
}

function normalizeCandidateScoreDetails(items) {
  if (!Array.isArray(items)) {
    return [];
  }
  return items.map((item) => ({
    id: numberOr(item?.id, 0),
    factorKey: stringOr(item?.factor_key, item?.factorKey, ""),
    factorLabel: stringOr(item?.factor_label, item?.factorLabel, ""),
    scoreDelta: numberOr(item?.score_delta, item?.scoreDelta, 0),
    detail: normalizeStructuredDetail(item?.detail_json ?? item?.detail),
    createdAt: stringOr(item?.created_at, item?.createdAt, "")
  }));
}

function normalizeCandidateSourceDetail(detail) {
  const normalized = normalizeStructuredDetail(detail);
  if (!normalized || typeof normalized !== "object" || Array.isArray(normalized)) {
    return {};
  }
  const next = { ...normalized };
  if (Array.isArray(normalized.videos)) {
    next.videos = normalized.videos.map((item) => normalizeCandidateVideoHit(item));
  }
  return next;
}

function normalizeCandidateVideoHit(item) {
  if (!item || typeof item !== "object") {
    return {
      uid: "",
      creatorName: "",
      videoId: "",
      title: "",
      description: "",
      publishTime: "",
      duration: 0,
      coverUrl: "",
      viewCount: 0,
      favoriteCount: 0
    };
  }
  return {
    uid: stringOr(item?.uid, item?.UID, ""),
    creatorName: stringOr(item?.creator_name, item?.CreatorName, ""),
    videoId: stringOr(item?.video_id, item?.VideoID, ""),
    title: stringOr(item?.title, item?.Title, ""),
    description: stringOr(item?.description, item?.Description, ""),
    publishTime: stringOr(item?.publish_time, item?.PublishTime, ""),
    duration: numberOr(item?.duration, item?.Duration, 0),
    coverUrl: stringOr(item?.cover_url, item?.CoverURL, ""),
    viewCount: numberOr(item?.view_count, item?.ViewCount, 0),
    favoriteCount: numberOr(item?.favorite_count, item?.FavoriteCount, 0)
  };
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

function applyConnectionPatch(previous, patch) {
  return {
    ...previous,
    connection: {
      status: stringOr(patch?.status, previous?.connection?.status, "connecting"),
      lastEventAt: stringOr(patch?.lastEventAt, previous?.connection?.lastEventAt, ""),
      lastEventType: stringOr(patch?.lastEventType, previous?.connection?.lastEventType, ""),
      lastError: stringOr(patch?.lastError, previous?.connection?.lastError, "")
    }
  };
}

function mergeEntityByID(previousItems, nextPatch, data, createEmptyItem) {
  const base = Array.isArray(previousItems) ? previousItems : [];
  const next = nextPatch && typeof nextPatch === "object" ? nextPatch : null;

  if (!next || !Number.isFinite(next.id) || next.id <= 0) {
    return base;
  }

  if (isDeletedEvent(data)) {
    return base.filter((item) => item?.id !== next.id);
  }

  const index = base.findIndex((item) => item?.id === next.id);
  const current = index >= 0 ? base[index] : createEmptyItem(next.id);
  const merged = {
    ...current,
    ...next
  };

  if (index >= 0) {
    const cloned = base.slice();
    cloned[index] = merged;
    return cloned;
  }
  return [merged, ...base];
}

function isDeletedEvent(data) {
  const action = String(data?.action || data?.op || data?.change || "").toLowerCase();
  return action === "deleted" || action === "remove" || action === "removed" || data?.deleted === true;
}

function extractEntity(data, key) {
  if (data?.[key] && typeof data[key] === "object") {
    return data[key];
  }
  return data;
}

function hasOwnField(value, key) {
  return Boolean(value) && typeof value === "object" && Object.prototype.hasOwnProperty.call(value, key);
}

function normalizeSystemPatch(previous, systemPatch) {
  return {
    ...previous,
    health: stringOr(systemPatch?.health, previous?.health, "unknown"),
    activeJobs: numberOr(systemPatch?.active_jobs, previous?.activeJobs, 0),
    authEnabled: booleanOr(systemPatch?.auth_enabled, previous?.authEnabled, false),
    riskLevel: stringOr(systemPatch?.risk_level, systemPatch?.risk?.level, previous?.riskLevel, "未知"),
    riskActive: booleanOr(systemPatch?.risk?.active, previous?.riskActive, false),
    riskBackoffUntil: stringOr(systemPatch?.risk?.backoff_until, previous?.riskBackoffUntil, ""),
    riskBackoffSeconds: numberOr(systemPatch?.risk?.backoff_seconds, previous?.riskBackoffSeconds, 0),
    riskLastHitAt: stringOr(systemPatch?.risk?.last_hit_at, previous?.riskLastHitAt, ""),
    riskLastReason: stringOr(systemPatch?.risk?.last_reason, previous?.riskLastReason, ""),
    mysqlOK: booleanOr(systemPatch?.mysql_ok, previous?.mysqlOK, false),
    cookieStatus: stringOr(systemPatch?.cookie?.status, previous?.cookieStatus, "unknown"),
    cookieConfigured: booleanOr(systemPatch?.cookie?.configured, previous?.cookieConfigured, false),
    cookieSource: stringOr(systemPatch?.cookie?.source, previous?.cookieSource, ""),
    cookieUname: stringOr(systemPatch?.cookie?.uname, previous?.cookieUname, ""),
    cookieMid: numberOr(systemPatch?.cookie?.mid, previous?.cookieMid, 0),
    cookieLastCheckAt: stringOr(systemPatch?.cookie?.last_check_at, previous?.cookieLastCheckAt, ""),
    cookieLastCheckResult: stringOr(systemPatch?.cookie?.last_check_result, previous?.cookieLastCheckResult, ""),
    cookieLastReloadAt: stringOr(systemPatch?.cookie?.last_reload_at, previous?.cookieLastReloadAt, ""),
    cookieLastReloadResult: stringOr(systemPatch?.cookie?.last_reload_result, previous?.cookieLastReloadResult, ""),
    cookieLastError: stringOr(systemPatch?.cookie?.last_error, previous?.cookieLastError, ""),
    lastJobAt: stringOr(systemPatch?.last_job_at, previous?.lastJobAt, ""),
    storageRoot: stringOr(systemPatch?.storage_root, previous?.storageRoot, "-"),
    overview: {
      ...previous?.overview,
      activeCreators: numberOr(systemPatch?.overview?.active_creators, previous?.overview?.activeCreators, 0),
      pendingJobs: numberOr(systemPatch?.overview?.pending_jobs, previous?.overview?.pendingJobs, 0),
      rareVideos: numberOr(systemPatch?.overview?.rare_videos, previous?.overview?.rareVideos, 0)
    }
  };
}

function applyJobChanged(previous, data) {
  const jobs = mergeEntityByID(previous.jobs, normalizeJobPatch(extractEntity(data, "job")), data, createEmptyJob);
  const pendingJobs = jobs.filter((job) => job.status === "queued" || job.status === "running").length;
  const latestJob = [...jobs].sort((left, right) => sortByTime(right) - sortByTime(left))[0] || null;

  return {
    ...previous,
    jobs,
    system: {
      ...previous.system,
      activeJobs: pendingJobs,
      lastJobAt: stringOr(latestJob?.updatedAt, latestJob?.createdAt, previous.system.lastJobAt, ""),
      overview: {
        ...previous.system.overview,
        pendingJobs
      }
    }
  };
}

function applyVideoChanged(previous, data) {
  const videos = mergeEntityByID(previous.videos, normalizeVideoPatch(extractEntity(data, "video")), data, createEmptyVideo);
  const rareVideos = videos.filter((video) => video.state === "OUT_OF_PRINT").length;

  return {
    ...previous,
    videos,
    system: {
      ...previous.system,
      overview: {
        ...previous.system.overview,
        rareVideos
      }
    }
  };
}

function applyCreatorChanged(previous, data) {
  const creators = mergeEntityByID(
    previous.creators,
    normalizeCreatorPatch(extractEntity(data, "creator")),
    data,
    createEmptyCreator
  );
  const activeCreators = creators.filter((creator) => creator.status === "active").length;

  return {
    ...previous,
    creators,
    system: {
      ...previous.system,
      overview: {
        ...previous.system.overview,
        activeCreators
      }
    }
  };
}

function createEmptyCreator(id) {
  return {
    id: numberOr(id, 0),
    uid: "",
    name: "",
    platform: "bilibili",
    status: "active"
  };
}

function createEmptyJob(id) {
  return {
    id: numberOr(id, 0),
    type: "",
    status: "queued",
    payload: {},
    errorMsg: "",
    createdAt: "",
    updatedAt: "",
    startedAt: "",
    finishedAt: "",
    origin: "remote"
  };
}

function createEmptyVideo(id) {
  return {
    id: numberOr(id, 0),
    platform: "bilibili",
    videoId: "",
    creatorId: 0,
    title: "",
    description: "",
    publishTime: "",
    duration: 0,
    coverUrl: "",
    viewCount: 0,
    favoriteCount: 0,
    state: "UNKNOWN",
    outOfPrintAt: "",
    stableAt: "",
    lastCheckAt: ""
  };
}

function createEmptyCandidate(id) {
  return {
    id: numberOr(id, 0),
    platform: "bilibili",
    uid: "",
    name: "",
    avatarUrl: "",
    profileUrl: "",
    followerCount: 0,
    status: "reviewing",
    score: 0,
    scoreVersion: "",
    lastDiscoveredAt: "",
    lastScoredAt: "",
    approvedAt: "",
    ignoredAt: "",
    blockedAt: "",
    createdAt: "",
    updatedAt: "",
    sources: []
  };
}

function normalizeCreatorPatch(item) {
  if (!item || typeof item !== "object") {
    return null;
  }
  const patch = {};
  if (hasOwnField(item, "id")) {
    patch.id = numberOr(item.id, 0);
  }
  if (hasOwnField(item, "uid")) {
    patch.uid = String(item.uid || "");
  }
  if (hasOwnField(item, "name")) {
    patch.name = String(item.name || "");
  }
  if (hasOwnField(item, "platform")) {
    patch.platform = String(item.platform || "bilibili");
  }
  if (hasOwnField(item, "status")) {
    patch.status = String(item.status || "active");
  }
  return patch;
}

function normalizeJobPatch(item) {
  if (!item || typeof item !== "object") {
    return null;
  }
  const patch = {};
  if (hasOwnField(item, "id")) {
    patch.id = numberOr(item.id, 0);
  }
  if (hasOwnField(item, "type")) {
    patch.type = String(item.type || "");
  }
  if (hasOwnField(item, "status")) {
    patch.status = String(item.status || "queued");
  }
  if (hasOwnField(item, "payload")) {
    patch.payload = objectOr(item.payload, {});
    patch.origin = stringOr(item.payload?.origin, item.payload?.source, "remote");
  }
  if (hasOwnField(item, "error_msg")) {
    patch.errorMsg = String(item.error_msg || "");
  }
  if (hasOwnField(item, "created_at")) {
    patch.createdAt = String(item.created_at || "");
  }
  if (hasOwnField(item, "updated_at")) {
    patch.updatedAt = String(item.updated_at || "");
  }
  if (hasOwnField(item, "started_at")) {
    patch.startedAt = String(item.started_at || "");
  }
  if (hasOwnField(item, "finished_at")) {
    patch.finishedAt = String(item.finished_at || "");
  }
  return patch;
}

function normalizeVideoPatch(item) {
  if (!item || typeof item !== "object") {
    return null;
  }
  const patch = {};
  if (hasOwnField(item, "id")) {
    patch.id = numberOr(item.id, 0);
  }
  if (hasOwnField(item, "platform")) {
    patch.platform = String(item.platform || "bilibili");
  }
  if (hasOwnField(item, "video_id")) {
    patch.videoId = String(item.video_id || "");
  }
  if (hasOwnField(item, "creator_id")) {
    patch.creatorId = numberOr(item.creator_id, 0);
  }
  if (hasOwnField(item, "title")) {
    patch.title = String(item.title || "");
  }
  if (hasOwnField(item, "description")) {
    patch.description = String(item.description || "");
  }
  if (hasOwnField(item, "publish_time")) {
    patch.publishTime = String(item.publish_time || "");
  }
  if (hasOwnField(item, "duration")) {
    patch.duration = numberOr(item.duration, 0);
  }
  if (hasOwnField(item, "cover_url")) {
    patch.coverUrl = String(item.cover_url || "");
  }
  if (hasOwnField(item, "view_count")) {
    patch.viewCount = numberOr(item.view_count, 0);
  }
  if (hasOwnField(item, "favorite_count")) {
    patch.favoriteCount = numberOr(item.favorite_count, 0);
  }
  if (hasOwnField(item, "state")) {
    patch.state = String(item.state || "UNKNOWN");
  }
  if (hasOwnField(item, "out_of_print_at")) {
    patch.outOfPrintAt = String(item.out_of_print_at || "");
  }
  if (hasOwnField(item, "stable_at")) {
    patch.stableAt = String(item.stable_at || "");
  }
  if (hasOwnField(item, "last_check_at")) {
    patch.lastCheckAt = String(item.last_check_at || "");
  }
  return patch;
}

function applyCandidateStatusPatch(candidate, nextStatus, actedAt) {
  const status = String(nextStatus || "").trim() || candidate?.status || "reviewing";
  return {
    ...candidate,
    status,
    approvedAt: status === "approved" ? actedAt : status === "reviewing" ? "" : candidate?.approvedAt || "",
    ignoredAt: status === "ignored" ? actedAt : status === "reviewing" ? "" : candidate?.ignoredAt || "",
    blockedAt: status === "blocked" ? actedAt : candidate?.blockedAt || "",
    updatedAt: actedAt || candidate?.updatedAt || ""
  };
}

function normalizeStructuredDetail(value) {
  if (value == null) {
    return {};
  }
  if (typeof value === "string") {
    try {
      return JSON.parse(value);
    } catch (_error) {
      return {
        raw: value
      };
    }
  }
  if (typeof value === "object") {
    return value;
  }
  return {
    raw: String(value)
  };
}

function createDefaultPaginationState() {
  return {
    creators: createPagerState(DEFAULT_LIST_PAGE_SIZE),
    jobs: createPagerState(DEFAULT_LIST_PAGE_SIZE),
    videos: createPagerState(DEFAULT_LIST_PAGE_SIZE)
  };
}

function normalizeCandidatePoolState(value, defaults, options = {}) {
  const resetItems = Boolean(options?.resetItems);
  const forceDefaultPageSize = Boolean(options?.forceDefaultPageSize);
  const forceDefaultFilterStatus = Boolean(options?.forceDefaultFilterStatus);
  const pageSize = normalizeSupportedPageSize(value?.pageSize, defaults.pageSize);
  const effectivePageSize = forceDefaultPageSize ? defaults.pageSize : pageSize;
  const currentStatus = String(value?.filters?.status || "").trim();
  const effectiveStatus = forceDefaultFilterStatus && currentStatus === "" ? defaults.filters.status : currentStatus;
  const items =
    resetItems || !Array.isArray(value?.items)
      ? defaults.items
      : value.items.slice(0, effectivePageSize);
  const shouldResetPage = forceDefaultPageSize || (forceDefaultFilterStatus && currentStatus === "");

  return {
    ...defaults,
    ...(value || {}),
    items,
    filters: {
      ...defaults.filters,
      ...(value?.filters || {}),
      status: effectiveStatus
    },
    page: shouldResetPage ? defaults.page : clampPositiveInteger(value?.page, defaults.page),
    pageSize: effectivePageSize,
    detail: value?.detail && typeof value.detail === "object" ? value.detail : defaults.detail
  };
}

function normalizePaginationState(value) {
  return {
    creators: normalizePagerState(value?.creators, DEFAULT_LIST_PAGE_SIZE),
    jobs: normalizePagerState(value?.jobs, DEFAULT_LIST_PAGE_SIZE),
    videos: normalizePagerState(value?.videos, DEFAULT_LIST_PAGE_SIZE)
  };
}

function createPagerState(defaultPageSize) {
  return normalizePagerState({ page: 1, pageSize: defaultPageSize }, defaultPageSize);
}

function normalizeSupportedPageSize(value, fallback) {
  const normalized = clampPositiveInteger(value, fallback);
  if (SUPPORTED_PAGE_SIZES.includes(normalized)) {
    return normalized;
  }
  return clampPositiveInteger(fallback, DEFAULT_CANDIDATE_PAGE_SIZE);
}

function clampPositiveInteger(value, fallback) {
  const normalized = Math.floor(Number(value));
  if (Number.isFinite(normalized) && normalized > 0) {
    return normalized;
  }

  const fallbackValue = Math.floor(Number(fallback));
  if (Number.isFinite(fallbackValue) && fallbackValue > 0) {
    return fallbackValue;
  }

  return 1;
}

function isDateWithinRange(value, start, end) {
  const timestamp = Date.parse(String(value || ""));
  if (!Number.isFinite(timestamp)) {
    return false;
  }
  return timestamp >= start.getTime() && timestamp < end.getTime();
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
