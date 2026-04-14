import { useEffect, useMemo, useState } from "react";
import {
  createCreator,
  deleteCreator,
  enqueueJob,
  formatRequestError,
  getSystemConfig,
  loadDashboardSnapshot,
  patchCreator,
  updateSystemConfig
} from "./lib/api";
import {
  applyRemoteSnapshot,
  deriveCleanupPreview,
  deriveMetrics,
  deriveTaskDiagnostics,
  loadState,
  makeLog,
  saveState
} from "./lib/state";

const sections = [
  { id: "overview", label: "总览" },
  { id: "creators", label: "博主" },
  { id: "tasks", label: "任务" },
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

function App() {
  const [state, setState] = useState(() => loadState());
  const [activeSection, setActiveSection] = useState("overview");
  const [selectedJobId, setSelectedJobId] = useState(null);
  const [toast, setToast] = useState("");
  const [busyAction, setBusyAction] = useState("");
  const [form, setForm] = useState({
    uid: "",
    name: "",
    platform: "bilibili",
    status: "active"
  });
  const [configPath, setConfigPath] = useState("");
  const [configText, setConfigText] = useState("");
  const [savedConfigText, setSavedConfigText] = useState("");
  const [configLoading, setConfigLoading] = useState(false);
  const [configSaving, setConfigSaving] = useState(false);
  const [configValidation, setConfigValidation] = useState(() => ({
    tone: "idle",
    title: "尚未执行保存校验",
    detail: "保存配置时会展示 YAML 与业务配置校验结果。"
  }));
  const metrics = useMemo(() => deriveMetrics(state), [state]);
  const taskDiagnostics = useMemo(() => deriveTaskDiagnostics(state), [state]);
  const cleanupPreview = useMemo(() => deriveCleanupPreview(state, 5), [state]);
  const configDiffPreview = useMemo(
    () => deriveConfigDiffPreview(savedConfigText, configText),
    [savedConfigText, configText]
  );
  const selectedJob = useMemo(() => {
    const jobs = Array.isArray(state.jobs) ? state.jobs : [];
    return jobs.find((job) => job.id === selectedJobId) || jobs[0] || null;
  }, [selectedJobId, state.jobs]);

  useEffect(() => {
    saveState(state);
  }, [state]);

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
    void syncDashboardFromAPI({ silent: true });
    void loadSystemConfig({ silent: true });
  }, []);

  useEffect(() => {
    setSelectedJobId((current) => {
      const jobs = Array.isArray(state.jobs) ? state.jobs : [];
      if (jobs.length === 0) {
        return null;
      }
      if (jobs.some((job) => job.id === current)) {
        return current;
      }
      return jobs[0].id;
    });
  }, [state.jobs]);

  function updateState(updater) {
    setState((previous) => (typeof updater === "function" ? updater(previous) : updater));
  }

  function pushLog(message) {
    updateState((previous) => ({
      ...previous,
      logs: [makeLog(message), ...(previous.logs || [])].slice(0, 18)
    }));
  }

  function showToast(message) {
    setToast(message);
    window.clearTimeout(showToast.timer);
    showToast.timer = window.setTimeout(() => setToast(""), 2200);
  }

  async function syncDashboardFromAPI({ silent = false } = {}) {
    setBusyAction("sync");
    try {
      const snapshot = await loadDashboardSnapshot(state.apiBase);
      const syncAt = nowLabel();
      updateState((previous) => ({
        ...applyRemoteSnapshot(previous, snapshot, syncAt),
        logs: [
          makeLog(`已同步真实数据: ${snapshot.creators.length} 个博主 / ${snapshot.jobs.length} 个任务 / ${snapshot.videos.length} 个视频`),
          ...(previous.logs || [])
        ].slice(0, 18)
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
        logs: [makeLog(`同步失败: ${message}`), ...(previous.logs || [])].slice(0, 18)
      }));
      showToast(message);
    } finally {
      setBusyAction("");
    }
  }

  async function refreshAfterMutation(successMessage) {
    const snapshot = await loadDashboardSnapshot(state.apiBase);
    const syncAt = nowLabel();
    updateState((previous) => ({
      ...applyRemoteSnapshot(previous, snapshot, syncAt),
      logs: [makeLog(successMessage), ...(previous.logs || [])].slice(0, 18)
    }));
  }

  async function loadSystemConfig({ silent = false } = {}) {
    setConfigLoading(true);
    try {
      const payload = await getSystemConfig(state.apiBase);
      const nextContent = String(payload?.content || "");
      setConfigPath(String(payload?.path || ""));
      setConfigText(nextContent);
      setSavedConfigText(nextContent);
      setConfigValidation({
        tone: "success",
        title: "配置已加载",
        detail: "当前编辑器内容已经和后端配置文件同步。修改后保存时会再次执行校验。"
      });
      if (!silent) {
        showToast("配置已加载");
      }
    } catch (error) {
      const message = formatRequestError(error);
      setConfigValidation({
        tone: "error",
        title: "配置加载失败",
        detail: message
      });
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
      setConfigValidation(
        result?.changed
          ? {
              tone: "success",
              title: "配置校验通过并已保存",
              detail: "配置文件已写回磁盘，后端正在重启并重新加载最新配置。"
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

  const storagePercent = `${metrics.storagePercent}%`;
  const healthLabel = healthText(state.system.health);
  const cookieLabel = cookieText(state.system.cookieStatus, state.system.cookieConfigured);
  const cookieSourceLabel = cookieSourceText(state.system.cookieSource);
  const riskBackoffLabel = riskBackoffText(state.system.riskActive, state.system.riskBackoffSeconds);
  const cleanupPressureBytes = Math.max(0, Number(state.storage.usedBytes || 0) - Number(state.storage.safeBytes || 0));
  const configDirty = configText !== savedConfigText;

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
          </div>
          <div className="command-actions">
            <button
              type="button"
              className="secondary-button"
              data-testid="sync-button"
              onClick={() => void syncDashboardFromAPI()}
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
            <div className="table-header">
              <span>UID</span>
              <span>名称</span>
              <span>平台</span>
              <span>状态</span>
              <span>操作</span>
            </div>
            <div className="table-body" data-testid="creator-list">
              {state.creators.map((creator) => (
                <div className="table-row" key={creator.id}>
                  <span>{creator.uid || "-"}</span>
                  <span>{creator.name || "-"}</span>
                  <span>{creator.platform}</span>
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
            <div className="stack-list" style={{ marginTop: 16 }} data-testid="job-list">
              {state.jobs.length > 0 ? (
                state.jobs.slice(0, 6).map((job) => (
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
            </div>
            <div className="stack-list">
              {state.videos.length > 0 ? (
                state.videos.slice(0, 6).map((video) => (
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
              <span className={configDirty ? "pill pill--warning" : "pill pill--soft"}>
                {configDirty ? "有未保存修改" : "已与文件同步"}
              </span>
            </div>
            <div className="config-meta">
              <span className="config-meta__label">配置路径</span>
              <code className="config-meta__value">{configPath || "-"}</code>
            </div>
            <p className="panel-note">保存前会先校验 YAML 与业务配置；保存成功后，后端容器会自动重启以加载新配置。</p>
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
              <span>{configLoading ? "正在从后端读取配置文件" : "当前编辑内容来自后端配置文件"}</span>
              <span>{configDirty ? "检测到未保存修改" : "当前内容未修改"}</span>
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
    case "error":
      return "失败";
    default:
      return "待校验";
  }
}

function VideoStateBadge({ state }) {
  return <span className={`status-badge status-badge--${videoStateTone(state)}`}>{videoStateText(state)}</span>;
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

function nowLabel() {
  return new Date().toLocaleString("zh-CN", { hour12: false });
}

export default App;
