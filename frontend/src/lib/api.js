const DEFAULT_TIMEOUT = 5000;

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
