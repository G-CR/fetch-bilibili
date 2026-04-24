import { useEffect, useMemo, useRef, useState } from "react";
import {
  approveCandidateCreator,
  blockCandidateCreator,
  createDashboardEventStream,
  createCreator,
  deleteCreator,
  enqueueJob,
  formatRequestError,
  getCandidateCreator,
  getSystemStatus,
  getSystemConfig,
  ignoreCandidateCreator,
  listCandidateCreators,
  loadDashboardSnapshot,
  patchCreator,
  reviewCandidateCreator,
  triggerCandidateDiscover,
  updateSystemConfig
} from "./lib/api";
import {
  applyCandidateDetailSnapshot,
  applyCandidateListSnapshot,
  applyCandidateReviewAction,
  applyLiveEvent,
  applyRemoteSnapshot,
  applySystemStatusSnapshot,
  deriveCandidateInsights,
  deriveCleanupPreview,
  deriveMetrics,
  deriveTaskDiagnostics,
  loadState,
  makeLog,
  paginateItems,
  resolvePagination,
  saveState
} from "./lib/state";

const sections = [
  { id: "overview", label: "总览" },
  { id: "creators", label: "博主" },
  { id: "candidates", label: "候选池" },
  { id: "tasks", label: "任务" },
  { id: "rare", label: "绝版" },
  { id: "storage", label: "存储" },
  { id: "risk", label: "风控" },
  { id: "settings", label: "设置" }
];

const quickActions = [
  { type: "fetch", label: "立即拉取" },
  { type: "check", label: "检查下架" },
  { type: "cleanup", label: "清理存储" }
];

const platformOptions = [
  { value: "bilibili", label: "B 站" },
  { value: "douyin", label: "抖音" },
  { value: "kuaishou", label: "快手" },
  { value: "xiaohongshu", label: "小红书" }
];

const SYSTEM_RECONCILE_INTERVAL_MS = 30 * 1000;
const SNAPSHOT_RECONCILE_INTERVAL_MS = 60 * 1000;
const CONFIG_RESTART_POLL_INTERVAL_MS = 1500;
const CONFIG_RESTART_TIMEOUT_MS = 45 * 1000;
const PAGE_SIZE_OPTIONS = [6, 12, 20, 50];
const DEFAULT_CANDIDATE_FILTERS = { status: "reviewing", minScore: 0, keyword: "" };
const AUTHORITATIVE_LIVE_EVENT_TYPES = new Set([
  "job.changed",
  "video.changed",
  "creator.changed",
  "storage.changed",
  "system.changed"
]);

function beginVersionedRequest(requestVersionRef, stateVersionRef) {
  const requestVersion = requestVersionRef.current + 1;
  requestVersionRef.current = requestVersion;
  return {
    requestVersion,
    stateVersion: stateVersionRef.current
  };
}

function isVersionedRequestCurrent(requestVersionRef, stateVersionRef, requestMeta) {
  return (
    requestVersionRef.current === requestMeta.requestVersion && stateVersionRef.current === requestMeta.stateVersion
  );
}

function beginRequest(requestVersionRef) {
  const requestVersion = requestVersionRef.current + 1;
  requestVersionRef.current = requestVersion;
  return requestVersion;
}

function isRequestCurrent(requestVersionRef, requestVersion) {
  return requestVersionRef.current === requestVersion;
}

