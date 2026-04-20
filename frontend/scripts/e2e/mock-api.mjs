import http from "node:http";

const port = Number(process.env.E2E_API_PORT || 43180);
const sseRetryMs = 250;
const streamClients = new Set();
const scheduledTimers = new Set();

let state = initialState();
let streamAvailable = true;
let nextEventID = 1;
let configRestartUntil = 0;

function initialState() {
  return {
    configText: `server:
  http_addr: ":8080"
  shutdown_timeout: 10s
storage:
  root: /data/archive
  limit_gb: 20
scheduler:
  fetch_interval: 45m
  check_interval: 24h
  cleanup_interval: 24h
  stable_days: 30
`,
    creators: [
      { id: 101, uid: "123456", name: "Mock 收藏向频道", platform: "bilibili", status: "active" },
      { id: 102, uid: "654321", name: "Mock 科技区 UP", platform: "bilibili", status: "paused" }
    ],
    jobs: [
      {
        id: 201,
        type: "check",
        status: "failed",
        payload: { video_id: 901 },
        error_msg: "Mock 视频接口返回 412",
        created_at: "2026-04-13T12:00:00Z",
        updated_at: "2026-04-13T12:03:00Z",
        finished_at: "2026-04-13T12:03:00Z"
      },
      {
        id: 202,
        type: "download",
        status: "running",
        payload: { video_id: 902 },
        created_at: "2026-04-13T12:05:00Z",
        updated_at: "2026-04-13T12:06:00Z",
        started_at: "2026-04-13T12:06:00Z"
      }
    ],
    videos: [
      {
        id: 901,
        platform: "bilibili",
        video_id: "BV1mock901",
        creator_id: 101,
        title: "Mock 绝版视频",
        description: "",
        publish_time: "2026-04-10T09:00:00Z",
        duration: 360,
        cover_url: "",
        view_count: 120,
        favorite_count: 20,
        state: "OUT_OF_PRINT",
        out_of_print_at: "2026-04-13T08:00:00Z",
        stable_at: "",
        last_check_at: "2026-04-13T12:00:00Z"
      },
      {
        id: 902,
        platform: "bilibili",
        video_id: "BV1mock902",
        creator_id: 101,
        title: "Mock 普通视频",
        description: "",
        publish_time: "2026-04-11T09:00:00Z",
        duration: 180,
        cover_url: "",
        view_count: 80,
        favorite_count: 8,
        state: "DOWNLOADED",
        out_of_print_at: "",
        stable_at: "",
        last_check_at: "2026-04-13T11:00:00Z"
      }
    ],
    candidates: [
      {
        candidate: {
          id: 301,
          platform: "bilibili",
          uid: "9001",
          name: "候选补档站",
          avatar_url: "",
          profile_url: "https://space.bilibili.com/9001",
          follower_count: 321000,
          status: "reviewing",
          score: 88,
          score_version: "v1",
          last_discovered_at: "2026-04-13T09:30:00+08:00",
          last_scored_at: "2026-04-13T09:35:00+08:00",
          approved_at: "",
          ignored_at: "",
          blocked_at: "",
          created_at: "2026-04-13T09:36:00+08:00",
          updated_at: "2026-04-13T09:36:00+08:00"
        },
        sources: [
          {
            id: 1,
            source_type: "keyword",
            source_value: "补档",
            source_label: "关键词：补档",
            weight: 15,
            detail_json: {
              keyword: "补档",
              videos: [
                {
                  UID: "9001",
                  CreatorName: "候选补档站",
                  VideoID: "BV1seed301",
                  Title: "补档测试视频",
                  PublishTime: "2026-04-13T08:00:00+08:00",
                  ViewCount: 4200,
                  FavoriteCount: 320
                }
              ]
            },
            created_at: "2026-04-13T09:36:00+08:00"
          }
        ],
        score_details: [
          {
            id: 11,
            factor_key: "keyword_risk",
            factor_label: "命中高风险关键词",
            score_delta: 30,
            detail_json: {
              keywords: ["补档", "未删减"]
            },
            created_at: "2026-04-13T09:35:00+08:00"
          },
          {
            id: 12,
            factor_key: "activity_30d",
            factor_label: "最近 30 天更新活跃",
            score_delta: 18,
            detail_json: {
              video_count: 6
            },
            created_at: "2026-04-13T09:35:00+08:00"
          }
        ]
      },
      {
        candidate: {
          id: 302,
          platform: "bilibili",
          uid: "9002",
          name: "观察名单",
          avatar_url: "",
          profile_url: "https://space.bilibili.com/9002",
          follower_count: 98000,
          status: "ignored",
          score: 64,
          score_version: "v1",
          last_discovered_at: "2026-04-12T12:00:00+08:00",
          last_scored_at: "2026-04-12T12:05:00+08:00",
          approved_at: "",
          ignored_at: "2026-04-13T10:00:00+08:00",
          blocked_at: "",
          created_at: "2026-04-12T12:06:00+08:00",
          updated_at: "2026-04-13T10:00:00+08:00"
        },
        sources: [
          {
            id: 2,
            source_type: "keyword",
            source_value: "切片",
            source_label: "关键词：切片",
            weight: 12,
            detail_json: {
              keyword: "切片",
              videos: []
            },
            created_at: "2026-04-12T12:06:00+08:00"
          }
        ],
        score_details: [
          {
            id: 13,
            factor_key: "feedback",
            factor_label: "人工反馈惩罚",
            score_delta: -6,
            detail_json: {
              ignore_count: 1
            },
            created_at: "2026-04-13T10:00:00+08:00"
          }
        ]
      }
    ],
    system: {
      health: "online",
      mysql_ok: true,
      auth_enabled: true,
      active_jobs: 2,
      risk_level: "中",
      risk: {
        level: "中",
        active: false,
        backoff_seconds: 0,
        backoff_until: "",
        last_hit_at: "2026-04-13T11:58:00Z",
        last_reason: "/x/web-interface/view 返回风控码 -412"
      },
      last_job_at: "2026-04-13T12:06:00Z",
      storage_root: "/data/archive",
      cookie: {
        configured: true,
        status: "valid",
        uname: "mock_user",
        source: "config",
        last_check_at: "2026-04-13T11:59:00Z",
        last_reload_at: "2026-04-13T11:50:00Z",
        last_check_result: "valid",
        last_reload_result: "success",
        last_error: ""
      },
      overview: {
        active_creators: 1,
        pending_jobs: 2,
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
      used_bytes: 1717986918,
      max_bytes: 2147483648,
      safe_bytes: 1610612736,
      usage_percent: 80,
      file_count: 22,
      hottest_bucket: "bilibili",
      rare_videos: 1,
      cleanup_rule: "绝版优先 -> 粉丝量 -> 播放量 -> 收藏量"
    }
  };
}

function sendJSON(res, status, payload) {
  res.writeHead(status, {
    "Content-Type": "application/json",
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Methods": "GET,POST,PUT,PATCH,DELETE,OPTIONS",
    "Access-Control-Allow-Headers": "Content-Type"
  });
  res.end(JSON.stringify(payload));
}

function sendEvent(res, type, payload) {
  res.write(`id: ${nextEventID}\n`);
  res.write(`event: ${type}\n`);
  res.write(`data: ${JSON.stringify(payload || {})}\n\n`);
  nextEventID += 1;
}

function broadcastEvent(type, payload) {
  for (const client of [...streamClients]) {
    try {
      sendEvent(client, type, payload);
    } catch (_error) {
      closeStreamClient(client);
    }
  }
}

function closeStreamClient(client) {
  if (!streamClients.has(client)) {
    return;
  }
  streamClients.delete(client);
  try {
    client.end();
  } catch (_error) {
    // ignore
  }
}

function closeAllStreamClients() {
  for (const client of [...streamClients]) {
    closeStreamClient(client);
  }
}

function clearScheduledTimers() {
  for (const timer of scheduledTimers) {
    clearTimeout(timer);
  }
  scheduledTimers.clear();
}

function schedule(delayMs, callback) {
  const timer = setTimeout(() => {
    scheduledTimers.delete(timer);
    callback();
  }, delayMs);
  scheduledTimers.add(timer);
}

function resetRuntimeState() {
  clearScheduledTimers();
  closeAllStreamClients();
  streamAvailable = true;
  nextEventID = 1;
  configRestartUntil = 0;
}

function isConfigRestarting() {
  return configRestartUntil > Date.now();
}

function triggerConfigRestart() {
  const restartMs = 1200;
  configRestartUntil = Date.now() + restartMs;
  streamAvailable = false;
  closeAllStreamClients();
  schedule(restartMs, () => {
    configRestartUntil = 0;
    streamAvailable = true;
  });
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    let body = "";
    req.on("data", (chunk) => {
      body += chunk;
    });
    req.on("end", () => {
      if (!body) {
        resolve({});
        return;
      }
      try {
        resolve(JSON.parse(body));
      } catch (error) {
        reject(error);
      }
    });
    req.on("error", reject);
  });
}

