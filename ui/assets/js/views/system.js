import { api } from "../api.js";
import { bytes, cls, debounce, duration, esc, level, modal, num, pct, setBusy, timeOnly, toast, typedConfirm } from "../dom.js";
import { errorText, t } from "../i18n.js";
import { icon } from "../icons.js";

export function renderSystem(root, ctx) {
  const admin = ctx.user?.role === "admin";
  const tabs = [
    ["overview", t("system.overview"), "system"],
    ["processes", t("system.processes"), "list"],
    ["services", t("system.services"), "server"],
    ["logs", t("system.logs"), "file"],
    ["power", t("system.power"), "power"],
  ].filter(([id]) => admin || !["services", "logs", "power"].includes(id));
  let active = "overview";
  let alive = true;
  let snap = null;
  let procs = [];
  let services = [];
  let timer = null;
  let logsUnit = "mazimad";
  let logsText = "";
  let logsLoading = false;

  root.innerHTML = `<div class="view-stack">
    <div class="section-head"><div><h2>${esc(t("system.title"))}</h2><div class="type-note">${esc(t("system.subtitle"))}</div></div><button class="btn ghost" data-refresh>${icon("refresh")}${esc(t("common.refresh"))}</button></div>
    <div class="win-tabs" role="tablist">${tabs.map(([id, label, ic]) => `<button class="win-tab${id === active ? " on" : ""}" data-tab="${id}" role="tab">${icon(ic)}${esc(label)}</button>`).join("")}</div>
    <div data-panel></div>
    <div class="type-note">${icon("info")}${esc(t("system.refreshHint"))}</div>
  </div>`;
  const panel = root.querySelector("[data-panel]");
  const setTab = (id) => {
    active = id;
    root.querySelectorAll("[data-tab]").forEach((b) => b.classList.toggle("on", b.dataset.tab === id));
    renderTab();
    if (id === "overview") loadOverview();
    if (id === "processes") loadProcs();
    if (id === "services") loadServices();
    if (id === "logs") renderLogs();
  };
  root.querySelectorAll("[data-tab]").forEach((b) => b.addEventListener("click", () => setTab(b.dataset.tab)));
  root.querySelector("[data-refresh]").addEventListener("click", () => {
    if (active === "overview") loadOverview();
    if (active === "processes") loadProcs();
    if (active === "services") loadServices();
    if (active === "logs") loadLogs();
    toast("info", t("common.refresh"));
  });

  const setLoading = () => { panel.innerHTML = `<div class="panel"><div class="empty"><span class="spinner"></span><p>${esc(t("common.loading"))}</p></div></div>`; };
  const fail = (err, retry) => {
    panel.innerHTML = `<div class="panel"><div class="empty">${icon("alert", "icon")}<strong>${esc(t("toast.error"))}</strong><p>${esc(errorText(err))}</p><button class="btn" data-retry>${icon("refresh")}${esc(t("common.retry"))}</button></div></div>`;
    panel.querySelector("[data-retry]")?.addEventListener("click", retry);
  };

  const renderTab = () => {
    if (active === "overview") {
      panel.innerHTML = snap ? overview(snap) : `<div class="skeleton" style="height:300px;border-radius:14px"></div>`;
      panel.querySelectorAll("[data-refresh-overview]").forEach((b) => b.addEventListener("click", loadOverview));
    } else if (active === "processes") {
      panel.innerHTML = `<div class="panel"><div class="toolbar"><input class="input" data-proc-search placeholder="${esc(t("system.searchProcess"))}" autocomplete="off"><span class="count" data-proc-count></span><button class="btn ghost" data-proc-refresh>${icon("refresh")}${esc(t("common.refresh"))}</button></div><div class="table-wrap"><table class="tbl"><thead><tr><th>${esc(t("system.process"))}</th><th class="num">${esc(t("system.pid"))}</th><th>${esc(t("system.user"))}</th><th class="num">CPU</th><th class="num">${esc(t("system.mem"))}</th><th class="num">RSS</th>${admin ? `<th></th>` : ""}</tr></thead><tbody data-proc-body><tr><td colspan="7"><div class="empty"><span class="spinner"></span></div></td></tr></tbody></table></div></div>`;
      panel.querySelector("[data-proc-refresh]").addEventListener("click", loadProcs);
      panel.querySelector("[data-proc-search]").addEventListener("input", debounce(paintProcs, 120));
      paintProcs();
    } else if (active === "services") {
      panel.innerHTML = `<div class="panel"><div class="panel-head"><h3>${esc(t("system.services"))}</h3><button class="btn ghost" data-svc-refresh>${icon("refresh")}${esc(t("common.refresh"))}</button></div><div class="table-wrap"><table class="tbl"><thead><tr><th>${esc(t("system.service"))}</th><th>${esc(t("system.state"))}</th><th>${esc(t("system.description"))}</th><th></th></tr></thead><tbody data-svc-body><tr><td colspan="4"><div class="empty"><span class="spinner"></span></div></td></tr></tbody></table></div></div>`;
      panel.querySelector("[data-svc-refresh]").addEventListener("click", loadServices);
      paintServices();
    } else if (active === "logs") {
      panel.innerHTML = `<div class="panel"><div class="toolbar"><input class="input mono" data-log-unit value="${esc(logsUnit)}" placeholder="${esc(t("system.unitHint"))}"><select class="select" data-log-lines><option value="100">100</option><option value="300" selected>300</option><option value="1000">1000</option><option value="0">${esc(t("system.logsAll"))}</option></select><button class="btn" data-log-load>${icon("refresh")}${esc(t("common.refresh"))}</button></div><div class="panel-body"><pre class="logbox" data-logbox>${esc(logsText || t("system.logsEmpty"))}</pre></div></div>`;
      panel.querySelector("[data-log-load]").addEventListener("click", () => {
        logsUnit = panel.querySelector("[data-log-unit]").value.trim() || "mazimad";
        loadLogs();
      });
      panel.querySelector("[data-log-unit]").addEventListener("keydown", (e) => { if (e.key === "Enter") loadLogs(); });
      if (!logsText) loadLogs();
    } else {
      panel.innerHTML = `<div class="power-grid">
        <div class="panel"><div class="panel-head"><h3>${icon("restart")}${esc(t("system.reboot"))}</h3></div><div class="panel-body"><p class="type-note">${esc(t("system.powerWarning"))}</p><button class="btn" data-power="reboot">${icon("restart")}${esc(t("system.reboot"))}</button></div></div>
        <div class="panel danger-panel"><div class="panel-head"><h3>${icon("power")}${esc(t("system.poweroff"))}</h3></div><div class="panel-body"><p class="type-note">${esc(t("system.powerWarning"))}</p><button class="btn danger" data-power="poweroff">${icon("power")}${esc(t("system.poweroff"))}</button></div></div>
      </div>`;
      panel.querySelectorAll("[data-power]").forEach((b) => b.addEventListener("click", () => power(b.dataset.power)));
    }
  };

  const overview = (s) => {
    const disk = (s.disks || []).find((d) => d.mountpoint === "/") || (s.disks || [])[0];
    const mem = s.memory || {};
    const swap = s.swap || {};
    const temps = s.temperatures || [];
    const maxTemp = temps.length ? Math.max(...temps.map((x) => x.celsius)) : null;
    return `<div class="view-stack">
      <div class="widget-grid cols-4">
        <div class="stat-card"><div class="stat-top"><span>${esc(t("system.cpu"))}</span>${icon("cpu")}</div><div class="stat-value">${pct(s.cpu_percent, 1)}</div><div class="stat-sub">${esc(t("system.cores", { n: s.cpu_cores || 0 }))}</div></div>
        <div class="stat-card"><div class="stat-top"><span>${esc(t("system.memory"))}</span>${icon("memory")}</div><div class="stat-value">${pct(mem.used_percent, 1)}</div><div class="stat-sub">${esc(bytes(mem.used))} / ${esc(bytes(mem.total))}</div></div>
        <div class="stat-card"><div class="stat-top"><span>${esc(t("system.uptime"))}</span>${icon("clock")}</div><div class="stat-value">${esc(duration(s.uptime_seconds))}</div><div class="stat-sub">${esc(s.hostname || "")}</div></div>
        <div class="stat-card"><div class="stat-top"><span>${esc(t("system.temp"))}</span>${icon("thermometer")}</div><div class="stat-value">${maxTemp == null ? "—" : `${num(maxTemp, 1)}°`}</div><div class="stat-sub">${esc(temps.length ? `${temps.length} sensor${temps.length === 1 ? "" : "s"}` : t("system.noTemp"))}</div></div>
      </div>
      <div class="dash-bottom">
        <div class="panel"><div class="panel-head"><h3>${esc(t("system.cpu"))}</h3><span class="chip">load ${num(s.load?.[0], 2)} · ${num(s.load?.[1], 2)} · ${num(s.load?.[2], 2)}</span></div><div class="panel-body">
          <div class="gauge ${level(s.cpu_percent)}" style="height:10px"><i style="width:${Math.min(100, s.cpu_percent || 0)}%"></i></div>
          <dl class="kv" style="margin-top:12px"><dt>${esc(t("system.cores", { n: s.cpu_cores || 0 }))}</dt><dd>${esc(s.arch || "—")}</dd><dt>${esc(t("dash.kernel"))}</dt><dd>${esc(s.kernel || "—")}</dd><dt>${esc(t("dash.arch"))}</dt><dd>${esc(s.os || "—")}</dd><dt>NTP</dt><dd>${s.ntp ? esc(t("common.yes")) : esc(t("common.no"))}</dd></dl>
        </div></div>
        <div class="panel"><div class="panel-head"><h3>${esc(t("system.memory"))} · ${esc(t("system.swap"))}</h3></div><div class="panel-body">
          <div class="health-rows">
            <div class="health-row"><span>RAM</span><b>${esc(bytes(mem.used))} / ${esc(bytes(mem.total))}</b><div class="gauge ${level(mem.used_percent)}"><i style="width:${Math.min(100, mem.used_percent || 0)}%"></i></div></div>
            <div class="health-row"><span>${esc(t("system.swap"))}</span><b>${esc(bytes(swap.used))} / ${esc(bytes(swap.total))}</b><div class="gauge ${level(swap.used_percent)}"><i style="width:${Math.min(100, swap.used_percent || 0)}%"></i></div></div>
          </div>
        </div></div>
      </div>
      <div class="dash-bottom">
        <div class="panel"><div class="panel-head"><h3>${esc(t("system.disks"))}</h3></div><div class="panel-body"><div class="disk-list">${(s.disks || []).map((d) => `<div class="disk-item"><div class="row"><b>${esc(d.mountpoint)} · ${esc(d.fstype)}</b><span>${esc(bytes(d.free))} ${esc(t("common.none").toLowerCase())}</span></div><div class="gauge ${level(d.used_percent)}"><i style="width:${Math.min(100, d.used_percent || 0)}%"></i></div><div class="type-note" style="margin-top:4px">${esc(d.device)} · ${esc(bytes(d.used))} / ${esc(bytes(d.total))} (${pct(d.used_percent, 0)})</div></div>`).join("") || `<div class="type-note">${esc(t("common.noData"))}</div>`}</div></div></div>
        <div class="panel"><div class="panel-head"><h3>${esc(t("system.network"))}</h3></div><div class="panel-body"><div class="net-list">${(s.network || []).map((n) => `<div class="net-item"><div class="row"><b>${esc(n.name)}</b><span class="chip">${esc(n.state || "—")}</span></div><div class="type-note">${esc((n.addresses || []).join(" · ") || "—")}</div><div class="type-note">↓ ${esc(bytes(n.rx_bytes))} · ↑ ${esc(bytes(n.tx_bytes))}</div></div>`).join("") || `<div class="type-note">${esc(t("common.noData"))}</div>`}</div></div></div>
      </div>
      <div class="dash-bottom">
        <div class="panel"><div class="panel-head"><h3>${esc(t("system.temp"))}</h3></div><div class="panel-body"><div class="tile-grid">${temps.map((x) => `<div class="tile"><div class="k">${esc(x.sensor)}</div><div class="v">${num(x.celsius, 1)}°C</div>${x.high ? `<div class="type-note">max ${num(x.high, 1)}°C</div>` : ""}</div>`).join("") || `<div class="type-note">${esc(t("system.noTemp"))}</div>`}</div></div></div>
        <div class="panel"><div class="panel-head"><h3>${esc(t("system.gpu"))}</h3></div><div class="panel-body"><div class="tile-grid">${(s.gpus || []).map((g) => `<div class="tile"><div class="k">${esc(g.name)}</div><div class="type-note">${esc(g.driver || "—")}</div><div class="type-note">${esc(g.render_node || "")}</div><div class="type-note">${esc((g.encoders || []).join(", "))}</div></div>`).join("") || `<div class="type-note">${esc(t("system.noGpu"))}</div>`}</div></div></div>
      </div>
      <div class="type-note">${esc(t("system.uptime"))}: ${esc(duration(s.uptime_seconds))} · ${esc(timeOnly(s.time))}</div>
    </div>`;
  };

  const paintProcs = () => {
    const body = panel.querySelector("[data-proc-body]");
    if (!body) return;
    const q = (panel.querySelector("[data-proc-search]")?.value || "").toLowerCase();
    const list = procs.filter((p) => !q || `${p.name} ${p.user} ${p.command}`.toLowerCase().includes(q));
    const count = panel.querySelector("[data-proc-count]");
    if (count) count.textContent = `${list.length}`;
    body.innerHTML = list.length ? list.map((p) => `<tr><td><b>${esc(p.name)}</b>${p.command ? `<div class="type-note ellipsis">${esc(p.command)}</div>` : ""}</td><td class="num">${p.pid}</td><td>${esc(p.user || "—")}</td><td class="num">${pct(p.cpu_percent, 1)}</td><td class="num">${pct(p.memory_percent, 1)}</td><td class="num">${esc(bytes(p.rss))}</td>${admin ? `<td class="num"><button class="btn icon-only ghost" data-signal="${p.pid}" title="${esc(t("system.kill"))}">${icon("x")}</button></td>` : ""}</tr>`).join("") : `<tr><td colspan="7"><div class="empty"><p>${esc(t("system.noProcesses"))}</p></div></td></tr>`;
    body.querySelectorAll("[data-signal]").forEach((b) => b.addEventListener("click", () => {
      const proc = procs.find((p) => String(p.pid) === b.dataset.signal);
      if (proc) signal(proc);
    }));
  };
  const loadProcs = async () => {
    try {
      const res = await api.get("/api/system/processes?limit=200");
      procs = res.processes || [];
      paintProcs();
    } catch (err) { fail(err, loadProcs); }
  };

  const paintServices = () => {
    const body = panel.querySelector("[data-svc-body]");
    if (!body) return;
    body.innerHTML = services.length ? services.map((s) => `<tr><td><b>${esc(s.name)}</b><div class="type-note">${esc(s.load || "")}</div></td><td><span class="badge ${cls(s.active)}">${esc(s.active || "—")}${s.sub ? ` / ${esc(s.sub)}` : ""}</span></td><td class="type-note">${esc(s.description || "")}</td><td class="num"><div class="row-actions">${["start", "restart", "stop"].map((a) => `<button class="btn icon-only ghost" data-svc="${esc(s.name)}:${a}" title="${esc(t("common." + a))}">${icon(a === "start" ? "play" : a === "stop" ? "stop" : "restart")}</button>`).join("")}</div></td></tr>`).join("") : `<tr><td colspan="4"><div class="empty"><p>${esc(t("system.noServices"))}</p></div></td></tr>`;
    body.querySelectorAll("[data-svc]").forEach((b) => b.addEventListener("click", () => {
      const [name, action] = b.dataset.svc.split(":");
      serviceAction(name, action);
    }));
  };
  const loadServices = async () => {
    if (!admin) return;
    try {
      const res = await api.get("/api/system/services");
      services = res.services || [];
      paintServices();
    } catch (err) { fail(err, loadServices); }
  };

  const renderLogs = () => {
    logsText = "";
    renderTab();
  };
  const loadLogs = async () => {
    if (!admin) return;
    const unit = panel.querySelector("[data-log-unit]")?.value.trim() || logsUnit;
    const lines = panel.querySelector("[data-log-lines]")?.value || "300";
    logsUnit = unit;
    const box = panel.querySelector("[data-logbox]");
    if (box) box.textContent = t("common.loading");
    logsLoading = true;
    try {
      const res = await api.get(`/api/system/logs?unit=${encodeURIComponent(unit)}&lines=${encodeURIComponent(lines)}`);
      logsText = res.logs || t("system.logsEmpty");
      logsLoading = false;
      if (box) box.textContent = logsText;
    } catch (err) {
      logsLoading = false;
      logsText = errorText(err);
      if (box) box.textContent = logsText;
      toast("bad", t("toast.error"), errorText(err));
    }
  };

  const serviceAction = async (name, action) => {
    if (action !== "start") {
      const ok = await new Promise((resolve) => {
        const m = modal({ title: `${t("system." + action)} · ${name}`, body: `<p class="type-note">${esc(t("system.powerWarning"))}</p>`, actions: [
          { label: t("common.cancel"), onClick: (close) => { close(); resolve(false); } },
          { label: t("common." + action), kind: "danger", onClick: (close) => { close(); resolve(true); } },
        ], onClose: () => resolve(false) });
      });
      if (!ok) return;
    }
    try {
      await api.post("/api/system/services", { name, action });
      toast("ok", t("system.actionDone"), `${name} · ${action}`);
      loadServices();
    } catch (err) { toast("bad", t("toast.error"), errorText(err)); }
  };

  const signal = async (proc) => {
    try {
      const data = await typedConfirm({
        title: t("system.killTitle"),
        message: t("system.killBody", { signal: "TERM", name: proc.name, pid: proc.pid }),
        word: "SIGNAL",
        password: true,
        confirmLabel: t("system.kill"),
        extra: `<div class="field"><label>${esc(t("system.signal"))}</label><select class="select" id="sig-kind">${["TERM", "KILL", "INT", "HUP"].map((s) => `<option>${s}</option>`).join("")}</select></div>`,
      });
      await api.post("/api/system/processes/signal", {
        pid: proc.pid,
        signal: data.form.querySelector("#sig-kind").value,
        password: data.password,
        confirm: true,
      });
      toast("ok", t("system.actionDone"), `${proc.name} (${proc.pid})`);
      loadProcs();
    } catch (err) {
      if (err?.message !== "cancelled") toast("bad", t("toast.error"), errorText(err));
    }
  };

  const power = async (action) => {
    const word = action === "reboot" ? "REBOOT" : "POWEROFF";
    try {
      const data = await typedConfirm({
        title: t("system." + action),
        message: action === "reboot" ? t("system.confirmReboot") : t("system.confirmPoweroff"),
        word,
        password: true,
        confirmLabel: t("system." + action),
      });
      await api.post("/api/system/power", { action, password: data.password, confirm: word });
      toast("warn", t("system.actionDone"), t("system.actionDoneBody"));
    } catch (err) {
      if (err?.message !== "cancelled") toast("bad", t("toast.error"), errorText(err));
    }
  };

  const loadOverview = async () => {
    try {
      snap = await api.get("/api/system");
      if (active === "overview") renderTab();
    } catch (err) { fail(err, loadOverview); }
  };
  loadOverview();
  timer = setInterval(() => { if (active === "overview" && alive) loadOverview(); }, 5000);
  return () => { alive = false; clearInterval(timer); };
}