function App() {
  const [state, setState] = useState(() => loadState());
  const hasStreamOpenedRef = useRef(false);
  const authoritativeStateVersionRef = useRef(0);
  const snapshotRequestVersionRef = useRef(0);
  const systemRequestVersionRef = useRef(0);
  const candidateRequestVersionRef = useRef(0);
  const candidateDetailRequestVersionRef = useRef(0);
  const candidateFiltersRef = useRef(state.candidatePool?.filters || DEFAULT_CANDIDATE_FILTERS);
  const [activeSection, setActiveSection] = useState("overview");
  const [selectedJobId, setSelectedJobId] = useState(null);
  const [toast, setToast] = useState("");
  const [busyAction, setBusyAction] = useState("");
  const [candidateLoading, setCandidateLoading] = useState(false);
  const [candidateDetailLoading, setCandidateDetailLoading] = useState(false);
  const [form, setForm] = useState({
    uid: "",
    name: "",
    platform: "bilibili",
    status: "active"
  });
  const [rareFilters, setRareFilters] = useState({
    creatorId: "all",
    keyword: ""
  });
  const [configPath, setConfigPath] = useState("");
  const [configText, setConfigText] = useState("");
  const [savedConfigText, setSavedConfigText] = useState("");
  const [configLoading, setConfigLoading] = useState(false);
  const [configSaving, setConfigSaving] = useState(false);
  const [configRestartState, setConfigRestartState] = useState(() => ({
    active: false,
    startedAt: 0,
    recoveredAt: 0,
    lastError: ""
  }));
  const [configValidation, setConfigValidation] = useState(() => ({
    tone: "idle",
    title: "尚未执行保存校验",
    detail: "保存配置时会展示 YAML 与业务配置校验结果。"
  }));
  const metrics = useMemo(() => deriveMetrics(state), [state]);
  const candidateInsights = useMemo(() => deriveCandidateInsights(state), [state]);
  const taskDiagnostics = useMemo(() => deriveTaskDiagnostics(state), [state]);
  const cleanupPreview = useMemo(() => deriveCleanupPreview(state, 5), [state]);
  const configDiffPreview = useMemo(
    () => deriveConfigDiffPreview(savedConfigText, configText),
    [savedConfigText, configText]
  );
  const creatorsPager = state.pagination?.creators || { page: 1, pageSize: PAGE_SIZE_OPTIONS[0] };
  const jobsPager = state.pagination?.jobs || { page: 1, pageSize: PAGE_SIZE_OPTIONS[0] };
  const videosPager = state.pagination?.videos || { page: 1, pageSize: PAGE_SIZE_OPTIONS[0] };
  const rareVideosPager = state.pagination?.rareVideos || { page: 1, pageSize: PAGE_SIZE_OPTIONS[0] };
  const candidateFilters = state.candidatePool?.filters || DEFAULT_CANDIDATE_FILTERS;
  const candidateItems = Array.isArray(state.candidatePool?.items) ? state.candidatePool.items : [];
  const paginatedCreators = useMemo(
    () => paginateItems(state.creators, creatorsPager.page, creatorsPager.pageSize, PAGE_SIZE_OPTIONS[0]),
    [creatorsPager.page, creatorsPager.pageSize, state.creators]
  );
  const paginatedJobs = useMemo(
    () => paginateItems(state.jobs, jobsPager.page, jobsPager.pageSize, PAGE_SIZE_OPTIONS[0]),
    [jobsPager.page, jobsPager.pageSize, state.jobs]
  );
  const paginatedVideos = useMemo(
    () => paginateItems(state.videos, videosPager.page, videosPager.pageSize, PAGE_SIZE_OPTIONS[0]),
    [state.videos, videosPager.page, videosPager.pageSize]
  );
  const rareCreatorOptions = useMemo(() => {
    const options = new Map();
    (Array.isArray(state.rareVideos) ? state.rareVideos : []).forEach((video) => {
      const creatorID = Number(video?.creatorId) || 0;
      if (!creatorID || options.has(creatorID)) {
        return;
      }
      options.set(creatorID, {
        id: creatorID,
        name: video?.creatorName || `博主 #${creatorID}`
      });
    });
    return [...options.values()].sort((left, right) => left.name.localeCompare(right.name, "zh-CN"));
  }, [state.rareVideos]);
  const filteredRareVideos = useMemo(() => {
    const creatorFilter = String(rareFilters.creatorId || "all");
    const keyword = String(rareFilters.keyword || "").trim().toLowerCase();
    const source = Array.isArray(state.rareVideos) ? state.rareVideos : [];

    return source
      .filter((video) => {
        if (creatorFilter !== "all" && String(video?.creatorId || "") !== creatorFilter) {
          return false;
        }
        if (!keyword) {
          return true;
        }
        const haystacks = [video?.title, video?.videoId, video?.creatorName]
          .map((value) => String(value || "").toLowerCase());
        return haystacks.some((value) => value.includes(keyword));
      })
      .sort((left, right) => {
        const timeDiff =
          (Date.parse(right?.outOfPrintAt || "") || Date.parse(right?.lastCheckAt || "") || 0) -
          (Date.parse(left?.outOfPrintAt || "") || Date.parse(left?.lastCheckAt || "") || 0);
        if (timeDiff !== 0) {
          return timeDiff;
        }
        return (Number(right?.id) || 0) - (Number(left?.id) || 0);
      });
  }, [rareFilters.creatorId, rareFilters.keyword, state.rareVideos]);
  const paginatedRareVideos = useMemo(
    () => paginateItems(filteredRareVideos, rareVideosPager.page, rareVideosPager.pageSize, PAGE_SIZE_OPTIONS[0]),
    [filteredRareVideos, rareVideosPager.page, rareVideosPager.pageSize]
  );
  const candidatePagination = useMemo(
    () =>
      resolvePagination(
        state.candidatePool?.total || 0,
        state.candidatePool?.page,
        state.candidatePool?.pageSize,
        PAGE_SIZE_OPTIONS[0]
      ),
    [state.candidatePool?.page, state.candidatePool?.pageSize, state.candidatePool?.total]
  );
  const selectedCandidateID = Number(state.candidatePool?.selectedID) || 0;
  const selectedCandidate = useMemo(() => {
    const detailCandidate = state.candidatePool?.detail?.candidate;
    if (detailCandidate?.id === selectedCandidateID) {
      return detailCandidate;
    }
    return candidateItems.find((item) => item.id === selectedCandidateID) || null;
  }, [candidateItems, selectedCandidateID, state.candidatePool?.detail]);
  const selectedJob = useMemo(() => {
    const jobs = paginatedJobs.items;
    return jobs.find((job) => job.id === selectedJobId) || jobs[0] || null;
  }, [paginatedJobs.items, selectedJobId]);

  useEffect(() => {
    saveState(state);
  }, [state]);

  useEffect(() => {
    candidateFiltersRef.current = state.candidatePool?.filters || DEFAULT_CANDIDATE_FILTERS;
  }, [state.candidatePool?.filters]);

  useEffect(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries
          .filter((entry) => entry.isIntersecting)
          .sort((a, b) => b.intersectionRatio - a.intersectionRatio)[0];
        if (visible?.target?.id) {
          setActiveSection(visible.target.id);
        }
      },
      { rootMargin: "-20% 0px -55% 0px", threshold: [0.2, 0.45, 0.7] }
    );

    sections.forEach((section) => {
      const node = document.getElementById(section.id);
      if (node) {
        observer.observe(node);
      }
    });

    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    let disposed = false;
    let stream = null;
    let systemTimer = 0;
    let snapshotTimer = 0;

    hasStreamOpenedRef.current = false;
    updateState((previous) => applyLiveEvent(previous, { type: "stream.connecting", data: {} }));

    const boot = async () => {
      await Promise.allSettled([
        syncDashboardFromAPI({ silent: true, withLog: false }),
        syncCandidatePool({ silent: true, withLog: false })
      ]);
      if (disposed) {
        return;
      }
      stream = createDashboardEventStream(state.apiBase, {
        onOpen: () => {
          const wasOpened = hasStreamOpenedRef.current;
          hasStreamOpenedRef.current = true;
          updateState((previous) => applyLiveEvent(previous, { type: "stream.live", data: {} }));
          if (wasOpened) {
            void Promise.allSettled([
              syncDashboardFromAPI({ silent: true, withLog: false }),
              syncCandidatePool({ silent: true, withLog: false })
            ]);
          }
        },
        onError: (error) => {
          const status = hasStreamOpenedRef.current ? "stream.reconnecting" : "stream.offline";
          updateState((previous) =>
            applyLiveEvent(previous, {
              type: status,
              data: { message: formatRequestError(error) }
            })
          );
        },
        onEvent: (event) => {
          if (AUTHORITATIVE_LIVE_EVENT_TYPES.has(event?.type)) {
            authoritativeStateVersionRef.current += 1;
          }
          updateState((previous) => applyLiveEvent(previous, event));
        }
      });

      systemTimer = window.setInterval(() => {
        void syncSystemStatus({ silent: true });
      }, SYSTEM_RECONCILE_INTERVAL_MS);
      snapshotTimer = window.setInterval(() => {
        void Promise.allSettled([
          syncDashboardFromAPI({ silent: true, withLog: false }),
          syncCandidatePool({ silent: true, withLog: false })
        ]);
      }, SNAPSHOT_RECONCILE_INTERVAL_MS);
    };

    void boot();
    void loadSystemConfig({ silent: true });

    return () => {
      disposed = true;
      window.clearInterval(systemTimer);
      window.clearInterval(snapshotTimer);
      stream?.close();
    };
  }, [state.apiBase]);

  useEffect(() => {
    setSelectedJobId((current) => {
      const jobs = paginatedJobs.items;
      if (jobs.length === 0) {
        return null;
      }
      if (jobs.some((job) => job.id === current)) {
        return current;
      }
      return jobs[0].id;
    });
  }, [paginatedJobs.items]);

  useEffect(() => {
    if (!configRestartState.active) {
      return undefined;
    }

    let cancelled = false;
    let timer = 0;
    const startedAt = Number(configRestartState.startedAt || Date.now());

    const poll = async () => {
      try {
        await getSystemStatus(state.apiBase);
        if (cancelled) {
          return;
        }

        setConfigRestartState({
          active: false,
          startedAt: 0,
          recoveredAt: Date.now(),
          lastError: ""
        });
        setConfigValidation({
          tone: "success",
          title: "后端已恢复并重新加载配置",
          detail: "检测到后端已经恢复响应，页面会继续自动同步系统状态；如需二次确认，可直接查看当前编辑器内容。"
        });
        pushLog("后端重启完成，配置联动已恢复");
        showToast("后端已恢复");

        await Promise.allSettled([
          syncDashboardFromAPI({ silent: true, withLog: false }),
          loadSystemConfig({ silent: true, preserveValidation: true })
        ]);
      } catch (error) {
        if (cancelled) {
          return;
        }

        const message = formatRequestError(error);
        if (Date.now() - startedAt >= CONFIG_RESTART_TIMEOUT_MS) {
          setConfigRestartState({
            active: false,
            startedAt: 0,
            recoveredAt: 0,
            lastError: message
          });
          setConfigValidation({
            tone: "error",
            title: "配置已保存，但等待后端恢复超时",
            detail: `配置文件已经写回，但在 45 秒内未检测到后端恢复。请稍后点击“重新加载”，或检查 docker compose logs app --tail=200。最后一次错误：${message}`
          });
          pushLog(`等待后端重启恢复超时: ${message}`);
          showToast("等待后端恢复超时");
          return;
        }

        setConfigRestartState((previous) => ({
          ...previous,
          lastError: message
        }));
        timer = window.setTimeout(() => {
          void poll();
        }, CONFIG_RESTART_POLL_INTERVAL_MS);
      }
    };

    timer = window.setTimeout(() => {
      void poll();
    }, CONFIG_RESTART_POLL_INTERVAL_MS);

    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [configRestartState.active, configRestartState.startedAt, state.apiBase]);

  function updateState(updater) {
    setState((previous) => (typeof updater === "function" ? updater(previous) : updater));
  }

  function pushLog(message) {
    updateState((previous) => ({
      ...previous,
      logs: [makeLog(message), ...(previous.logs || [])].slice(0, 18)
    }));
  }

  function applyConfigDocument(payload) {
    const nextContent = String(payload?.content || "");
    setConfigPath(String(payload?.path || ""));
    setConfigText(nextContent);
    setSavedConfigText(nextContent);
  }

  function showToast(message) {
    setToast(message);
    window.clearTimeout(showToast.timer);
    showToast.timer = window.setTimeout(() => setToast(""), 2200);
  }

  async function syncAllData({ silent = false } = {}) {
    const [dashboardResult, candidateResult] = await Promise.allSettled([
      syncDashboardFromAPI({ silent, withLog: !silent }),
      syncCandidatePool({ silent, withLog: false })
    ]);

    if (candidateResult.status === "rejected") {
      const message = formatRequestError(candidateResult.reason);
      pushLog(`刷新候选池失败: ${message}`);
      if (!silent) {
        showToast(message);
      }
    }

    return dashboardResult;
  }

  async function syncDashboardFromAPI({ silent = false, withLog = !silent } = {}) {
    const requestMeta = beginVersionedRequest(snapshotRequestVersionRef, authoritativeStateVersionRef);
    if (!silent) {
      setBusyAction("sync");
    }
    try {
      const snapshot = await loadDashboardSnapshot(state.apiBase);
      if (!isVersionedRequestCurrent(snapshotRequestVersionRef, authoritativeStateVersionRef, requestMeta)) {
        return;
      }
      authoritativeStateVersionRef.current += 1;
      const syncAt = nowLabel();
      updateState((previous) => ({
        ...applyRemoteSnapshot(previous, snapshot, syncAt),
        logs: withLog
          ? [
              makeLog(
                `已同步真实数据: ${snapshot.creators.length} 个博主 / ${snapshot.jobs.length} 个任务 / ${snapshot.videos.length} 个视频 / ${snapshot.rareVideos.length} 个绝版`
              ),
              ...(previous.logs || [])
            ].slice(0, 18)
          : previous.logs || []
      }));
      if (!silent) {
        showToast("同步完成");
      }
    } catch (error) {
      const message = formatRequestError(error);
      updateState((previous) => ({
        ...previous,
        system: {
          ...previous.system,
          health: "degraded"
        },
        logs: withLog ? [makeLog(`同步失败: ${message}`), ...(previous.logs || [])].slice(0, 18) : previous.logs || []
      }));
      if (!silent) {
        showToast(message);
      }
    } finally {
      if (!silent) {
        setBusyAction("");
      }
    }
  }

  async function syncCandidatePool({
    silent = false,
    withLog = !silent,
    filters = candidateFiltersRef.current,
    page = state.candidatePool?.page,
    pageSize = state.candidatePool?.pageSize
  } = {}) {
    const requestVersion = beginRequest(candidateRequestVersionRef);
    if (!silent) {
      setCandidateLoading(true);
    }
    try {
      const appliedFilters = {
        status: String(filters?.status || ""),
        minScore: Number(filters?.minScore) || 0,
        keyword: String(filters?.keyword || "")
      };
      const requestedPage = normalizePositiveInteger(page, Number(state.candidatePool?.page) || 1);
      const requestedPageSize = normalizePositiveInteger(pageSize, Number(state.candidatePool?.pageSize) || PAGE_SIZE_OPTIONS[0]);
      let payload = await listCandidateCreators(state.apiBase, {
        page: requestedPage,
        page_size: requestedPageSize,
        status: appliedFilters.status,
        min_score: appliedFilters.minScore,
        keyword: appliedFilters.keyword
      });
      const resolvedPage = resolvePagination(payload.total, requestedPage, requestedPageSize, requestedPageSize);
      if (payload.total > 0 && payload.items.length === 0 && requestedPage > resolvedPage.totalPages) {
        payload = await listCandidateCreators(state.apiBase, {
          page: resolvedPage.totalPages,
          page_size: requestedPageSize,
          status: appliedFilters.status,
          min_score: appliedFilters.minScore,
          keyword: appliedFilters.keyword
        });
      }
      if (!isRequestCurrent(candidateRequestVersionRef, requestVersion)) {
        return;
      }

      const syncAt = nowLabel();
      updateState((previous) => ({
        ...applyCandidateListSnapshot(
          {
            ...previous,
            candidatePool: {
              ...previous.candidatePool,
              filters: appliedFilters
            }
          },
          payload,
          syncAt
        ),
        logs: withLog
          ? [makeLog(`已同步候选池: ${payload.items.length} 个候选 / 总数 ${payload.total}`), ...(previous.logs || [])].slice(0, 18)
          : previous.logs || []
      }));
    } catch (error) {
      const message = formatRequestError(error);
      if (withLog) {
        pushLog(`刷新候选池失败: ${message}`);
      }
      if (!silent) {
        throw error;
      }
    } finally {
      if (!silent) {
        setCandidateLoading(false);
      }
    }
  }

  async function loadCandidateDetail(id, { silent = false } = {}) {
    const candidateID = Number(id) || 0;
    if (candidateID <= 0) {
      return;
    }
    const requestVersion = beginRequest(candidateDetailRequestVersionRef);
    updateState((previous) => ({
      ...previous,
      candidatePool: {
        ...previous.candidatePool,
        selectedID: candidateID
      }
    }));
    if (!silent) {
      setCandidateDetailLoading(true);
    }
    try {
      const payload = await getCandidateCreator(state.apiBase, candidateID);
      if (!isRequestCurrent(candidateDetailRequestVersionRef, requestVersion)) {
        return;
      }
      updateState((previous) => applyCandidateDetailSnapshot(previous, payload));
    } catch (error) {
      const message = formatRequestError(error);
      pushLog(`加载候选详情失败: ${message}`);
      if (!silent) {
        showToast(message);
      }
    } finally {
      if (!silent) {
        setCandidateDetailLoading(false);
      }
    }
  }

  async function syncSystemStatus({ silent = false } = {}) {
    try {
      const requestMeta = beginVersionedRequest(systemRequestVersionRef, authoritativeStateVersionRef);
      const payload = await getSystemStatus(state.apiBase);
      if (!isVersionedRequestCurrent(systemRequestVersionRef, authoritativeStateVersionRef, requestMeta)) {
        return;
      }
      authoritativeStateVersionRef.current += 1;
      updateState((previous) => applySystemStatusSnapshot(previous, payload));
    } catch (error) {
      if (!silent) {
        const message = formatRequestError(error);
        pushLog(`刷新系统状态失败: ${message}`);
        showToast(message);
      }
    }
  }

  async function refreshAfterMutation(successMessage) {
    const requestMeta = beginVersionedRequest(snapshotRequestVersionRef, authoritativeStateVersionRef);
    const snapshot = await loadDashboardSnapshot(state.apiBase);
    if (!isVersionedRequestCurrent(snapshotRequestVersionRef, authoritativeStateVersionRef, requestMeta)) {
      pushLog(successMessage);
      return;
    }
    authoritativeStateVersionRef.current += 1;
    const syncAt = nowLabel();
    updateState((previous) => ({
      ...applyRemoteSnapshot(previous, snapshot, syncAt),
      logs: [makeLog(successMessage), ...(previous.logs || [])].slice(0, 18)
    }));
  }

  async function refreshCandidateAfterMutation(successMessage, { includeDashboard = false, detailID = selectedCandidateID } = {}) {
    await syncCandidatePool({ silent: true, withLog: false });
    if (detailID > 0) {
      await loadCandidateDetail(detailID, { silent: true });
    }
    if (includeDashboard) {
      await refreshAfterMutation(successMessage);
      return;
    }
    pushLog(successMessage);
  }

  async function loadSystemConfig({ silent = false, preserveValidation = false } = {}) {
    setConfigLoading(true);
    try {
      const payload = await getSystemConfig(state.apiBase);
      applyConfigDocument(payload);
      if (!preserveValidation) {
        setConfigValidation({
          tone: "success",
          title: "配置已加载",
          detail: "当前编辑器内容已经和后端配置文件同步。修改后保存时会再次执行校验。"
        });
      }
      if (!silent) {
        showToast("配置已加载");
      }
    } catch (error) {
      const message = formatRequestError(error);
      if (!preserveValidation) {
        setConfigValidation({
          tone: "error",
          title: "配置加载失败",
          detail: message
        });
      }
      pushLog(`加载配置失败: ${message}`);
      if (!silent) {
        showToast(message);
      }
    } finally {
      setConfigLoading(false);
    }
  }

  async function handleSaveConfig() {
    if (configText === savedConfigText) {
      setConfigRestartState((previous) => ({
        ...previous,
        active: false,
        startedAt: 0,
        lastError: ""
      }));
      setConfigValidation({
        tone: "idle",
        title: "配置未变化",
        detail: "当前编辑内容和后端文件一致，无需重启后端。"
      });
      showToast("配置未变化");
      return;
    }

    setConfigSaving(true);
    try {
      const result = await updateSystemConfig(state.apiBase, configText);
      setSavedConfigText(configText);
      if (result?.path) {
        setConfigPath(String(result.path));
      }
      if (result?.changed) {
        setConfigRestartState({
          active: true,
          startedAt: Date.now(),
          recoveredAt: 0,
          lastError: ""
        });
      } else {
        setConfigRestartState((previous) => ({
          ...previous,
          active: false,
          startedAt: 0,
          lastError: ""
        }));
      }
      setConfigValidation(
        result?.changed
          ? {
              tone: "warning",
              title: "配置校验通过并已保存，后端正在重启",
              detail: "配置文件已写回磁盘。后端在短暂重启期间可能暂时不可用，页面会自动检测服务恢复并继续同步。"
            }
          : {
              tone: "idle",
              title: "配置未变化",
              detail: "保存请求已处理，但内容与原文件一致。"
            }
      );
      pushLog(result?.changed ? `配置已保存，后端准备重启: ${result.path || configPath || "-"}` : "配置未变化");
      showToast(result?.changed ? "配置已保存，后端正在重启" : "配置未变化");
    } catch (error) {
      const message = formatRequestError(error);
      setConfigRestartState((previous) => ({
        ...previous,
        active: false,
        startedAt: 0,
        lastError: message
      }));
      setConfigValidation({
        tone: "error",
        title: "配置校验失败",
        detail: message
      });
      pushLog(`保存配置失败: ${message}`);
      showToast(message);
    } finally {
      setConfigSaving(false);
    }
  }

  async function handleCreateCreator(event) {
    event.preventDefault();
    if (!form.uid.trim() && !form.name.trim()) {
      showToast("请填写 UID 或名称");
      return;
    }

    setBusyAction("create");
    try {
      await createCreator(state.apiBase, form);
      await refreshAfterMutation(`已通过 API 添加博主: ${form.name || form.uid}`);
      showToast("博主已添加");
      setForm({ uid: "", name: "", platform: "bilibili", status: "active" });
    } catch (error) {
      const message = formatRequestError(error);
      pushLog(`添加失败: ${message}`);
      showToast(message);
    } finally {
      setBusyAction("");
    }
  }

  function scrollToSection(id) {
    document.getElementById(id)?.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  async function handleAction(type) {
    setBusyAction(type);
    try {
      await enqueueJob(state.apiBase, type);
      await refreshAfterMutation(`已触发任务: ${jobText(type)}`);
      showToast("任务已入队");
    } catch (error) {
      const message = formatRequestError(error);
      pushLog(`触发任务失败: ${message}`);
      showToast(message);
    } finally {
      setBusyAction("");
    }
  }

  async function toggleCreator(id) {
    const creator = state.creators.find((item) => item.id === id);
    if (!creator) {
      showToast("博主不存在");
      return;
    }

    const nextStatus = creator.status === "active" ? "paused" : "active";
    setBusyAction(`toggle-${id}`);
    try {
      await patchCreator(state.apiBase, id, { status: nextStatus });
      await refreshAfterMutation(`已通过 API 更新博主状态: #${id} -> ${nextStatus}`);
      showToast(nextStatus === "active" ? "已启用" : "已暂停");
    } catch (error) {
      const message = formatRequestError(error);
      pushLog(`更新博主状态失败: ${message}`);
      showToast(message);
    } finally {
      setBusyAction("");
    }
  }

  async function removeCreator(id) {
    setBusyAction(`remove-${id}`);
    try {
      await deleteCreator(state.apiBase, id);
      await refreshAfterMutation(`已通过 API 停止追踪博主: #${id}`);
      showToast("已停止追踪");
    } catch (error) {
      const message = formatRequestError(error);
      pushLog(`停止追踪失败: ${message}`);
      showToast(message);
    } finally {
      setBusyAction("");
    }
  }

  function updateCandidateFilters(patch) {
    updateState((previous) => ({
      ...previous,
      candidatePool: {
        ...previous.candidatePool,
        filters: {
          ...(previous.candidatePool?.filters || {}),
          ...patch
        }
      }
    }));
  }

  function updateLocalPager(key, patch) {
    updateState((previous) => {
      const items =
        key === "creators"
          ? previous.creators
          : key === "jobs"
            ? previous.jobs
            : key === "videos"
              ? previous.videos
              : key === "rareVideos"
                ? filteredRareVideos
                : [];
      const currentPager = previous.pagination?.[key] || { page: 1, pageSize: PAGE_SIZE_OPTIONS[0] };
      const nextPageSize = normalizePositiveInteger(patch?.pageSize, currentPager.pageSize);
      const requestedPage = normalizePositiveInteger(patch?.page, currentPager.page);
      const nextPagination = resolvePagination(items.length, requestedPage, nextPageSize, PAGE_SIZE_OPTIONS[0]);

      return {
        ...previous,
        pagination: {
          ...(previous.pagination || {}),
          [key]: {
            page: nextPagination.page,
            pageSize: nextPagination.pageSize
          }
        }
      };
    });
  }

  function updateRareFilters(patch) {
    setRareFilters((previous) => ({
      ...previous,
      ...patch
    }));
    updateLocalPager("rareVideos", { page: 1 });
  }

  async function handleCandidateFilterSubmit(event) {
    event.preventDefault();
    try {
      await syncCandidatePool({
        filters: candidateFilters,
        page: 1,
        pageSize: state.candidatePool?.pageSize,
        silent: false
      });
      showToast("候选池已刷新");
    } catch (error) {
      const message = formatRequestError(error);
      pushLog(`候选筛选失败: ${message}`);
      showToast(message);
    }
  }

  async function handleCandidateRefresh() {
    try {
      await syncCandidatePool({ silent: false });
      showToast("候选池已刷新");
    } catch (error) {
      const message = formatRequestError(error);
      pushLog(`刷新候选池失败: ${message}`);
      showToast(message);
    }
  }

  async function changeCandidatePage(nextPage) {
    try {
      await syncCandidatePool({ silent: false, withLog: false, page: nextPage });
    } catch (error) {
      const message = formatRequestError(error);
      pushLog(`切换候选分页失败: ${message}`);
      showToast(message);
    }
  }

  async function changeCandidatePageSize(nextPageSize) {
    try {
      await syncCandidatePool({ silent: false, withLog: false, page: 1, pageSize: nextPageSize });
    } catch (error) {
      const message = formatRequestError(error);
      pushLog(`更新候选每页条数失败: ${message}`);
      showToast(message);
    }
  }

  async function openCandidateDrawer(id) {
    await loadCandidateDetail(id);
  }

  function closeCandidateDrawer() {
    setCandidateDetailLoading(false);
    updateState((previous) => ({
      ...previous,
      candidatePool: {
        ...previous.candidatePool,
        selectedID: 0,
        detail: null
      }
    }));
  }

  async function handleCandidateDiscover() {
    setBusyAction("candidate-discover");
    try {
      await triggerCandidateDiscover(state.apiBase);
      await syncCandidatePool({ silent: true, withLog: false });
      await refreshAfterMutation("已触发候选发现任务");
      showToast("候选发现已入队");
    } catch (error) {
      const message = formatRequestError(error);
      pushLog(`触发候选发现失败: ${message}`);
      showToast(message);
    } finally {
      setBusyAction("");
    }
  }

  async function handleCandidateDecision(id, action) {
    const actionKey = `candidate-${action}-${id}`;
    setBusyAction(actionKey);
    try {
      if (action === "approve") {
        await approveCandidateCreator(state.apiBase, id);
        updateState((previous) => applyCandidateReviewAction(previous, id, "approved"));
        await refreshCandidateAfterMutation(`候选已加入正式追踪: #${id}`, { includeDashboard: true, detailID: id });
        showToast("已加入追踪");
        return;
      }

      const actionMap = {
        ignore: {
          request: ignoreCandidateCreator,
          status: "ignored",
          success: "候选已忽略",
          toast: "已忽略"
        },
        block: {
          request: blockCandidateCreator,
          status: "blocked",
          success: "候选已拉黑",
          toast: "已拉黑"
        },
        review: {
          request: reviewCandidateCreator,
          status: "reviewing",
          success: "候选已恢复审核",
          toast: "已恢复审核"
        }
      };

      const current = actionMap[action];
      if (!current) {
        showToast("候选动作不存在");
        return;
      }

      await current.request(state.apiBase, id);
      updateState((previous) => applyCandidateReviewAction(previous, id, current.status));
      await refreshCandidateAfterMutation(`${current.success}: #${id}`, { detailID: id });
      showToast(current.toast);
    } catch (error) {
      const message = formatRequestError(error);
      pushLog(`候选操作失败: ${message}`);
      showToast(message);
    } finally {
      setBusyAction("");
    }
  }

  const storagePercent = `${metrics.storagePercent}%`;
  const healthLabel = healthText(state.system.health);
  const connectionLabel = connectionText(state.connection?.status);
  const connectionStatusClass = connectionStatusClassName(state.connection?.status);
  const cookieLabel = cookieText(state.system.cookieStatus, state.system.cookieConfigured);
  const cookieSourceLabel = cookieSourceText(state.system.cookieSource);
  const riskBackoffLabel = riskBackoffText(state.system.riskActive, state.system.riskBackoffSeconds);
  const cleanupPressureBytes = Math.max(0, Number(state.storage.usedBytes || 0) - Number(state.storage.safeBytes || 0));
  const configDirty = configText !== savedConfigText;
  const configRestarting = configRestartState.active;
  const candidateDrawerOpen = selectedCandidateID > 0;
  const candidateDrawerDetail = state.candidatePool?.detail;

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand-panel">
          <div className="brand-mark">BV</div>
          <div>
            <p className="eyebrow">Bili Vault</p>
            <h1 className="brand-name">绝版视频库</h1>
          </div>
        </div>

        <div className="status-panel">
          <div className={`health-dot health-dot--${state.system.health}`} />
          <div>
            <div className="status-label">后端接口</div>
            <div className="status-meta">{healthLabel}</div>
          </div>
        </div>

        <nav className="nav">
          {sections.map((section) => (
            <button
              key={section.id}
              type="button"
              className={section.id === activeSection ? "nav-link nav-link--active" : "nav-link"}
              onClick={() => scrollToSection(section.id)}
            >
              <span>{section.label}</span>
            </button>
          ))}
        </nav>

        <div className="sidebar-actions">
          <p className="sidebar-caption">快捷动作</p>
          {quickActions.map((action) => (
            <button
              key={action.type}
              type="button"
              className="secondary-button"
              onClick={() => handleAction(action.type)}
              disabled={busyAction === action.type}
            >
              {busyAction === action.type ? "处理中..." : action.label}
            </button>
          ))}
        </div>
      </aside>

      <main className="workspace">
        <header className="command-bar">
          <div>
            <p className="eyebrow">系统总览</p>
            <h2>绝版视频库</h2>
            <p className="command-copy">用于查看博主追踪、任务执行、视频状态与存储情况；当前数据来自后端接口。</p>
            <div className={`live-connection ${connectionStatusClass}`} data-testid="live-connection-status">
              <span className="live-connection-dot" />
              <span>实时连接状态：{connectionLabel}</span>
            </div>
          </div>
          <div className="command-actions">
            <button
              type="button"
              className="secondary-button"
              data-testid="sync-button"
              onClick={() => void syncAllData()}
              disabled={busyAction === "sync"}
            >
              {busyAction === "sync" ? "同步中..." : "同步数据"}
            </button>
            <button type="button" className="primary-button" onClick={() => handleAction("fetch")}>
              立即拉取
            </button>
          </div>
        </header>

        <section id="overview" className="section surface surface--highlight">
          <div className="section-header">
            <div>
              <p className="section-kicker">系统概况</p>
              <h3>系统概况</h3>
            </div>
            <span className="pill">最近同步: {state.system.lastSyncAt}</span>
          </div>

          <div className="metric-grid">
            <MetricCard label="已维护博主" value={metrics.creators} detail="配置文件与 HTTP 接口" />
            <MetricCard label="待处理任务" value={metrics.pendingJobs} detail="队列中与执行中" />
            <MetricCard label="绝版视频" value={metrics.rareVideos} detail="已标记绝版的视频" />
            <MetricCard
              label="存储占用"
              value={storagePercent}
              detail={`${formatBytes(state.storage.usedBytes)} / ${formatBytes(state.storage.limitBytes)}`}
            />
          </div>

          <div className="overview-grid">
            <div className="panel">
              <div className="panel-header">
                <div>
                  <p className="section-kicker">运行态</p>
                  <h4>最近任务队列</h4>
                </div>
                <span className="pill pill--soft">{state.system.activeJobs} 个活跃任务</span>
              </div>
              <div className="stack-list">
                {state.jobs.length > 0 ? (
                  state.jobs.slice(0, 4).map((job) => (
                    <div className="stack-row" key={job.id}>
                      <div>
                        <div className="row-title">{jobText(job.type)}</div>
                        <div className="row-meta">{jobMeta(job)}</div>
                      </div>
                      <StatusBadge status={job.status} />
                    </div>
                  ))
                ) : (
                  <p className="panel-note">当前没有任务记录</p>
                )}
              </div>
            </div>

            <div className="panel panel--tall">
              <div className="panel-header">
                <div>
                  <p className="section-kicker">系统状态</p>
                  <h4>系统状态</h4>
                </div>
                <span className="pill">风险 {state.system.riskLevel}</span>
              </div>
              <div className="signal-grid">
                <SignalItem label="数据来源" value="后端接口" />
                <SignalItem label="系统健康" value={healthLabel} />
                <SignalItem label="热点目录" value={state.storage.hottestBucket || "-"} />
                <SignalItem label="最近任务" value={formatDateTime(state.system.lastJobAt)} />
              </div>
            </div>
          </div>
        </section>

        <section id="creators" className="section section-grid">
          <div className="panel panel--span">
            <div className="panel-header">
              <div>
                <p className="section-kicker">博主管理</p>
                <h3>博主管理与追踪状态</h3>
              </div>
              <span className="pill pill--soft">列表来自后端</span>
            </div>

            <form className="creator-form" onSubmit={handleCreateCreator}>
              <label>
                UID
                <input
                  value={form.uid}
                  onChange={(event) => setForm((previous) => ({ ...previous, uid: event.target.value }))}
                  placeholder="如: 123456"
                />
              </label>
              <label>
                名称
                <input
                  value={form.name}
                  onChange={(event) => setForm((previous) => ({ ...previous, name: event.target.value }))}
                  placeholder="如: 示例博主"
                />
              </label>
              <label>
                平台
                <select
                  value={form.platform}
                  onChange={(event) => setForm((previous) => ({ ...previous, platform: event.target.value }))}
                >
                  {platformOptions.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                状态
                <select
                  value={form.status}
                  onChange={(event) => setForm((previous) => ({ ...previous, status: event.target.value }))}
                >
                  <option value="active">启用</option>
                  <option value="paused">暂停</option>
                </select>
              </label>
              <button type="submit" className="primary-button" data-testid="creator-submit" disabled={busyAction === "create"}>
                {busyAction === "create" ? "提交中..." : "添加博主"}
              </button>
            </form>
          </div>

          <div className="panel panel--span">
            <div className="panel-header panel-header--compact">
              <div>
                <p className="section-kicker">追踪列表</p>
                <h3>已追踪博主</h3>
              </div>
              <PaginationControls
                page={paginatedCreators.page}
                pageSize={paginatedCreators.pageSize}
                total={paginatedCreators.total}
                totalPages={paginatedCreators.totalPages}
                onPageChange={(page) => updateLocalPager("creators", { page })}
                onPageSizeChange={(pageSize) => updateLocalPager("creators", { page: 1, pageSize })}
              />
            </div>
            <div className="table-header">
              <span>UID</span>
              <span>名称</span>
              <span>平台</span>
              <span>本地视频</span>
              <span>占用空间</span>
              <span>状态</span>
              <span>操作</span>
            </div>
            <div className="table-body" data-testid="creator-list">
              {paginatedCreators.items.map((creator) => (
                <div className="table-row" key={creator.id}>
                  <span>{creator.uid || "-"}</span>
                  <span>{creator.name || "-"}</span>
                  <span>{creator.platform}</span>
                  <span>{creator.localVideoCount}</span>
                  <span>{formatBytes(creator.storageBytes)}</span>
                  <span>{creator.status}</span>
                  <span className="row-actions">
                    <button
                      type="button"
                      className="ghost-link"
                      onClick={() => toggleCreator(creator.id)}
                      disabled={busyAction === `toggle-${creator.id}`}
                    >
                      {creator.status === "active" ? "暂停" : "启用"}
                    </button>
                    <button
                      type="button"
                      className="ghost-link"
                      onClick={() => removeCreator(creator.id)}
                      disabled={busyAction === `remove-${creator.id}`}
                    >
                      停止追踪
                    </button>
                  </span>
                </div>
              ))}
            </div>
          </div>
        </section>

        <section id="candidates" className="section surface surface--highlight candidate-surface">
          <div className="section-header">
            <div>
              <p className="section-kicker">种子池</p>
              <h3>候选池与人工审核</h3>
              <p className="panel-note">用于查看发现链路产出的候选博主、评分拆解与人工审核状态。</p>
            </div>
            <div className="command-actions">
              <span className="pill pill--soft">候选同步: {state.candidatePool?.lastSyncAt || "未同步"}</span>
              <button
                type="button"
                className="secondary-button"
                onClick={() => void handleCandidateRefresh()}
                disabled={candidateLoading}
              >
                {candidateLoading ? "刷新中..." : "刷新候选"}
              </button>
              <button
                type="button"
                className="primary-button"
                onClick={() => void handleCandidateDiscover()}
                disabled={busyAction === "candidate-discover"}
              >
                {busyAction === "candidate-discover" ? "发现中..." : "手动发现"}
              </button>
            </div>
          </div>

          <div className="metric-grid metric-grid--candidates">
            <MetricCard label="新候选数" value={candidateInsights.reviewingCount} detail="当前待人工审核" />
            <MetricCard label="高优候选数" value={candidateInsights.highPriorityCount} detail="评分 >= 80 且仍在审核中" />
            <MetricCard label="今日发现数" value={candidateInsights.discoveredTodayCount} detail="按最近发现时间统计" />
            <MetricCard label="已忽略数" value={candidateInsights.ignoredCount} detail="已人工筛除，仍保留在候选池" />
          </div>

          <div className="panel panel--span" style={{ marginTop: 18 }}>
            <div className="panel-header">
              <div>
                <p className="section-kicker">筛选与审核</p>
                <h3>候选筛选与审核队列</h3>
              </div>
              <span className="pill pill--soft">总数 {state.candidatePool?.total || 0}</span>
            </div>

            <form className="candidate-filters" onSubmit={handleCandidateFilterSubmit}>
              <label>
                状态
                <select
                  value={candidateFilters.status}
                  onChange={(event) => updateCandidateFilters({ status: event.target.value })}
                >
                  <option value="">全部</option>
                  <option value="reviewing">审核中</option>
                  <option value="ignored">已忽略</option>
                  <option value="approved">已批准</option>
                  <option value="blocked">已拉黑</option>
                </select>
              </label>
              <label>
                最低分
                <select
                  value={candidateFilters.minScore}
                  onChange={(event) => updateCandidateFilters({ minScore: Number(event.target.value) || 0 })}
                >
                  <option value="0">不限</option>
                  <option value="60">60 分</option>
                  <option value="80">80 分</option>
                  <option value="90">90 分</option>
                </select>
              </label>
              <label className="candidate-filters__keyword">
                关键词
                <input
                  aria-label="候选关键词筛选"
                  value={candidateFilters.keyword}
                  onChange={(event) => updateCandidateFilters({ keyword: event.target.value })}
                  placeholder="名称 / UID / 来源标签"
                />
              </label>
              <button
                type="submit"
                className="secondary-button"
                data-testid="candidate-filter-submit"
                disabled={candidateLoading}
              >
                {candidateLoading ? "查询中..." : "筛选候选"}
              </button>
            </form>

            <div className="candidate-band-strip">
              <DetailStat label="高分" value={candidateInsights.scoreBands.high} tone="success" />
              <DetailStat label="中分" value={candidateInsights.scoreBands.medium} />
              <DetailStat label="低分" value={candidateInsights.scoreBands.low} tone="danger" />
            </div>

            <div className="candidate-list" data-testid="candidate-list">
              {candidateItems.length > 0 ? (
                candidateItems.map((candidate) => (
                  <CandidateRow
                    key={`candidate-${candidate.id}`}
                    candidate={candidate}
                    busyAction={busyAction}
                    onView={() => void openCandidateDrawer(candidate.id)}
                    onApprove={() => void handleCandidateDecision(candidate.id, "approve")}
                    onIgnore={() => void handleCandidateDecision(candidate.id, "ignore")}
                    onBlock={() => void handleCandidateDecision(candidate.id, "block")}
                    onReview={() => void handleCandidateDecision(candidate.id, "review")}
                  />
                ))
              ) : (
                <p className="panel-note">当前没有候选，请先触发一次发现任务，或调整筛选条件。</p>
              )}
            </div>

            <PaginationControls
              page={candidatePagination.page}
              pageSize={candidatePagination.pageSize}
              total={candidatePagination.total}
              totalPages={candidatePagination.totalPages}
              disabled={candidateLoading}
              onPageChange={(page) => void changeCandidatePage(page)}
              onPageSizeChange={(pageSize) => void changeCandidatePageSize(pageSize)}
            />
          </div>
        </section>

        <section id="tasks" className="section section-grid">
          <div className="panel">
            <div className="panel-header">
              <div>
                <p className="section-kicker">任务控制</p>
                <h3>任务操作与队列明细</h3>
              </div>
              <span className="pill pill--soft">失败 {taskDiagnostics.failedCount}</span>
            </div>
            <div className="action-row">
              {quickActions.map((action) => (
                <button
                  key={action.type}
                  type="button"
                  className="secondary-button"
                  data-testid={action.type === "fetch" ? "quick-action-fetch" : undefined}
                  onClick={() => handleAction(action.type)}
                  disabled={busyAction === action.type}
                >
                  {busyAction === action.type ? "处理中..." : action.label}
                </button>
              ))}
            </div>
            <div className="status-strip" style={{ marginTop: 16 }}>
              <DetailStat label="待执行" value={taskDiagnostics.queuedCount} />
              <DetailStat label="运行中" value={taskDiagnostics.runningCount} />
              <DetailStat label="失败任务" value={taskDiagnostics.failedCount} tone="danger" />
            </div>
            <PaginationControls
              page={paginatedJobs.page}
              pageSize={paginatedJobs.pageSize}
              total={paginatedJobs.total}
              totalPages={paginatedJobs.totalPages}
              onPageChange={(page) => updateLocalPager("jobs", { page })}
              onPageSizeChange={(pageSize) => updateLocalPager("jobs", { page: 1, pageSize })}
              className="pagination-controls--top"
            />
            <div className="stack-list" style={{ marginTop: 16 }} data-testid="job-list">
              {paginatedJobs.items.length > 0 ? (
                paginatedJobs.items.map((job) => (
                  <button
                    type="button"
                    className={job.id === selectedJob?.id ? "stack-select stack-select--active" : "stack-select"}
                    key={`task-${job.id}`}
                    onClick={() => setSelectedJobId(job.id)}
                  >
                    <div className="stack-copy">
                      <div className="row-title">{jobText(job.type)}</div>
                      <div className="row-meta">{jobMeta(job)}</div>
                      <div className={job.errorMsg ? "row-submeta row-submeta--danger" : "row-submeta"}>
                        {job.errorMsg ? `失败原因: ${truncateText(job.errorMsg, 56)}` : `更新时间: ${formatDateTime(job.updatedAt)}`}
                      </div>
                    </div>
                    <StatusBadge status={job.status} />
                  </button>
                ))
              ) : (
                <p className="panel-note">暂无任务数据</p>
              )}
            </div>
          </div>

          <div className="panel" data-testid="job-detail-panel">
            <div className="panel-header">
              <div>
                <p className="section-kicker">任务详情</p>
                <h3>{selectedJob ? jobText(selectedJob.type) : "选择任务查看详情"}</h3>
              </div>
              {selectedJob ? <StatusBadge status={selectedJob.status} /> : null}
            </div>
            {selectedJob ? (
              <>
                <div className="detail-grid">
                  <DetailItem label="任务 ID" value={`#${selectedJob.id}`} />
                  <DetailItem label="类型" value={jobText(selectedJob.type)} />
                  <DetailItem label="创建时间" value={formatDateTime(selectedJob.createdAt)} />
                  <DetailItem label="更新时间" value={formatDateTime(selectedJob.updatedAt)} />
                  <DetailItem label="开始时间" value={formatDateTime(selectedJob.startedAt)} />
                  <DetailItem label="完成时间" value={formatDateTime(selectedJob.finishedAt)} />
                </div>

                <div className="detail-block">
                  <span className="signal-label">失败原因</span>
                  <div className={selectedJob.errorMsg ? "reason-box reason-box--danger" : "reason-box"}>
                    {selectedJob.errorMsg
                      ? selectedJob.errorMsg
                      : taskDiagnostics.latestFailure
                        ? `最近失败任务 #${taskDiagnostics.latestFailure.id}: ${taskDiagnostics.latestFailure.errorMsg || "无错误文本"}`
                        : "当前没有失败记录"}
                  </div>
                </div>

                <div className="detail-block">
                  <span className="signal-label">Payload</span>
                  {payloadRows(selectedJob.payload).length > 0 ? (
                    <div className="payload-list">
                      {payloadRows(selectedJob.payload).map((item) => (
                        <div className="payload-row" key={`${selectedJob.id}-${item.key}`}>
                          <span className="payload-key">{item.key}</span>
                          <span className="payload-value">{item.value}</span>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="panel-note">当前任务没有附带 payload。</p>
                  )}
                </div>
              </>
            ) : (
              <p className="panel-note">暂无任务数据</p>
            )}
          </div>

          <div className="panel panel--span">
            <div className="panel-header">
              <div>
                <p className="section-kicker">最新视频</p>
                <h3>最近视频</h3>
              </div>
              <PaginationControls
                page={paginatedVideos.page}
                pageSize={paginatedVideos.pageSize}
                total={paginatedVideos.total}
                totalPages={paginatedVideos.totalPages}
                onPageChange={(page) => updateLocalPager("videos", { page })}
                onPageSizeChange={(pageSize) => updateLocalPager("videos", { page: 1, pageSize })}
              />
            </div>
            <div className="stack-list">
              {paginatedVideos.items.length > 0 ? (
                paginatedVideos.items.map((video) => (
                  <div className="stack-row" key={video.id}>
                    <div>
                      <div className="row-title">{video.title || video.videoId || `视频 #${video.id}`}</div>
                      <div className="row-meta">{videoMeta(video)}</div>
                    </div>
                    <VideoStateBadge state={video.state} />
                  </div>
                ))
              ) : (
                <p className="panel-note">暂无视频数据</p>
              )}
            </div>
          </div>
        </section>

        <section id="rare" className="section section-grid">
          <div className="panel panel--span">
            <div className="panel-header">
              <div>
                <p className="section-kicker">绝版沉淀</p>
                <h3>绝版视频</h3>
              </div>
              <span className="pill pill--soft">当前累计 {metrics.rareVideos} 个</span>
            </div>
            <p className="panel-note">
              这里集中展示 `OUT_OF_PRINT` 视频清单。当前基于真实接口拉取最近 500 条绝版记录，并结合博主列表补齐名称。
            </p>
            <div className="rare-filters">
              <label className="settings-field">
                按博主筛选
                <select
                  value={rareFilters.creatorId}
                  onChange={(event) => updateRareFilters({ creatorId: event.target.value })}
                >
                  <option value="all">全部博主</option>
                  {rareCreatorOptions.map((creator) => (
                    <option key={`rare-creator-${creator.id}`} value={String(creator.id)}>
                      {creator.name}
                    </option>
                  ))}
                </select>
              </label>
              <label className="settings-field">
                关键词
                <input
                  value={rareFilters.keyword}
                  onChange={(event) => updateRareFilters({ keyword: event.target.value })}
                  placeholder="搜索标题 / BV / 博主名"
                />
              </label>
            </div>
            <PaginationControls
              page={paginatedRareVideos.page}
              pageSize={paginatedRareVideos.pageSize}
              total={paginatedRareVideos.total}
              totalPages={paginatedRareVideos.totalPages}
              onPageChange={(page) => updateLocalPager("rareVideos", { page })}
              onPageSizeChange={(pageSize) => updateLocalPager("rareVideos", { page: 1, pageSize })}
              className="pagination-controls--top"
            />
            <div className="rare-video-list" data-testid="rare-video-list">
              {paginatedRareVideos.items.length > 0 ? (
                paginatedRareVideos.items.map((video) => (
                  <div className="rare-video-row" key={`rare-${video.id}`}>
                    <div className="rare-video-row__main">
                      <div className="row-title">{video.title || video.videoId || `视频 #${video.id}`}</div>
                      <div className="row-meta">{rareVideoMeta(video)}</div>
                      <div className="row-submeta">{rareVideoTimelineText(video)}</div>
                    </div>
                    <div className="rare-video-row__side">
                      <VideoStateBadge state={video.state} />
                    </div>
                  </div>
                ))
              ) : (
                <p className="panel-note">当前没有匹配的绝版视频。</p>
              )}
            </div>
          </div>
        </section>

        <section id="storage" className="section section-grid">
          <div className="panel">
            <div className="panel-header">
              <div>
                <p className="section-kicker">存储</p>
                <h3>存储容量与清理策略</h3>
              </div>
            </div>
            <div className="storage-meter">
              <div className="storage-fill" style={{ width: storagePercent }} />
            </div>
            <div className="signal-grid">
              <SignalItem label="已用容量" value={formatBytes(state.storage.usedBytes)} />
              <SignalItem label="安全阈值" value={formatBytes(state.storage.safeBytes)} />
              <SignalItem label="总额度" value={formatBytes(state.storage.limitBytes)} />
              <SignalItem label="使用率" value={storagePercent} />
              <SignalItem label="清理压力" value={cleanupPressureBytes > 0 ? formatBytes(cleanupPressureBytes) : "阈值内"} />
              <SignalItem label="预览候选" value={`${cleanupPreview.length} 个`} />
            </div>
          </div>

          <div className="panel">
            <div className="panel-header">
              <div>
                <p className="section-kicker">清理预览</p>
                <h3>清理候选预览</h3>
              </div>
            </div>
            <p className="panel-note">
              前端当前按「非绝版 → 播放量 → 收藏量」预估清理顺序。博主粉丝量暂未从现有 API 暴露，实际删除仍以后端排序为准。
            </p>
            <div className="preview-list">
              {cleanupPreview.length > 0 ? (
                cleanupPreview.map((video, index) => (
                  <div className="preview-item" key={`cleanup-${video.id}`}>
                    <div>
                      <div className="row-title">{video.title || video.videoId || `视频 #${video.id}`}</div>
                      <div className="row-meta">{video.videoId || "-"}</div>
                      <div className="preview-reason">{cleanupPreviewText(video)}</div>
                    </div>
                    <div className="preview-side">
                      <span className="pill pill--soft">优先级 {index + 1}</span>
                      <VideoStateBadge state={video.state} />
                    </div>
                  </div>
                ))
              ) : (
                <p className="panel-note">当前没有可预览的清理候选。</p>
              )}
            </div>
            <ul className="text-list" style={{ marginTop: 12 }}>
              <li>存储目录：{state.storage.rootDir || "-"}</li>
              <li>热点目录：{state.storage.hottestBucket || "-"}</li>
              <li>文件数量：{state.storage.fileCount || 0} 个</li>
              <li>绝版累计：{metrics.rareVideos} 个</li>
              <li>清理策略：{state.storage.cleanupRule}</li>
            </ul>
          </div>
        </section>

        <section id="risk" className="section section-grid">
          <div className="panel">
            <div className="panel-header">
              <div>
                <p className="section-kicker">风控参数</p>
                <h3>风控与连接配置</h3>
              </div>
            </div>
            <div className="signal-grid">
              <SignalItem label="全局 QPS" value={state.limits.globalQps} />
              <SignalItem label="单博主 QPS" value={state.limits.perCreatorQps} />
              <SignalItem label="下载并发" value={state.limits.downloadConcurrency} />
              <SignalItem label="检查并发" value={state.limits.checkConcurrency} />
              <SignalItem label="Cookie 状态" value={cookieLabel} />
              <SignalItem label="Cookie 来源" value={cookieSourceLabel} />
              <SignalItem label="认证监控" value={state.system.authEnabled ? "已启用" : "未启用"} />
              <SignalItem label="退避剩余" value={riskBackoffLabel} />
              <SignalItem label="MySQL" value={state.system.mysqlOK ? "已连接" : "异常"} />
            </div>
          </div>

          <div className="panel">
            <div className="panel-header">
              <div>
                <p className="section-kicker">运行建议</p>
                <h3>运行建议</h3>
              </div>
            </div>
            <ul className="text-list">
              <li>{cookieAdvice(state.system.cookieConfigured, state.system.cookieStatus, state.system.cookieUname)}</li>
              <li>Cookie 来源：{cookieSourceLabel}。</li>
              <li>
                最近认证检查：{state.system.cookieLastCheckResult ? `${cookieCheckResultText(state.system.cookieLastCheckResult)} / ${formatDateTime(state.system.cookieLastCheckAt)}` : "暂无记录"}。
              </li>
              <li>
                最近配置刷新：{state.system.cookieLastReloadResult ? `${cookieReloadResultText(state.system.cookieLastReloadResult)} / ${formatDateTime(state.system.cookieLastReloadAt)}` : "暂无记录"}。
              </li>
              <li>当前风控退避：{riskBackoffLabel}。</li>
              <li>最近风控命中：{state.system.riskLastReason ? `${state.system.riskLastReason} / ${formatDateTime(state.system.riskLastHitAt)}` : "暂无记录"}。</li>
              <li>最近错误：{state.system.cookieLastError || "暂无记录"}。</li>
              <li>
                调度周期：抓取 {state.scheduler.fetchInterval}，检查 {state.scheduler.checkInterval}，清理 {state.scheduler.cleanupInterval}。
              </li>
              <li>稳定阈值：{state.scheduler.stableDays} 天。</li>
              <li>当前风险等级：{state.system.riskLevel}。</li>
              <li>存储根目录：{state.system.storageRoot || "-"}</li>
            </ul>
          </div>
        </section>

        <section id="settings" className="section section-grid">
          <div className="panel">
            <div className="panel-header">
              <div>
                <p className="section-kicker">连接设置</p>
                <h3>前端连接设置</h3>
              </div>
            </div>
            <label className="settings-field">
              API 地址
              <input
                value={state.apiBase}
                onChange={(event) =>
                  updateState((previous) => ({
                    ...previous,
                    apiBase: event.target.value
                  }))
                }
                placeholder="http://localhost:8080"
              />
            </label>
            <p className="panel-note">修改 API 地址后，点击“同步数据”即可重新拉取后端数据。</p>
          </div>

          <div className="panel">
            <div className="panel-header">
              <div>
                <p className="section-kicker">配置状态</p>
                <h3>系统配置文件</h3>
              </div>
              <span className={configRestarting || configDirty ? "pill pill--warning" : "pill pill--soft"}>
                {configRestarting ? "等待后端恢复" : configDirty ? "有未保存修改" : "已与文件同步"}
              </span>
            </div>
            <div className="config-meta">
              <span className="config-meta__label">配置路径</span>
              <code className="config-meta__value">{configPath || "-"}</code>
            </div>
            <p className="panel-note">保存前会先校验 YAML 与业务配置。若内容发生变化，后端会自动重启以加载新配置，页面会自动等待恢复并继续同步。</p>
          </div>

          <div className="panel panel--span config-editor">
            <div className="panel-header">
              <div>
                <p className="section-kicker">文件编辑</p>
                <h3>配置文件编辑</h3>
              </div>
              <div className="config-toolbar">
                <button
                  type="button"
                  className="secondary-button"
                  onClick={() => void loadSystemConfig()}
                  disabled={configLoading || configSaving}
                >
                  {configLoading ? "加载中..." : "重新加载"}
                </button>
                <button
                  type="button"
                  className="primary-button"
                  data-testid="config-save-button"
                  onClick={() => void handleSaveConfig()}
                  disabled={configLoading || configSaving}
                >
                  {configSaving ? "保存中..." : "保存配置"}
                </button>
              </div>
            </div>

            <div className="config-status">
              <span>
                {configLoading
                  ? "正在从后端读取配置文件"
                  : configRestarting
                    ? "配置已写回，正在检测后端恢复"
                    : "当前编辑内容来自后端配置文件"}
              </span>
              <span>
                {configRestarting
                  ? "重启期间接口可能短暂不可用，无需手动刷新"
                  : configDirty
                    ? "检测到未保存修改"
                    : "当前内容未修改"}
              </span>
            </div>

            <label className="settings-field">
              配置内容
              <textarea
                data-testid="config-editor"
                className="config-textarea"
                value={configText}
                onChange={(event) => setConfigText(event.target.value)}
                placeholder="server:\n  http_addr: :8080"
                spellCheck="false"
              />
            </label>

            <div className="config-inspector">
              <div className="config-card">
                <div className="panel-header">
                  <div>
                    <p className="section-kicker">变更检查</p>
                    <h3>保存前差异预览</h3>
                  </div>
                  <span className="pill pill--soft">
                    {configDiffPreview.total} 处变更
                  </span>
                </div>
                <div className="config-diff-list" data-testid="config-diff-preview">
                  {configDiffPreview.lines.length > 0 ? (
                    configDiffPreview.lines.map((line) => (
                      <div
                        key={`${line.type}-${line.lineNumber}-${line.text}`}
                        className={
                          line.type === "add" ? "config-diff-line config-diff-line--add" : "config-diff-line config-diff-line--remove"
                        }
                      >
                        <span className="config-diff-prefix">{line.type === "add" ? "+" : "-"}</span>
                        <span className="config-diff-number">L{line.lineNumber}</span>
                        <code className="config-diff-text">{line.text || "(空行)"}</code>
                      </div>
                    ))
                  ) : (
                    <p className="panel-note">当前没有未保存差异。</p>
                  )}
                  {configDiffPreview.truncated ? <p className="panel-note">仅展示前 12 行差异，完整内容请以编辑器为准。</p> : null}
                </div>
              </div>

              <div className={`config-card config-card--${configValidation.tone}`}>
                <div className="panel-header">
                  <div>
                    <p className="section-kicker">校验反馈</p>
                    <h3>校验结果详情</h3>
                  </div>
                  <span className="pill pill--soft">{configValidationText(configValidation.tone)}</span>
                </div>
                <div className="config-validation" data-testid="config-validation-detail">
                  <strong className="config-validation__title">{configValidation.title}</strong>
                  <p className="config-validation__detail">{configValidation.detail}</p>
                </div>
              </div>
            </div>
          </div>

          <div className="panel">
            <div className="panel-header">
              <div>
                <p className="section-kicker">最近事件</p>
                <h3>最近操作记录</h3>
              </div>
            </div>
            <div className="log-list">
              {state.logs.map((log) => (
                <div className="log-item" key={log.id}>
                  <span className="log-at">{log.at}</span>
                  <span>{log.message}</span>
                </div>
              ))}
            </div>
          </div>
        </section>

        <div
          className={candidateDrawerOpen ? "candidate-drawer candidate-drawer--open" : "candidate-drawer"}
          data-testid="candidate-drawer"
        >
          <button
            type="button"
            className="candidate-drawer__backdrop"
            onClick={closeCandidateDrawer}
            aria-label="关闭候选详情抽屉"
          />
          <aside className="candidate-drawer__panel">
            <div className="panel-header">
              <div>
                <p className="section-kicker">候选详情</p>
                <h3>{selectedCandidate ? selectedCandidate.name || selectedCandidate.uid : "选择候选查看来源与评分拆解"}</h3>
              </div>
              <button type="button" className="ghost-link" onClick={closeCandidateDrawer}>
                关闭
              </button>
            </div>

            {selectedCandidate ? (
              <>
                <div className="detail-grid candidate-detail-grid">
                  <DetailItem label="UID" value={selectedCandidate.uid || "-"} />
                  <DetailItem label="平台" value={selectedCandidate.platform || "bilibili"} />
                  <DetailItem label="粉丝量" value={formatFollowerCount(selectedCandidate.followerCount)} />
                  <DetailItem label="当前分数" value={`${selectedCandidate.score || 0} 分`} />
                  <DetailItem label="审核状态" value={reviewStatusText(selectedCandidate.status)} />
                  <DetailItem label="最近发现" value={formatDateTime(selectedCandidate.lastDiscoveredAt)} />
                </div>

                <div className="action-row candidate-drawer__actions">
                  {selectedCandidate.status === "reviewing" ? (
                    <>
                      <button
                        type="button"
                        className="primary-button"
                        onClick={() => void handleCandidateDecision(selectedCandidate.id, "approve")}
                        disabled={busyAction === `candidate-approve-${selectedCandidate.id}`}
                      >
                        加入追踪
                      </button>
                      <button
                        type="button"
                        className="secondary-button"
                        onClick={() => void handleCandidateDecision(selectedCandidate.id, "ignore")}
                        disabled={busyAction === `candidate-ignore-${selectedCandidate.id}`}
                      >
                        忽略
                      </button>
                      <button
                        type="button"
                        className="secondary-button"
                        onClick={() => void handleCandidateDecision(selectedCandidate.id, "block")}
                        disabled={busyAction === `candidate-block-${selectedCandidate.id}`}
                      >
                        拉黑
                      </button>
                    </>
                  ) : null}

                  {selectedCandidate.status === "ignored" ? (
                    <button
                      type="button"
                      className="secondary-button"
                      onClick={() => void handleCandidateDecision(selectedCandidate.id, "review")}
                      disabled={busyAction === `candidate-review-${selectedCandidate.id}`}
                    >
                      恢复审核
                    </button>
                  ) : null}
                </div>

                {selectedCandidate.profileUrl ? (
                  <a className="candidate-link" href={selectedCandidate.profileUrl} target="_blank" rel="noreferrer">
                    打开博主主页
                  </a>
                ) : null}

                <div className="detail-block">
                  <div className="panel-header">
                    <div>
                      <p className="section-kicker">来源</p>
                      <h3>来源列表</h3>
                    </div>
                    <span className="pill pill--soft">
                      {(candidateDrawerDetail?.sources || selectedCandidate.sources || []).length} 条
                    </span>
                  </div>
                  <div className="candidate-source-list">
                    {(candidateDrawerDetail?.sources || selectedCandidate.sources || []).length > 0 ? (
                      (candidateDrawerDetail?.sources || selectedCandidate.sources || []).map((source) => (
                        <CandidateSourceCard key={`source-${source.id}-${source.sourceValue}`} source={source} />
                      ))
                    ) : (
                      <p className="panel-note">暂无来源明细</p>
                    )}
                  </div>
                </div>

                <div className="detail-block">
                  <div className="panel-header">
                    <div>
                      <p className="section-kicker">评分</p>
                      <h3>评分拆解</h3>
                    </div>
                    <span className="pill pill--soft">
                      {(candidateDrawerDetail?.scoreDetails || []).length} 项
                    </span>
                  </div>
                  {candidateDetailLoading && !(candidateDrawerDetail?.scoreDetails || []).length ? (
                    <p className="panel-note">正在加载候选详情...</p>
                  ) : (candidateDrawerDetail?.scoreDetails || []).length > 0 ? (
                    <div className="candidate-score-list">
                      {candidateDrawerDetail.scoreDetails.map((detail) => (
                        <CandidateScoreDetailCard key={`score-${detail.id}-${detail.factorKey}`} detail={detail} />
                      ))}
                    </div>
                  ) : (
                    <p className="panel-note">暂无评分拆解</p>
                  )}
                </div>
              </>
            ) : (
              <div className="candidate-drawer__empty">
                <p>选择候选查看来源与评分拆解</p>
                <p className="panel-note">详情抽屉会展示命中来源、公开视频摘要与评分因子，便于人工决策。</p>
              </div>
            )}
          </aside>
        </div>
      </main>

      <div className={toast ? "toast toast--visible" : "toast"}>{toast}</div>
    </div>
  );
}

function MetricCard({ label, value, detail }) {
  return (
    <div className="metric-card">
      <p className="metric-label">{label}</p>
      <div className="metric-value">{value}</div>
      <p className="metric-detail">{detail}</p>
    </div>
  );
}

function SignalItem({ label, value }) {
  return (
    <div className="signal-item">
      <span className="signal-label">{label}</span>
      <strong className="signal-value">{value}</strong>
    </div>
  );
}

function DetailStat({ label, value, tone = "" }) {
  return (
    <div className={tone ? `detail-stat detail-stat--${tone}` : "detail-stat"}>
      <span className="signal-label">{label}</span>
      <strong className="signal-value">{value}</strong>
    </div>
  );
}

function DetailItem({ label, value }) {
  return (
    <div className="detail-item">
      <span className="signal-label">{label}</span>
      <strong className="signal-value">{value || "-"}</strong>
    </div>
  );
}

function PaginationControls({
  page,
  pageSize,
  total,
  totalPages,
  disabled = false,
  onPageChange,
  onPageSizeChange,
  className = ""
}) {
  const currentPage = normalizePositiveInteger(page, 1);
  const currentPageSize = normalizePositiveInteger(pageSize, PAGE_SIZE_OPTIONS[0]);
  const pageCount = Math.max(1, normalizePositiveInteger(totalPages, 1));
  const totalCount = Math.max(0, Number(total) || 0);
  const rootClassName = className ? `pagination-controls ${className}` : "pagination-controls";

  return (
    <div className={rootClassName}>
      <div className="pagination-controls__summary">
        <span>共 {totalCount} 项</span>
        <span>
          第 {currentPage} / {pageCount} 页
        </span>
      </div>
      <label className="pagination-controls__size">
        <span>每页显示</span>
        <select
          value={currentPageSize}
          onChange={(event) => onPageSizeChange?.(Number(event.target.value) || currentPageSize)}
          disabled={disabled}
        >
          {PAGE_SIZE_OPTIONS.map((option) => (
            <option key={`page-size-${option}`} value={option}>
              {option} 条
            </option>
          ))}
        </select>
      </label>
      <div className="pagination-controls__actions">
        <button
          type="button"
          className="ghost-link pagination-controls__button"
          onClick={() => onPageChange?.(currentPage - 1)}
          disabled={disabled || currentPage <= 1}
        >
          上一页
        </button>
        <button
          type="button"
          className="ghost-link pagination-controls__button"
          onClick={() => onPageChange?.(currentPage + 1)}
          disabled={disabled || currentPage >= pageCount}
        >
          下一页
        </button>
      </div>
    </div>
  );
}

function StatusBadge({ status }) {
  return <span className={`status-badge status-badge--${statusTone(status)}`}>{statusText(status)}</span>;
}

function deriveConfigDiffPreview(previousText, nextText, limit = 12) {
  const before = String(previousText || "").split("\n");
  const after = String(nextText || "").split("\n");
  const maxLength = Math.max(before.length, after.length);
  const lines = [];
  let total = 0;

  for (let index = 0; index < maxLength; index += 1) {
    const prevLine = before[index];
    const nextLine = after[index];
    if (prevLine === nextLine) {
      continue;
    }
    total += 1;
    if (prevLine !== undefined && lines.length < limit) {
      lines.push({ type: "remove", lineNumber: index + 1, text: prevLine });
    }
    if (nextLine !== undefined && lines.length < limit) {
      lines.push({ type: "add", lineNumber: index + 1, text: nextLine });
    }
  }

  return {
    total,
    lines,
    truncated: total > limit
  };
}

function configValidationText(tone) {
  switch (tone) {
    case "success":
      return "通过";
    case "warning":
      return "处理中";
    case "error":
      return "失败";
    default:
      return "待校验";
  }
}

function VideoStateBadge({ state }) {
  return <span className={`status-badge status-badge--${videoStateTone(state)}`}>{videoStateText(state)}</span>;
}

function CandidateRow({ candidate, busyAction, onView, onApprove, onIgnore, onBlock, onReview }) {
  const score = Number(candidate?.score) || 0;
  const sourceLabels = Array.isArray(candidate?.sources)
    ? candidate.sources
        .slice(0, 3)
        .map((source) => source?.sourceLabel || source?.sourceValue || "")
        .filter(Boolean)
    : [];

  return (
    <div className="candidate-row" data-testid={`candidate-row-${candidate.id}`}>
      <div className="candidate-row__main">
        <div className="candidate-row__title">
          <span>{candidate.name || candidate.uid || `候选 #${candidate.id}`}</span>
          <span className="candidate-row__uid">UID {candidate.uid || "-"}</span>
        </div>
        <div className="candidate-row__meta">
          粉丝 {formatFollowerCount(candidate.followerCount)} · 最近发现 {formatDateTime(candidate.lastDiscoveredAt)}
        </div>
        {sourceLabels.length > 0 ? (
          <div className="candidate-chip-list">
            {sourceLabels.map((label) => (
              <span className="candidate-chip" key={`${candidate.id}-${label}`}>
                {label}
              </span>
            ))}
          </div>
        ) : null}
      </div>

      <div className="candidate-row__status">
        <span className={`score-chip score-chip--${candidateScoreTone(score)}`}>{score} 分</span>
        <span className={`status-badge status-badge--${reviewStatusTone(candidate.status)}`}>{reviewStatusText(candidate.status)}</span>
      </div>

      <div className="row-actions candidate-row__actions">
        {candidate.status === "reviewing" ? (
          <>
            <button
              type="button"
              className="ghost-link"
              onClick={onApprove}
              disabled={busyAction === `candidate-approve-${candidate.id}`}
            >
              加入追踪
            </button>
            <button
              type="button"
              className="ghost-link"
              onClick={onIgnore}
              disabled={busyAction === `candidate-ignore-${candidate.id}`}
            >
              忽略
            </button>
            <button
              type="button"
              className="ghost-link"
              onClick={onBlock}
              disabled={busyAction === `candidate-block-${candidate.id}`}
            >
              拉黑
            </button>
          </>
        ) : null}

        {candidate.status === "ignored" ? (
          <>
            <button
              type="button"
              className="ghost-link"
              onClick={onReview}
              disabled={busyAction === `candidate-review-${candidate.id}`}
            >
              恢复审核
            </button>
            <button
              type="button"
              className="ghost-link"
              onClick={onBlock}
              disabled={busyAction === `candidate-block-${candidate.id}`}
            >
              拉黑
            </button>
          </>
        ) : null}

        <button type="button" className="ghost-link" onClick={onView}>
          查看详情
        </button>
      </div>
    </div>
  );
}

function CandidateSourceCard({ source }) {
  const videos = Array.isArray(source?.detail?.videos) ? source.detail.videos : [];
  return (
    <div className="candidate-source-card">
      <div className="candidate-source-card__header">
        <div>
          <div className="row-title">{source.sourceLabel || source.sourceValue || source.sourceType || "未知来源"}</div>
          <div className="row-meta">权重 {source.weight || 0} · 记录于 {formatDateTime(source.createdAt)}</div>
        </div>
        <span className="pill pill--soft">{source.sourceType || "source"}</span>
      </div>
      {videos.length > 0 ? (
        <div className="candidate-video-list">
          {videos.map((video) => (
            <div className="candidate-video-row" key={`${source.id}-${video.videoId || video.title}`}>
              <div>
                <div className="row-title">{video.title || video.videoId || "未命名视频"}</div>
                <div className="row-meta">
                  {video.videoId || "-"} · 发布于 {formatDateTime(video.publishTime)}
                </div>
              </div>
              <div className="candidate-video-side">
                <span>播放 {formatCount(video.viewCount)}</span>
                <span>收藏 {formatCount(video.favoriteCount)}</span>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <p className="panel-note">该来源暂无公开视频摘要。</p>
      )}
    </div>
  );
}

function CandidateScoreDetailCard({ detail }) {
  const detailEntries = detail?.detail && typeof detail.detail === "object" ? Object.entries(detail.detail) : [];
  return (
    <div className="candidate-score-card">
      <div className="candidate-score-card__header">
        <div>
          <div className="row-title">{detail.factorLabel || detail.factorKey || "未命名因子"}</div>
          <div className="row-meta">{detail.factorKey || "-"}</div>
        </div>
        <span className={`score-chip score-chip--${detail.scoreDelta >= 80 ? "high" : detail.scoreDelta > 0 ? "medium" : "low"}`}>
          {detail.scoreDelta > 0 ? `+${detail.scoreDelta}` : detail.scoreDelta}
        </span>
      </div>
      {detailEntries.length > 0 ? (
        <div className="payload-list candidate-score-card__details">
          {detailEntries.map(([key, value]) => (
            <div className="payload-row" key={`${detail.id}-${key}`}>
              <span className="payload-key">{key}</span>
              <span className="payload-value">{formatPayloadValue(value)}</span>
            </div>
          ))}
        </div>
      ) : (
        <p className="panel-note">暂无评分附加信息。</p>
      )}
    </div>
  );
}

function statusText(status) {
  switch (status) {
    case "queued":
      return "待执行";
    case "running":
      return "执行中";
    case "success":
      return "已完成";
    case "failed":
      return "失败";
    default:
      return status || "未知";
  }
}

function statusTone(status) {
  switch (status) {
    case "queued":
      return "queued";
    case "running":
      return "running";
    case "success":
      return "success";
    case "failed":
      return "failed";
    default:
      return "queued";
  }
}

function reviewStatusText(status) {
  switch (status) {
    case "reviewing":
      return "审核中";
    case "ignored":
      return "已忽略";
    case "approved":
      return "已批准";
    case "blocked":
      return "已拉黑";
    default:
      return status || "未知";
  }
}

function reviewStatusTone(status) {
  switch (status) {
    case "reviewing":
      return "running";
    case "ignored":
      return "queued";
    case "approved":
      return "success";
    case "blocked":
      return "failed";
    default:
      return "queued";
  }
}

function candidateScoreTone(score) {
  if (score >= 80) {
    return "high";
  }
  if (score >= 60) {
    return "medium";
  }
  return "low";
}

function videoStateText(state) {
  switch (state) {
    case "OUT_OF_PRINT":
      return "绝版";
    case "STABLE":
      return "稳定";
    case "DOWNLOADING":
      return "下载中";
    case "DOWNLOADED":
      return "已下载";
    case "ONLINE":
      return "在线";
    default:
      return state || "未知";
  }
}

function videoStateTone(state) {
  switch (state) {
    case "OUT_OF_PRINT":
      return "failed";
    case "STABLE":
    case "DOWNLOADED":
      return "success";
    case "DOWNLOADING":
      return "running";
    default:
      return "queued";
  }
}

function healthText(health) {
  switch (health) {
    case "online":
      return "连接正常";
    case "degraded":
      return "存在异常";
    default:
      return "状态未知";
  }
}

function connectionText(status) {
  switch (status) {
    case "connecting":
      return "连接中";
    case "live":
      return "实时同步中";
    case "reconnecting":
      return "重连中";
    case "offline":
      return "连接中断";
    default:
      return "状态未知";
  }
}

function connectionStatusClassName(status) {
  switch (status) {
    case "live":
      return "live-connection--live";
    case "reconnecting":
      return "live-connection--reconnecting";
    case "offline":
      return "live-connection--offline";
    default:
      return "live-connection--connecting";
  }
}

function cookieText(status, configured) {
  if (!configured) {
    return "未配置";
  }
  switch (status) {
    case "valid":
      return "有效";
    case "invalid":
      return "失效";
    case "error":
      return "检查失败";
    default:
      return "待确认";
  }
}

function cookieAdvice(configured, status, uname) {
  if (!configured) {
    return "当前未配置 Cookie / SESSDATA，建议先配置再进行高频真实抓取。";
  }
  if (status === "valid") {
    return uname ? `当前 Cookie 已生效，登录用户为 ${uname}。` : "当前 Cookie 已生效，可用于真实抓取。";
  }
  if (status === "invalid") {
    return "当前 Cookie 已失效，建议尽快更换，避免接口返回 403/412。";
  }
  return "Cookie 状态暂不可确认，建议先在后端检查认证连通性。";
}

function cookieSourceText(source) {
  switch (source) {
    case "config":
      return "配置文件";
    case "cookie_file":
      return "Cookie 文件";
    case "sessdata_file":
      return "SESSDATA 文件";
    default:
      return "未知";
  }
}

function cookieCheckResultText(result) {
  switch (result) {
    case "valid":
      return "有效";
    case "invalid":
      return "失效";
    case "error":
      return "检查失败";
    default:
      return result || "未知";
  }
}

function cookieReloadResultText(result) {
  switch (result) {
    case "success":
      return "刷新成功";
    case "no_change":
      return "无变化";
    case "error":
      return "刷新失败";
    default:
      return result || "未知";
  }
}

function riskBackoffText(active, seconds) {
  if (!active) {
    return "未触发";
  }
  const remain = Number(seconds) || 0;
  return remain > 0 ? `${remain} 秒` : "退避中";
}

function jobText(type) {
  switch (type) {
    case "fetch":
      return "拉取最新视频";
    case "discover":
      return "发现候选博主";
    case "check":
      return "检查视频状态";
    case "cleanup":
      return "清理存储";
    case "download":
      return "下载视频";
    default:
      return type || "未知任务";
  }
}

function jobMeta(job) {
  if (job.createdAt) {
    return `创建于 ${formatDateTime(job.createdAt)}`;
  }
  if (job.errorMsg) {
    return `错误: ${job.errorMsg}`;
  }
  return `来源: ${job.origin || "manual"}`;
}

function videoMeta(video) {
  const views = formatCount(video.viewCount);
  const favorites = formatCount(video.favoriteCount);
  const publish = formatDateTime(video.publishTime);
  return `发布时间 ${publish} · 播放 ${views} · 收藏 ${favorites}`;
}

function rareVideoMeta(video) {
  return `${video.videoId || "-"} · ${video.creatorName || `博主 #${video.creatorId || "-"}`}`;
}

function rareVideoTimelineText(video) {
  return `绝版于 ${formatDateTime(video.outOfPrintAt)} · 最近检查 ${formatDateTime(video.lastCheckAt)}`;
}

function cleanupPreviewText(video) {
  return `${(video.reasons || []).join(" · ")}${video.reasons?.length ? " · " : ""}状态 ${videoStateText(video.state)}`;
}

function payloadRows(payload) {
  if (!payload || typeof payload !== "object") {
    return [];
  }
  return Object.entries(payload)
    .filter(([, value]) => value !== undefined && value !== null && value !== "")
    .map(([key, value]) => ({
      key,
      value: formatPayloadValue(value)
    }));
}

function formatPayloadValue(value) {
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  try {
    return JSON.stringify(value);
  } catch (_error) {
    return String(value);
  }
}

function truncateText(value, limit) {
  const text = String(value || "");
  if (text.length <= limit) {
    return text;
  }
  return `${text.slice(0, Math.max(0, limit - 1))}…`;
}

function formatDateTime(value) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString("zh-CN", { hour12: false });
}

function formatBytes(value) {
  const bytes = Number(value) || 0;
  if (bytes <= 0) {
    return "0 B";
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  const exponent = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const amount = bytes / 1024 ** exponent;
  const digits = amount >= 100 || exponent === 0 ? 0 : amount >= 10 ? 1 : 2;
  return `${amount.toFixed(digits)} ${units[exponent]}`;
}

function formatCount(value) {
  const count = Number(value) || 0;
  if (count >= 100000000) {
    return `${(count / 100000000).toFixed(1)} 亿`;
  }
  if (count >= 10000) {
    return `${(count / 10000).toFixed(1)} 万`;
  }
  return String(count);
}

function formatFollowerCount(value) {
  const count = Number(value) || 0;
  if (count <= 0) {
    return "待补全";
  }
  return formatCount(count);
}

function normalizePositiveInteger(value, fallback = 1) {
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

function nowLabel() {
  return new Date().toLocaleString("zh-CN", { hour12: false });
}

export default App;