function nextID(items) {
  return items.reduce((max, item) => Math.max(max, Number(item.id) || 0), 0) + 1;
}

function recalcJobCounters() {
  const pendingJobs = state.jobs.filter((item) => item.status === "queued" || item.status === "running").length;
  state.system.active_jobs = pendingJobs;
  state.system.overview.pending_jobs = pendingJobs;
}

function recalcCreatorCounters() {
  state.system.overview.active_creators = state.creators.filter((item) => item.status === "active").length;
}

function broadcastCreatorOverview() {
  recalcCreatorCounters();
  broadcastEvent("system.changed", {
    system: {
      overview: {
        active_creators: state.system.overview.active_creators
      }
    }
  });
}

function enqueueMockJob(type, payload = {}) {
  const now = new Date().toISOString();
  const job = {
    id: nextID(state.jobs),
    type: String(type || "fetch"),
    status: "queued",
    payload: {
      origin: "mock_api",
      ...payload
    },
    created_at: now,
    updated_at: now
  };
  state.jobs.unshift(job);
  recalcJobCounters();
  state.system.last_job_at = now;
  broadcastEvent("job.changed", { job });
  broadcastEvent("system.changed", {
    system: {
      active_jobs: state.system.active_jobs,
      last_job_at: state.system.last_job_at,
      overview: {
        pending_jobs: state.system.overview.pending_jobs
      }
    }
  });
  scheduleJobLifecycle(job.id);
  return job;
}

