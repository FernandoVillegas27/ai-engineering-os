(() => {
  "use strict";

  const state = {
    runs: [],
    observability: null,
    runId: "",
    runState: null,
    events: [],
    messages: [],
    artifacts: [],
    promotion: null,
    controls: null,
    refreshTimer: null,
    eventSource: null,
    streamRunId: "",
    generation: 0,
  };

  const $ = (id) => document.getElementById(id);
  const els = {
    runList: $("run-list"), runCount: $("run-count"), systemSummary: $("system-summary"), refreshRuns: $("refresh-runs"),
    connectionDot: $("connection-dot"), connectionLabel: $("connection-label"),
    empty: $("empty-state"), dashboard: $("dashboard"), title: $("run-title"),
    status: $("run-status"), copy: $("copy-run-id"),
    progressValue: $("progress-value"), progressBar: $("progress-bar"),
    agentCount: $("agent-count"), agentSubtitle: $("agent-subtitle"),
    coordinationValue: $("coordination-value"), coordinationSubtitle: $("coordination-subtitle"),
    qualityValue: $("quality-value"), qualitySubtitle: $("quality-subtitle"),
    taskCount: $("task-count"), taskList: $("task-list"),
    activeAgentCount: $("active-agent-count"), agentList: $("agent-list"),
    eventCount: $("event-count"), eventList: $("event-list"),
    messageCount: $("message-count"), messageList: $("message-list"),
    artifactCount: $("artifact-count"), artifactList: $("artifact-list"),
    promotion: $("promotion-view"), controls: $("controls-view"), toast: $("toast"),
  };

  function escapeHTML(value) {
    return String(value ?? "").replace(/[&<>"']/g, (ch) => ({
      "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
    })[ch]);
  }

  function shortId(value, max = 28) {
    value = String(value || "");
    if (value.length <= max) return value;
    return value.slice(0, Math.max(8, max - 9)) + "…" + value.slice(-8);
  }

  function statusClass(value) {
    const v = String(value || "").toLowerCase();
    if (["completed", "passed", "approved", "ready"].includes(v)) return "success";
    if (["running", "active", "assigned"].includes(v)) return "running";
    if (["failed", "cancelled", "rejected", "blocked"].includes(v)) return "danger";
    if (["paused", "waiting", "partial", "requested"].includes(v)) return "warning";
    return "neutral";
  }

  function setConnection(ok, text) {
    els.connectionDot.className = "status-dot " + (ok ? "live" : "error");
    els.connectionLabel.textContent = text;
  }

  async function api(path, optional = false) {
    const response = await fetch(path, {headers: {"Accept": "application/json"}, cache: "no-store"});
    if (optional && response.status === 404) return null;
    if (!response.ok) {
      let detail = response.statusText;
      try {
        const body = await response.json();
        detail = body.error || detail;
      } catch (_) {}
      throw new Error(detail || ("HTTP " + response.status));
    }
    return response.json();
  }

  function toast(message) {
    els.toast.textContent = message;
    els.toast.classList.add("show");
    window.setTimeout(() => els.toast.classList.remove("show"), 1800);
  }

  async function loadRuns(preserveSelection = true) {
    try {
      const [runs, observability] = await Promise.all([
        api("/runs"),
        api("/observability").catch(() => null),
      ]);
      state.runs = Array.isArray(runs) ? runs : [];
      state.observability = observability;
      setConnection(true, "Control Plane online");
      renderRuns();
      renderSystemSummary();
      if (!preserveSelection || !state.runId) {
        const query = new URLSearchParams(location.search).get("run");
        const preferred = query && state.runs.some((r) => r.run_id === query) ? query : state.runs[0]?.run_id;
        if (preferred) await selectRun(preferred);
      } else if (!state.runs.some((r) => r.run_id === state.runId)) {
        resetSelection();
      }
    } catch (error) {
      setConnection(false, "Backend unavailable");
      els.runCount.textContent = error.message;
    }
  }

  function renderSystemSummary() {
    const value = state.observability;
    if (!value) {
      els.systemSummary.textContent = "Local metrics unavailable";
      return;
    }
    const events = Number(value.event_count || 0);
    const tasks = Number(value.task_count || 0);
    const approvals = Number(value.pending_approvals || 0);
    els.systemSummary.textContent = events + " events · " + tasks + " tasks" +
      (approvals ? " · " + approvals + " approval(s)" : "");
  }

  function renderRuns() {
    els.runCount.textContent = state.runs.length + (state.runs.length === 1 ? " run" : " runs");
    if (!state.runs.length) {
      els.runList.innerHTML = '<div class="empty-list">No durable runs yet.</div>';
      return;
    }
    els.runList.innerHTML = state.runs.map((run) => {
      const p = run.progress || {};
      const done = Number(p.completed || 0);
      const total = Number(p.total || 0);
      return '<button class="run-item ' + (run.run_id === state.runId ? "active" : "") +
        '" data-run="' + escapeHTML(run.run_id) + '" type="button">' +
        '<div class="run-item-row"><span class="run-item-id" title="' + escapeHTML(run.run_id) + '">' +
        escapeHTML(shortId(run.run_id, 26)) + '</span><span class="mini-status ' +
        statusClass(run.status) + '">' + escapeHTML(run.status || "unknown") + '</span></div>' +
        '<div class="run-item-meta"><span>' + done + '/' + total + ' tasks</span><span>#' +
        escapeHTML(run.last_sequence || 0) + '</span></div></button>';
    }).join("");
    els.runList.querySelectorAll("[data-run]").forEach((button) => {
      button.addEventListener("click", () => selectRun(button.dataset.run));
    });
  }

  async function selectRun(runId) {
    if (!runId) return;
    state.runId = runId;
    state.generation++;
    const generation = state.generation;
    renderRuns();
    els.empty.classList.add("hidden");
    els.dashboard.classList.remove("hidden");
    els.title.textContent = runId;
    els.copy.disabled = false;
    history.replaceState(null, "", "/ui/?run=" + encodeURIComponent(runId));
    await refreshSelected(generation);
    connectStream(runId);
    scheduleRefresh();
  }

  function resetSelection() {
    state.runId = "";
    state.runState = null;
    state.generation++;
    closeStream();
    els.empty.classList.remove("hidden");
    els.dashboard.classList.add("hidden");
    els.title.textContent = "Choose a run";
    els.status.textContent = "No run";
    els.status.className = "status-pill neutral";
    els.copy.disabled = true;
    history.replaceState(null, "", "/ui/");
  }

  async function refreshSelected(generation = state.generation) {
    const runId = state.runId;
    if (!runId) return;
    const encoded = encodeURIComponent(runId);
    try {
      const [runState, events, messages, artifacts, promotion, controls] = await Promise.all([
        api("/runs/" + encoded + "/state"),
        api("/runs/" + encoded + "/events"),
        api("/runs/" + encoded + "/messages").catch(() => []),
        api("/runs/" + encoded + "/artifacts").catch(() => []),
        api("/runs/" + encoded + "/promotion", true).catch(() => null),
        api("/runs/" + encoded + "/controls").catch(() => null),
      ]);
      if (generation !== state.generation || runId !== state.runId) return;
      state.runState = runState;
      state.events = Array.isArray(events) ? events : [];
      state.messages = Array.isArray(messages) ? messages : [];
      state.artifacts = Array.isArray(artifacts) ? artifacts : [];
      state.promotion = promotion;
      state.controls = controls;
      renderSelected();
      setConnection(true, "Live · last event #" + (runState.last_sequence || 0));
    } catch (error) {
      if (generation !== state.generation) return;
      setConnection(false, error.message);
      toast("Refresh failed: " + error.message);
    }
  }

  function closeStream() {
    if (state.eventSource) {
      state.eventSource.close();
      state.eventSource = null;
    }
    state.streamRunId = "";
  }

  function connectStream(runId) {
    closeStream();
    if (!runId || typeof EventSource === "undefined") return;
    const last = Number(state.runState?.last_sequence || 0);
    const source = new EventSource(
      "/runs/" + encodeURIComponent(runId) + "/stream?after=" + encodeURIComponent(last)
    );
    state.eventSource = source;
    state.streamRunId = runId;

    source.onopen = () => {
      if (state.streamRunId === runId) setConnection(true, "Live stream connected");
    };
    source.onmessage = () => {
      if (state.runId === runId) refreshSelected();
    };
    source.onerror = () => {
      if (state.streamRunId !== runId) return;
      setConnection(false, "Live stream reconnecting · polling active");
    };
    [
      "run.started", "run.completed", "run.failed", "run.cancelled", "run.paused", "run.resumed",
      "task.queued", "task.assigned", "task.started", "task.completed", "task.failed", "task.blocked",
      "message.sent", "artifact.created", "quality.updated", "review.updated", "promotion.updated"
    ].forEach((eventName) => {
      source.addEventListener(eventName, () => {
        if (state.runId === runId) refreshSelected();
      });
    });
  }

  function scheduleRefresh() {
    if (state.refreshTimer) clearInterval(state.refreshTimer);
    state.refreshTimer = setInterval(() => {
      if (state.runId && document.visibilityState === "visible") refreshSelected();
      loadRuns(true);
    }, 5000);
  }

  function renderSelected() {
    const s = state.runState || {};
    const progress = s.progress || {};
    const tasks = Object.values(s.tasks || {}).sort((a,b) => String(a.id).localeCompare(String(b.id)));
    const agents = Object.values(s.agents || {}).sort((a,b) => String(a.id).localeCompare(String(b.id)));
    const completed = Number(progress.completed || 0);
    const total = Number(progress.total || tasks.length || 0);
    const pct = total ? Math.min(100, Math.round((completed / total) * 100)) : 0;
    const activeAgents = agents.filter((a) => ["active","running"].includes(String(a.status || "").toLowerCase()));

    els.status.textContent = s.status || "unknown";
    els.status.className = "status-pill " + statusClass(s.status);
    els.progressValue.textContent = completed + " / " + total;
    els.progressBar.style.width = pct + "%";
    els.agentCount.textContent = agents.length;
    els.agentSubtitle.textContent = activeAgents.length ? activeAgents.length + " active" : "No active agents";
    els.coordinationValue.textContent = Number(s.message_count || 0) + Number(s.artifact_count || 0);
    els.coordinationSubtitle.textContent = (s.message_count || 0) + " messages · " + (s.artifact_count || 0) + " artifacts";

    const quality = s.quality || {};
    els.qualityValue.textContent = (quality.verification || "—") + " / " + (quality.review || "—");
    els.qualitySubtitle.textContent = (quality.baseline_comparisons || 0) + " baseline · " + (quality.corrections || 0) + " corrections";

    els.taskCount.textContent = tasks.length;
    els.activeAgentCount.textContent = activeAgents.length + " active";
    els.eventCount.textContent = state.events.length;
    els.messageCount.textContent = state.messages.length;
    els.artifactCount.textContent = state.artifacts.length;

    renderTasks(tasks);
    renderAgents(agents);
    renderEvents();
    renderMessages();
    renderArtifacts();
    renderPromotion();
    renderControls();
  }

  function renderTasks(tasks) {
    if (!tasks.length) {
      els.taskList.innerHTML = '<div class="empty-list">No tasks projected.</div>';
      return;
    }
    els.taskList.innerHTML = tasks.map((task) => {
      const deps = Array.isArray(task.depends_on) && task.depends_on.length ? " · depends " + task.depends_on.join(", ") : "";
      const scopes = Array.isArray(task.scopes) && task.scopes.length ? " · " + task.scopes.length + " scope(s)" : "";
      const toolOps = formatToolOperations(task.tool_operations);
      return '<div class="task-row"><div><div class="task-title">' + escapeHTML(task.id || "task") +
        '</div><div class="task-meta">' + escapeHTML(task.agent_id || "unassigned") + escapeHTML(deps + scopes) +
        '</div>' + (toolOps ? '<div class="task-meta">tools · ' + escapeHTML(toolOps) + '</div>' : '') +
        '</div><div class="task-status ' + statusClass(task.status) + '">' + escapeHTML(task.status || "queued") + '</div></div>';
    }).join("");
  }

  function formatToolOperations(value) {
    if (!value || typeof value !== "object") return "";
    return Object.keys(value).sort().map((tool) => {
      const operations = Array.isArray(value[tool]) ? [...new Set(value[tool].map(String))].sort() : [];
      return operations.length ? tool + "[" + operations.join(",") + "]" : "";
    }).filter(Boolean).join("; ");
  }

  function initials(value) {
    return String(value || "AI").split(/[-_ ]+/).filter(Boolean).slice(0,2).map((v) => v[0]).join("").toUpperCase();
  }

  function renderAgents(agents) {
    if (!agents.length) {
      els.agentList.innerHTML = '<div class="empty-list">No agents projected.</div>';
      return;
    }
    els.agentList.innerHTML = agents.map((agent) => {
      const active = Array.isArray(agent.active_tasks) ? agent.active_tasks : [];
      return '<div class="agent-row"><div class="agent-avatar">' + escapeHTML(initials(agent.id)) +
        '</div><div class="agent-info"><div class="agent-id">' + escapeHTML(agent.id || "agent") +
        '</div><div class="agent-meta">' + escapeHTML(active.length ? active.join(", ") : (agent.task_count || 0) + " task(s)") +
        '</div></div><span class="agent-state ' + escapeHTML(agent.status || "") + '"></span></div>';
    }).join("");
  }

  function renderEvents() {
    const events = [...state.events].reverse();
    if (!events.length) {
      els.eventList.innerHTML = '<div class="empty-list">No events.</div>';
      return;
    }
    els.eventList.innerHTML = events.map((event) => {
      const meta = [];
      if (event.agent_id) meta.push("agent=" + event.agent_id);
      if (event.task_id) meta.push("task=" + event.task_id);
      if (event.created_at) meta.push(formatTime(event.created_at));
      return '<div class="timeline-item"><div class="timeline-seq">#' + escapeHTML(event.sequence || 0) +
        '</div><div class="timeline-line"><span class="timeline-dot"></span></div><div><div class="timeline-type">' +
        escapeHTML(event.type || "event") + '</div><div class="timeline-meta">' + meta.map(escapeHTML).join("<span>·</span>") +
        '</div>' + (event.summary ? '<div class="timeline-summary">' + escapeHTML(event.summary) + '</div>' : "") + '</div></div>';
    }).join("");
  }

  function renderMessages() {
    if (!state.messages.length) {
      els.messageList.innerHTML = '<div class="empty-list">No persistent messages for this run.</div>';
      return;
    }
    els.messageList.innerHTML = [...state.messages].reverse().map((msg) => {
      const to = msg.to ? (msg.to.kind + ":" + msg.to.id) : "—";
      return '<article class="detail-card"><div class="detail-card-head"><div class="detail-card-title">' +
        escapeHTML(msg.kind || "message") + '</div><span class="count-badge">' + escapeHTML(formatTime(msg.created_at)) +
        '</span></div><div class="detail-card-meta">' + escapeHTML((msg.from_agent || "unknown") + " → " + to) +
        (msg.task_id ? " · task=" + escapeHTML(msg.task_id) : "") + '</div><div class="detail-card-body">' +
        escapeHTML(payloadPreview(msg.payload)) + '</div></article>';
    }).join("");
  }

  function renderArtifacts() {
    if (!state.artifacts.length) {
      els.artifactList.innerHTML = '<div class="empty-list">No immutable artifacts for this run.</div>';
      return;
    }
    els.artifactList.innerHTML = [...state.artifacts].reverse().map((artifact) => {
      return '<article class="detail-card"><div class="detail-card-head"><div class="detail-card-title">' +
        escapeHTML(artifact.type || "artifact") + '</div><span class="count-badge">' +
        escapeHTML(shortId(artifact.id, 20)) + '</span></div><div class="detail-card-meta">' +
        escapeHTML((artifact.producer_agent || "unknown") + (artifact.task_id ? " · task=" + artifact.task_id : "")) +
        '</div><div class="detail-card-body">' + escapeHTML(artifact.summary || payloadPreview(artifact.payload)) +
        '</div></article>';
    }).join("");
  }

  function renderPromotion() {
    const p = state.promotion;
    if (!p) {
      els.promotion.innerHTML = '<div class="notice">No promotion candidate has been persisted for this run.</div>';
      return;
    }
    const cls = p.ready && p.quality_passed ? "success" : "warning";
    els.promotion.innerHTML =
      '<div class="notice ' + cls + '">' + (p.ready ? "Promotion candidate is currently valid." : "Promotion candidate is not currently ready.") +
      ' This surface is read-only; promotion still requires the explicit CLI approval boundary.</div>' +
      '<div class="kv-grid" style="margin-top:10px">' +
      kv("Quality passed", p.quality_passed ? "Yes" : "No") +
      kv("Current readiness", p.ready ? "Ready" : "Not ready") +
      kv("Declared changes", p.change_count ?? 0) +
      kv("Created", formatTime(p.created_at)) + '</div>';
  }

  function renderControls() {
    const c = state.controls;
    if (!c) {
      els.controls.innerHTML = '<div class="notice">Run-control capability derivation is unavailable.</div>';
      return;
    }
    const flags = [
      ["Consistent", c.consistent],
      ["Can pause", c.can_pause],
      ["Can resume", c.can_resume],
      ["Can cancel", c.can_cancel],
      ["Can retry", c.can_retry],
      ["Can recover", c.can_recover],
    ];
    els.controls.innerHTML =
      '<div class="notice">These are deterministic capabilities only. This web client does not issue mutation/control commands.</div>' +
      '<div class="kv-grid" style="margin-top:10px">' +
      flags.map(([label,value]) => kv(label, value ? "Yes" : "No")).join("") +
      kv("Pause boundary", c.pause_boundary || "—") +
      kv("Recovery", c.recovery_reason || c.recovery_status || "—") +
      '</div>';
  }

  function kv(label, value) {
    return '<div class="kv"><div class="kv-label">' + escapeHTML(label) + '</div><div class="kv-value">' + escapeHTML(value) + '</div></div>';
  }

  function payloadPreview(value) {
    if (value == null) return "—";
    if (typeof value === "string") return value;
    try {
      const text = JSON.stringify(value, null, 2);
      return text.length > 4000 ? text.slice(0, 3997) + "..." : text;
    } catch (_) {
      return String(value);
    }
  }

  function formatTime(value) {
    if (!value) return "—";
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return String(value);
    return date.toLocaleString([], {month:"short", day:"2-digit", hour:"2-digit", minute:"2-digit", second:"2-digit"});
  }

  document.querySelectorAll(".tab").forEach((button) => {
    button.addEventListener("click", () => {
      document.querySelectorAll(".tab").forEach((tab) => tab.classList.toggle("active", tab === button));
      document.querySelectorAll(".tab-pane").forEach((pane) => pane.classList.remove("active"));
      const pane = $("tab-" + button.dataset.tab);
      if (pane) pane.classList.add("active");
    });
  });

  els.refreshRuns.addEventListener("click", () => loadRuns(true));
  els.copy.addEventListener("click", async () => {
    if (!state.runId) return;
    try {
      await navigator.clipboard.writeText(state.runId);
      toast("Run ID copied");
    } catch (_) {
      toast("Clipboard unavailable");
    }
  });
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") {
      loadRuns(true);
      refreshSelected();
      if (state.runId && state.streamRunId !== state.runId) connectStream(state.runId);
    }
  });

  loadRuns(false);
  scheduleRefresh();
})();
