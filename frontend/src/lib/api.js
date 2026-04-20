const DEFAULT_TIMEOUT = 5000;
const DEFAULT_STREAM_RECONNECT_DELAY = 1000;

async function request(baseURL, path, options = {}) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), DEFAULT_TIMEOUT);
  const url = `${String(baseURL || "").replace(/\/$/, "")}${path}`;

  try {
    const response = await fetch(url, {
      ...options,
      headers: {
        "Content-Type": "application/json",
        ...(options.headers || {})
      },
      signal: controller.signal
    });

    const data = await response.json().catch(() => null);
    if (!response.ok) {
      const message = data?.error || `请求失败: ${response.status}`;
      throw new Error(message);
    }
    return data;
  } finally {
    clearTimeout(timeout);
  }
}

export async function listCreators(baseURL) {
  const payload = await request(baseURL, `/creators${buildQuery({ limit: 200 })}`, {
    method: "GET"
  });
  return Array.isArray(payload?.items) ? payload.items : [];
}

export async function createCreator(baseURL, creator) {
  return request(baseURL, "/creators", {
    method: "POST",
    body: JSON.stringify(creator)
  });
}

export async function deleteCreator(baseURL, id) {
  return request(baseURL, `/creators/${id}`, {
    method: "DELETE"
  });
}

export async function patchCreator(baseURL, id, patch) {
  return request(baseURL, `/creators/${id}`, {
    method: "PATCH",
    body: JSON.stringify(patch)
  });
}

export async function enqueueJob(baseURL, type) {
  return request(baseURL, "/jobs", {
    method: "POST",
    body: JSON.stringify({ type })
  });
}

export async function listJobs(baseURL, filters = {}) {
  const payload = await request(baseURL, `/jobs${buildQuery(filters)}`, {
    method: "GET"
  });
  return Array.isArray(payload?.items) ? payload.items : [];
}

export async function listVideos(baseURL, filters = {}) {
  const payload = await request(baseURL, `/videos${buildQuery(filters)}`, {
    method: "GET"
  });
  return Array.isArray(payload?.items) ? payload.items : [];
}

export async function listCandidateCreators(baseURL, filters = {}) {
  const payload = await request(baseURL, `/candidate-creators${buildQuery(filters)}`, {
    method: "GET"
  });
  return {
    items: Array.isArray(payload?.items) ? payload.items : [],
    total: Number(payload?.total) || 0,
    page: Number(payload?.page) || 1,
    page_size: Number(payload?.page_size) || Number(filters?.page_size) || 20
  };
}

export async function getCandidateCreator(baseURL, id) {
  return request(baseURL, `/candidate-creators/${id}`, {
    method: "GET"
  });
}

export async function triggerCandidateDiscover(baseURL) {
  return request(baseURL, "/candidate-creators/discover", {
    method: "POST"
  });
}

export async function approveCandidateCreator(baseURL, id) {
  return request(baseURL, `/candidate-creators/${id}/approve`, {
    method: "POST"
  });
}

export async function ignoreCandidateCreator(baseURL, id) {
  return request(baseURL, `/candidate-creators/${id}/ignore`, {
    method: "POST"
  });
}

export async function blockCandidateCreator(baseURL, id) {
  return request(baseURL, `/candidate-creators/${id}/block`, {
    method: "POST"
  });
}

export async function reviewCandidateCreator(baseURL, id) {
  return request(baseURL, `/candidate-creators/${id}/review`, {
    method: "POST"
  });
}

export async function getSystemStatus(baseURL) {
  return request(baseURL, "/system/status", {
    method: "GET"
  });
}

export async function getStorageStats(baseURL) {
  return request(baseURL, "/storage/stats", {
    method: "GET"
  });
}

export async function getSystemConfig(baseURL) {
  return request(baseURL, "/system/config", {
    method: "GET"
  });
}

export async function updateSystemConfig(baseURL, content) {
  return request(baseURL, "/system/config", {
    method: "PUT",
    body: JSON.stringify({ content })
  });
}

export async function loadDashboardSnapshot(baseURL) {
  const [creators, jobs, videos, system, storage] = await Promise.all([
    listCreators(baseURL),
    listJobs(baseURL, { limit: 12 }),
    listVideos(baseURL, { limit: 8 }),
    getSystemStatus(baseURL),
    getStorageStats(baseURL)
  ]);

  return {
    creators,
    jobs,
    videos,
    system,
    storage
  };
}

export function createDashboardEventStream(baseURL, handlers = {}, options = {}) {
  if (typeof EventSource === "undefined") {
    handlers.onError?.(new Error("当前环境不支持 EventSource"));
    return { close() {} };
  }

  const root = String(baseURL || "").replace(/\/$/, "");
  const url = `${root}/events/stream`;
  const reconnectDelay = Math.max(Number(options.reconnectDelayMs) || DEFAULT_STREAM_RECONNECT_DELAY, 200);
  const eventTypes = [
    "job.changed",
    "video.changed",
    "creator.changed",
    "storage.changed",
    "system.changed",
    "hello",
    "heartbeat"
  ];
  const subscriptions = new Map();
  let closed = false;
  let reconnectTimer = 0;
  let currentSource = null;

  const clearReconnectTimer = () => {
    if (reconnectTimer) {
      window.clearTimeout(reconnectTimer);
      reconnectTimer = 0;
    }
  };

  const detachSource = (source) => {
    if (!source) {
      return;
    }
    const unsubs = subscriptions.get(source) || [];
    unsubs.forEach((fn) => fn());
    subscriptions.delete(source);
    source.onopen = null;
    source.onerror = null;
    source.close();
    if (currentSource === source) {
      currentSource = null;
    }
  };

  const scheduleReconnect = () => {
    if (closed || reconnectTimer) {
      return;
    }
    reconnectTimer = window.setTimeout(() => {
      reconnectTimer = 0;
      connect();
    }, reconnectDelay);
  };

  const connect = () => {
    if (closed) {
      return;
    }

    const source = new EventSource(url);
    currentSource = source;

    source.onopen = () => {
      if (closed || currentSource !== source) {
        return;
      }
      clearReconnectTimer();
      handlers.onOpen?.();
    };

    source.onerror = (error) => {
      if (closed || currentSource !== source) {
        return;
      }
      handlers.onError?.(error);
      detachSource(source);
      scheduleReconnect();
    };

    const unsubs = eventTypes.map((type) => {
      const listener = (message) => {
        if (closed || currentSource !== source) {
          return;
        }
        handlers.onEvent?.({
          type,
          data: parseEventData(message?.data)
        });
      };
      source.addEventListener(type, listener);
      return () => source.removeEventListener(type, listener);
    });
    subscriptions.set(source, unsubs);
  };

  connect();

  return {
    close() {
      closed = true;
      clearReconnectTimer();
      detachSource(currentSource);
    }
  };
}

export function formatRequestError(error) {
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return "请求失败";
}

function buildQuery(params = {}) {
  const search = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (value === undefined || value === null || value === "") {
      return;
    }
    search.set(key, String(value));
  });

  const query = search.toString();
  return query ? `?${query}` : "";
}

function parseEventData(raw) {
  if (!raw) {
    return {};
  }
  try {
    return JSON.parse(raw);
  } catch (_error) {
    return {};
  }
}