function listCandidateItems() {
  return state.candidates.map((item) => ({
    ...item.candidate,
    sources: item.sources
  }));
}

function findCandidateRecord(id) {
  return state.candidates.find((item) => item?.candidate?.id === id) || null;
}

function applyCandidateStatus(candidate, status) {
  const now = new Date().toISOString();
  return {
    ...candidate,
    status,
    approved_at: status === "approved" ? now : status === "reviewing" ? "" : candidate.approved_at || "",
    ignored_at: status === "ignored" ? now : status === "reviewing" ? "" : candidate.ignored_at || "",
    blocked_at: status === "blocked" ? now : candidate.blocked_at || "",
    updated_at: now
  };
}

function updateCandidateStatus(id, status) {
  let nextRecord = null;
  state.candidates = state.candidates.map((record) => {
    if (record?.candidate?.id !== id) {
      return record;
    }
    nextRecord = {
      ...record,
      candidate: applyCandidateStatus(record.candidate, status)
    };
    return nextRecord;
  });
  return nextRecord;
}

function buildJobEventPayload(job, patch) {
  const payload = {
    id: job.id
  };

  if (Object.prototype.hasOwnProperty.call(patch, "type")) {
    payload.type = String(job.type || "");
  }
  if (Object.prototype.hasOwnProperty.call(patch, "status")) {
    payload.status = String(job.status || "");
  }
  if (Object.prototype.hasOwnProperty.call(patch, "payload")) {
    payload.payload = job.payload || {};
  }
  if (Object.prototype.hasOwnProperty.call(patch, "error_msg")) {
    payload.error_msg = String(job.error_msg || "");
  }
  if (Object.prototype.hasOwnProperty.call(patch, "created_at")) {
    payload.created_at = String(job.created_at || "");
  }
  if (Object.prototype.hasOwnProperty.call(patch, "updated_at")) {
    payload.updated_at = String(job.updated_at || "");
  }
  if (Object.prototype.hasOwnProperty.call(patch, "started_at")) {
    payload.started_at = String(job.started_at || "");
  }
  if (Object.prototype.hasOwnProperty.call(patch, "finished_at")) {
    payload.finished_at = String(job.finished_at || "");
  }

  return payload;
}

