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
      state.system.overview.active_creators = state.creators.filter((item) => item.status === "active").length;
      broadcastEvent("creator.changed", { creator });
      broadcastEvent("system.changed", {
        system: {
          overview: {
            active_creators: state.system.overview.active_creators
          }
        }
      });
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
      state.system.overview.active_creators = state.creators.filter((item) => item.status === "active").length;
      const creator = state.creators.find((item) => item.id === id);
      broadcastEvent("creator.changed", { creator });
      broadcastEvent("system.changed", {
        system: {
          overview: {
            active_creators: state.system.overview.active_creators
          }
        }
      });
      sendJSON(res, 200, creator);
      return;
    }

    if (req.method === "GET" && req.url.startsWith("/jobs")) {
      sendJSON(res, 200, { items: state.jobs });
      return;
    }

    if (req.method === "POST" && req.url === "/jobs") {
      const payload = await readBody(req);
      const now = new Date().toISOString();
      const job = {
        id: nextID(state.jobs),
        type: String(payload.type || "fetch"),
        status: "queued",
        payload: { origin: "mock_api" },
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
