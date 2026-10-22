import { api } from "../api.js";
import { icon } from "../icons.js";
import { t } from "../i18n.js";
import { bytes, duration, esc, historyPoint, level, num, pct, sparkline, timeOnly } from "../dom.js";

export function renderDashboard(root, ctx) {
  let alive = true;
  let snap = null;
  let history = [];
  let services = [];
  let timer = null;
  let histTimer = null;
  let servicesTimer = null;

  const greeting = () => {
    const h = new Date().getHours();
    return h < 12 ? t("dash.goodMorning") : h < 18 ? t("dash.goodAfternoon") : t("dash.goodEvening");
  };
  const displayName = () => (ctx.user?.display_name || ctx.user?.username || "").split(" ")[0] || t("common.admin");
  const rootDisk = (disks = []) => disks.find((d) => d.mountpoint === "/") || disks.find((d) => d.mountpoint === "/System/Volumes/Data") || disks[0];
  const netTotals = (net = []) => net.reduce((a, n) => ({ rx: a.rx + (n.rx_bytes || 0), tx: a.tx + (n.tx_bytes || 0) }), { rx: 0, tx: 0 });
  const health = (s) => {
    if (!s) return { score: 0, label: t("common.loading") };
    const disk = rootDisk(s.disks);
    let score = 100;
    score -= Math.max(0, (s.cpu_percent || 0) - 40) * 0.35;
    score -= Math.max(0, (s.memory?.used_percent || 0) - 55) * 0.3;
    score -= Math.max(0, (disk?.used_percent || 0) - 65) * 0.45;
    score = Math.max(4, Math.min(100, Math.round(score)));
    return { score, label: score >= 80 ? t("dash.healthy") : score >= 55 ? t("dash.attention") : t("dash.critical") };
  };
  const paint = () => {
    if (!alive) return;
    if (!snap) {
      root.innerHTML = `<div class="dashboard"><div class="skeleton" style="height:190px;border-radius:18px"></div><div class="widget-grid">${Array(4).fill('<div class="skeleton" style="height:96px;border-radius:14px"></div>').join("")}</div><div class="skeleton" style="height:240px;border-radius:14px"></div></div>`;
      return;
    }
    const disk = rootDisk(snap.disks);
    const net = netTotals(snap.network);
    const h = health(snap);
    const mem = snap.memory || {};
    const load = snap.load || [0, 0, 0];
    const scoreClass = h.score >= 80 ? "" : h.score >= 55 ? "warn" : "bad";
    const serviceRows = services.slice(0, 5).map((s) => `<div class="svc"><span class="dot ${s.active === "active" ? "" : s.active === "failed" ? "bad" : "warn"}"></span><span class="nm">${esc(s.name)}</span><span class="st">${esc(s.active || "—")}</span></div>`).join("");
    root.innerHTML = `<div class="dashboard">
      <section class="hero">
        <div class="hero-card">
          <div class="hero-top"><span class="chip ${h.score >= 80 ? "ok" : h.score >= 55 ? "warn" : "bad"}">${icon(h.score >= 80 ? "check" : "alert")}${esc(h.label)}</span><span class="chip">${icon("server")}${esc(snap.hostname || ctx.settings?.hostname || "mazima")}</span></div>
          <h1>${esc(t("dash.title", { greeting: greeting(), name: displayName() }))}</h1>
          <p>${esc(t("dash.subtitle", { host: snap.hostname || "MazimaOS" }))}</p>
          <div class="hero-meta">
            <span>${icon("clock")}${esc(t("dash.uptime"))} · ${esc(duration(snap.uptime_seconds))}</span>
            <span>${icon("cpu")}${esc(snap.arch || "—")}</span>
            <span>${icon("system")}${esc(snap.kernel || "—")}</span>
            <span>${icon("wifi")}${snap.ntp ? "NTP" : esc(t("common.unknown"))}</span>
          </div>
          <div class="hero-actions">
            <button class="btn primary" data-open="apps">${icon("plus")}${esc(t("dash.quickApps"))}</button>
            <button class="btn" data-session="terminal">${icon("terminal")}${esc(t("dash.quickTerminal"))}</button>
            <button class="btn ghost" data-open="system">${icon("system")}${esc(t("dash.quickSystem"))}</button>
          </div>
        </div>
        <div class="health-card">
          <div class="health-head"><h3>${esc(t("dash.health"))}</h3><span class="chip ${h.score >= 80 ? "ok" : h.score >= 55 ? "warn" : "bad"}">${esc(h.label)}</span></div>
          <div class="health-score"><b>${h.score}</b><span>/100 · ${esc(t("dash.healthOf"))}</span></div>
          <div class="health-rows">
            <div class="health-row"><span>${esc(t("dash.cpu"))}</span><b>${pct(snap.cpu_percent)}</b><div class="gauge ${level(snap.cpu_percent)}"><i style="width:${Math.min(100, snap.cpu_percent || 0)}%"></i></div></div>
            <div class="health-row"><span>${esc(t("dash.memory"))}</span><b>${pct(mem.used_percent)}</b><div class="gauge ${level(mem.used_percent)}"><i style="width:${Math.min(100, mem.used_percent || 0)}%"></i></div></div>
            <div class="health-row"><span>${esc(t("dash.disk"))}</span><b>${pct(disk?.used_percent)}</b><div class="gauge ${level(disk?.used_percent)}"><i style="width:${Math.min(100, disk?.used_percent || 0)}%"></i></div></div>
            <div class="health-row"><span>${esc(t("dash.load"))}</span><b>${num(load[0], 2)} / ${num(load[1], 2)} / ${num(load[2], 2)}</b></div>
          </div>
          <div class="type-note">${icon("info")} ${esc(t("system.refreshHint"))}</div>
        </div>
      </section>
      <section class="widget-grid">
        <div class="stat-card"><div class="stat-top"><span>${esc(t("dash.cpu"))}</span>${icon("cpu")}</div><div class="stat-value">${pct(snap.cpu_percent, 1)}</div><div class="stat-sub">${esc(t("system.cores", { n: snap.cpu_cores || 0 }))} · load ${num(load[0], 2)}</div></div>
        <div class="stat-card"><div class="stat-top"><span>${esc(t("dash.memory"))}</span>${icon("memory")}</div><div class="stat-value">${pct(mem.used_percent, 1)}</div><div class="stat-sub">${esc(bytes(mem.used))} / ${esc(bytes(mem.total))}</div></div>
        <div class="stat-card"><div class="stat-top"><span>${esc(t("dash.disk"))}</span>${icon("disk")}</div><div class="stat-value">${pct(disk?.used_percent, 1)}</div><div class="stat-sub">${esc(bytes(disk?.used))} / ${esc(bytes(disk?.total))}</div></div>
        <div class="stat-card"><div class="stat-top"><span>${esc(t("dash.network"))}</span>${icon("network")}</div><div class="stat-value">${esc(bytes(net.rx))}</div><div class="stat-sub">${esc(t("dash.received"))} · ${esc(t("dash.sent"))} ${esc(bytes(net.tx))}</div></div>
      </section>
      <section class="quick-grid">
        <button class="quick-card" data-open="apps"><span class="qicon">${icon("apps")}</span><span><strong>${esc(t("dash.quickApps"))}</strong><span>${esc(t("dash.quickAppsBody"))}</span></span></button>
        <button class="quick-card" data-session="terminal"><span class="qicon">${icon("terminal")}</span><span><strong>${esc(t("dash.quickTerminal"))}</strong><span>${esc(t("dash.quickTerminalBody"))}</span></span></button>
        <button class="quick-card" data-open="system"><span class="qicon">${icon("system")}</span><span><strong>${esc(t("dash.quickSystem"))}</strong><span>${esc(t("dash.quickSystemBody"))}</span></span></button>
      </section>
      <section class="dash-bottom">
        <div class="panel"><div class="panel-head"><h3>${esc(t("dash.performance"))}</h3><div class="legend"><span><i style="background:var(--accent)"></i>CPU</span><span><i style="background:var(--accent-2)"></i>RAM</span></div></div>
          <div class="chartbox"><div class="spark-wrap"><canvas id="dash-spark"></canvas></div><div class="type-note" style="margin-top:8px">${esc(t("dash.lastHour"))}</div></div></div>
        <div class="panel"><div class="panel-head"><h3>${esc(t("dash.storage"))}</h3><span class="chip">${esc(t("system.disks"))}</span></div><div class="panel-body"><div class="disk-list">
          ${(snap.disks || []).slice(0, 4).map((d) => `<div class="disk-item"><div class="row"><b>${esc(d.mountpoint || d.device)}</b><span>${esc(bytes(d.used))} / ${esc(bytes(d.total))}</span></div><div class="gauge ${level(d.used_percent)}"><i style="width:${Math.min(100, d.used_percent || 0)}%"></i></div></div>`).join("") || `<div class="muted type-note">${esc(t("dash.noData"))}</div>`}
        </div></div></div>
      </section>
      <section class="dash-bottom">
        <div class="panel"><div class="panel-head"><h3>${esc(t("dash.services"))}</h3><span class="chip">${services.length || 0}</span></div><div class="panel-body tight services-mini">${serviceRows || `<div class="muted type-note" style="padding:8px">${esc(t("dash.noServices"))}</div>`}</div></div>
        <div class="panel"><div class="panel-head"><h3>${esc(t("dash.host"))}</h3><span class="chip">${icon("shield")}${esc(t("dash.online"))}</span></div><div class="panel-body"><dl class="kv">
          <dt>${esc(t("settings.hostname"))}</dt><dd>${esc(snap.hostname || "—")}</dd>
          <dt>${esc(t("dash.kernel"))}</dt><dd>${esc(snap.kernel || "—")}</dd>
          <dt>${esc(t("dash.arch"))}</dt><dd>${esc(snap.arch || "—")}</dd>
          <dt>${esc(t("dash.uptime"))}</dt><dd>${esc(duration(snap.uptime_seconds))}</dd>
          <dt>${esc(t("dash.temp"))}</dt><dd>${snap.temperatures?.length ? esc(`${num(Math.max(...snap.temperatures.map((x) => x.celsius)), 1)} °C`) : "—"}</dd>
          <dt>${esc(t("common.online"))}</dt><dd>${esc(timeOnly(snap.time))}</dd>
        </dl></div></div>
      </section>
    </div>`;
    root.querySelectorAll("[data-open]").forEach((b) => b.addEventListener("click", () => ctx.open(b.dataset.open)));
    root.querySelectorAll("[data-session]").forEach((b) => b.addEventListener("click", () => ctx.openSession(b.dataset.session)));
    const canvas = root.querySelector("#dash-spark");
    if (canvas) sparkline(canvas, history);
  };

  const fail = (err) => {
    if (!alive) return;
    root.innerHTML = `<div class="dashboard"><div class="panel"><div class="empty">${icon("alert", "icon")}<strong>${esc(t("toast.error"))}</strong><p>${esc(err?.message || String(err))}</p><button class="btn" data-retry>${icon("refresh")}${esc(t("common.retry"))}</button></div></div></div>`;
    root.querySelector("[data-retry]")?.addEventListener("click", () => load());
  };
  const load = async () => {
    try {
      const s = await api.get("/api/system");
      snap = s;
      if (!history.length) {
        try {
          const hs = await api.get("/api/system/history");
          history = (hs.history || []).map(historyPoint);
        } catch {}
      }
      if (!history.length) history = [historyPoint(s)];
      paint();
    } catch (err) { fail(err); }
  };
  const loadHistory = async () => {
    try {
      const hs = await api.get("/api/system/history");
      history = (hs.history || []).map(historyPoint);
      const canvas = root.querySelector("#dash-spark");
      if (canvas) sparkline(canvas, history);
    } catch {}
  };
  const loadServices = async () => {
    if (ctx.user?.role !== "admin") return;
    try {
      const res = await api.get("/api/system/services");
      services = res.services || [];
      if (snap) paint();
    } catch {}
  };
  load();
  loadServices();
  timer = setInterval(load, 5000);
  histTimer = setInterval(loadHistory, 30000);
  servicesTimer = setInterval(loadServices, 60000);
  return () => { alive = false; clearInterval(timer); clearInterval(histTimer); clearInterval(servicesTimer); };
}