function updateJob(id, patch) {
  let updatedJob = null;
  state.jobs = state.jobs.map((job) => {
    if (job.id !== id) {
      return job;
    }
    updatedJob = {
      ...job,
      ...patch
    };
    return updatedJob;
  });

  if (!updatedJob) {
    return null;
  }

  recalcJobCounters();
  state.system.last_job_at = String(updatedJob.updated_at || updatedJob.created_at || state.system.last_job_at || "");
  broadcastEvent("job.changed", { job: buildJobEventPayload(updatedJob, patch) });
  broadcastEvent("system.changed", {
    system: {
      active_jobs: state.system.active_jobs,
      last_job_at: state.system.last_job_at,
      overview: {
        pending_jobs: state.system.overview.pending_jobs
      }
    }
  });
  return updatedJob;
}

function scheduleJobLifecycle(jobID) {
  schedule(500, () => {
    const runningAt = new Date().toISOString();
    updateJob(jobID, {
      status: "running",
      started_at: runningAt,
      updated_at: runningAt
    });
  });

  schedule(1200, () => {
    const finishedAt = new Date().toISOString();
    updateJob(jobID, {
      status: "success",
      finished_at: finishedAt,
      updated_at: finishedAt,
      error_msg: ""
    });
  });
}

const server = http.createServer(async (req, res) => {
  if (!req.url) {
    sendJSON(res, 404, { error: "not found" });
    return;
  }

  if (req.method === "OPTIONS") {
    sendJSON(res, 204, {});
    return;
  }

  try {
    if (isConfigRestarting() && !req.url.startsWith("/__")) {
      if (req.method === "GET" && req.url === "/events/stream") {
        res.writeHead(503, {
          "Content-Type": "text/plain; charset=utf-8",
          "Cache-Control": "no-cache",
          "Access-Control-Allow-Origin": "*"
        });
        res.end("server restarting");
        return;
      }

      sendJSON(res, 503, { error: "后端正在重启，请稍后重试" });
      return;
    }

    if (req.method === "GET" && req.url === "/healthz") {
      sendJSON(res, 200, { status: "ok" });
      return;
    }

    if (req.method === "GET" && req.url === "/events/stream") {
      if (!streamAvailable) {
        res.writeHead(503, {
          "Content-Type": "text/plain; charset=utf-8",
          "Cache-Control": "no-cache",
          "Access-Control-Allow-Origin": "*"
        });
        res.end("stream unavailable");
        return;
      }

      res.writeHead(200, {
        "Content-Type": "text/event-stream",
        "Cache-Control": "no-cache, no-transform",
        Connection: "keep-alive",
        "Access-Control-Allow-Origin": "*"
      });
      res.write(`retry: ${sseRetryMs}\n\n`);
      sendEvent(res, "hello", {
        server_time: new Date().toISOString(),
        stream: "mock-api"
      });
      streamClients.add(res);

      req.on("close", () => {
        streamClients.delete(res);
      });
      return;
    }

    if (req.method === "POST" && req.url === "/__reset") {
      resetRuntimeState();
      state = initialState();
      sendJSON(res, 200, { status: "ok" });
      return;
    }

    if (req.method === "POST" && req.url === "/__mock/events/disconnect") {
      streamAvailable = false;
      closeAllStreamClients();
      sendJSON(res, 200, { status: "ok", stream: "offline" });
      return;
    }

    if (req.method === "POST" && req.url === "/__mock/events/recover") {
      streamAvailable = true;
      sendJSON(res, 200, { status: "ok", stream: "live" });
      return;
    }

    if (req.method === "GET" && req.url.startsWith("/candidate-creators")) {
      const url = new URL(req.url, "http://127.0.0.1");
      const path = url.pathname;

      if (path === "/candidate-creators") {
        const status = String(url.searchParams.get("status") || "").trim();
        const keyword = String(url.searchParams.get("keyword") || "").trim();
        const minScore = Number(url.searchParams.get("min_score") || 0);
        const page = Math.max(Number(url.searchParams.get("page") || 1), 1);
        const pageSize = Math.max(Number(url.searchParams.get("page_size") || 20), 1);

        const filtered = listCandidateItems().filter((item) => {
          if (status && item.status !== status) {
            return false;
          }
          if (minScore > 0 && Number(item.score || 0) < minScore) {
            return false;
          }
          if (!keyword) {
            return true;
          }
          const sourceLabels = Array.isArray(item.sources) ? item.sources.map((source) => source.source_label || "").join(" ") : "";
          const haystack = `${item.name || ""} ${item.uid || ""} ${sourceLabels}`;
          return haystack.includes(keyword);
        });

        const total = filtered.length;
        const startIndex = (page - 1) * pageSize;
        const items = filtered.slice(startIndex, startIndex + pageSize);

        sendJSON(res, 200, {
          items,
          total,
          page,
          page_size: pageSize
        });
        return;
      }

      const match = path.match(/^\/candidate-creators\/(\d+)$/);
      if (match) {
        const record = findCandidateRecord(Number(match[1]));
        if (!record) {
          sendJSON(res, 404, { error: "候选不存在" });
          return;
        }
        sendJSON(res, 200, record);
        return;
      }
    }

    if (req.method === "POST" && req.url === "/candidate-creators/discover") {
      enqueueMockJob("discover", { scope: "candidate_discover" });

      if (!findCandidateRecord(303)) {
        state.candidates.unshift({
          candidate: {
            id: 303,
            platform: "bilibili",
            uid: "9003",
            name: "新发现候选",
            avatar_url: "",
            profile_url: "https://space.bilibili.com/9003",
            follower_count: 156000,
            status: "reviewing",
            score: 82,
            score_version: "v1",
            last_discovered_at: new Date().toISOString(),
            last_scored_at: new Date().toISOString(),
            approved_at: "",
            ignored_at: "",
            blocked_at: "",
            created_at: new Date().toISOString(),
            updated_at: new Date().toISOString()
          },
          sources: [
            {
              id: 3,
              source_type: "keyword",
              source_value: "重传",
              source_label: "关键词：重传",
              weight: 15,
              detail_json: {
                keyword: "重传",
                videos: [
                  {
                    UID: "9003",
                    CreatorName: "新发现候选",
                    VideoID: "BV1seed303",
                    Title: "重传样本视频",
                    PublishTime: new Date().toISOString(),
                    ViewCount: 1800,
                    FavoriteCount: 140
                  }
                ]
              },
              created_at: new Date().toISOString()
            }
          ],
          score_details: [
            {
              id: 14,
              factor_key: "keyword_risk",
              factor_label: "命中高风险关键词",
              score_delta: 24,
              detail_json: {
                keywords: ["重传"]
              },
              created_at: new Date().toISOString()
            }
          ]
        });
      }

      sendJSON(res, 200, { status: "queued", type: "discover" });
      return;
    }

    if (req.method === "POST" && req.url.startsWith("/candidate-creators/")) {
      const url = new URL(req.url, "http://127.0.0.1");
      const match = url.pathname.match(/^\/candidate-creators\/(\d+)\/(approve|ignore|block|review)$/);
      if (match) {
        const id = Number(match[1]);
        const action = match[2];
        const record = findCandidateRecord(id);
        if (!record) {
          sendJSON(res, 404, { error: "候选不存在" });
          return;
        }

        if (action === "approve") {
          const updated = updateCandidateStatus(id, "approved");
          let creator = state.creators.find((item) => item.uid === updated?.candidate?.uid) || null;
          if (!creator) {
            creator = {
              id: nextID(state.creators),
              uid: updated?.candidate?.uid || "",
              name: updated?.candidate?.name || "",
              platform: updated?.candidate?.platform || "bilibili",
              status: "active"
            };
            state.creators.unshift(creator);
            broadcastEvent("creator.changed", { creator });
            broadcastCreatorOverview();
          }
          enqueueMockJob("fetch", { creator_id: creator.id, source: "candidate_approve" });
          sendJSON(res, 200, creator);
          return;
        }

        const nextStatus = action === "ignore" ? "ignored" : action === "block" ? "blocked" : "reviewing";
        updateCandidateStatus(id, nextStatus);
        sendJSON(res, 200, {
          status: "ok",
          action,
          candidate_id: id
        });
        return;
      }
    }

    if (req.method === "GET" && req.url.startsWith("/creators")) {
      sendJSON(res, 200, { items: state.creators });
      return;
    }

    if (req.method === "POST" && req.url === "/creators") {
      const payload = await readBody(req);
      const creator = {
        id: nextID(state.creators),
        uid: String(payload.uid || ""),
        name: String(payload.name || ""),
        platform: String(payload.platform || "bilibili"),
        status: String(payload.status || "active")
      };
      state.creators.unshift(creator);
      recalcCreatorCounters();
      broadcastEvent("creator.changed", { creator });
      broadcastCreatorOverview();
      sendJSON(res, 200, creator);
      return;
    }

    if (req.method === "PATCH" && req.url.startsWith("/creators/")) {
      const id = Number(req.url.split("/")[2]);
      const payload = await readBody(req);
      let found = false;
      state.creators = state.creators.map((creator) => {
        if (creator.id !== id) {
          return creator;
        }
        found = true;
        return {
          ...creator,
          name: payload.name === undefined ? creator.name : String(payload.name || ""),
          status: payload.status === undefined ? creator.status : String(payload.status || creator.status)
        };
      });
      if (!found) {
        sendJSON(res, 404, { error: "not found" });
        return;
      }
      recalcCreatorCounters();
      const creator = state.creators.find((item) => item.id === id);
      broadcastEvent("creator.changed", { creator });
      broadcastCreatorOverview();
      sendJSON(res, 200, creator);
      return;
    }

    if (req.method === "GET" && req.url.startsWith("/jobs")) {
      sendJSON(res, 200, { items: state.jobs });
      return;
    }

    if (req.method === "POST" && req.url === "/jobs") {
      const payload = await readBody(req);
      const job = enqueueMockJob(String(payload.type || "fetch"));
      sendJSON(res, 200, { status: "queued", type: job.type });
      return;
    }

    if (req.method === "GET" && req.url.startsWith("/videos")) {
      sendJSON(res, 200, { items: state.videos });
      return;
    }

    if (req.method === "GET" && req.url === "/system/status") {
      sendJSON(res, 200, state.system);
      return;
    }

    if (req.method === "GET" && req.url === "/system/config") {
      sendJSON(res, 200, {
        path: "/app/config.yaml",
        content: state.configText
      });
      return;
    }

    if (req.method === "PUT" && req.url === "/system/config") {
      const payload = await readBody(req);
      const nextContent = String(payload.content || "");
      const tabLineIndex = nextContent.split("\n").findIndex((line) => line.includes("\t"));
      if (tabLineIndex >= 0) {
        sendJSON(res, 400, {
          error: `配置校验失败: 第 ${tabLineIndex + 1} 行包含 Tab 缩进，请改为空格缩进`
        });
        return;
      }
      const changed = nextContent !== state.configText;
      if (changed) {
        state.configText = nextContent;
        triggerConfigRestart();
      }
      sendJSON(res, 200, {
        changed,
        restart_scheduled: changed,
        path: "/app/config.yaml"
      });
      return;
    }

    if (req.method === "GET" && req.url === "/storage/stats") {
      sendJSON(res, 200, state.storage);
      return;
    }

    sendJSON(res, 404, { error: "not found" });
  } catch (error) {
    sendJSON(res, 500, { error: error instanceof Error ? error.message : "unknown error" });
  }
});

server.listen(port, "127.0.0.1", () => {
  console.log(`mock api listening on http://127.0.0.1:${port}`);
});
