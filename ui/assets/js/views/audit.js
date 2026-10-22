import { api } from "../api.js";
import { dateTime, debounce, esc, modal, relative } from "../dom.js";
import { errorText, t } from "../i18n.js";
import { icon } from "../icons.js";

export function renderAudit(root, ctx) {
  const admin = ctx.user?.role === "admin";
  let events = [];
  let query = "";
  let result = "";
  let loading = false;
  if (!admin) {
    root.innerHTML = `<div class="view-stack"><div class="section-head"><h2>${esc(t("audit.title"))}</h2></div><div class="panel"><div class="empty">${icon("shield", "icon")}<strong>${esc(t("common.adminOnly"))}</strong><p>${esc(t("common.permissionDenied"))}</p></div></div></div>`;
    return () => {};
  }
  root.innerHTML = `<div class="view-stack">
    <div class="section-head"><div><h2>${esc(t("audit.title"))}</h2><div class="type-note">${esc(t("audit.subtitle"))}</div></div><button class="btn ghost" data-refresh>${icon("refresh")}${esc(t("common.refresh"))}</button></div>
    <div class="panel"><div class="toolbar">
      <input class="input" data-search placeholder="${esc(t("audit.search"))}" autocomplete="off">
      <select class="select" data-result><option value="">${esc(t("common.all"))}</option><option value="ok">${esc(t("audit.ok"))}</option><option value="denied">${esc(t("audit.denied"))}</option><option value="error">${esc(t("audit.error"))}</option></select>
      <span class="count" data-count></span>
    </div><div data-list><div class="empty"><span class="spinner"></span></div></div></div>
  </div>`;
  const list = root.querySelector("[data-list]");
  const count = root.querySelector("[data-count]");
  const paint = () => {
    count.textContent = `${events.length}`;
    list.innerHTML = events.length ? `<div class="audit-list">${events.map((e, i) => `<button class="audit-row" data-i="${i}">
      <span class="when" title="${esc(dateTime(e.time))}">${esc(relative(e.time))}</span>
      <span class="who">${icon(e.actor_type === "user" ? "user" : e.actor_type === "token" ? "key" : e.actor_type === "system" ? "server" : "globe")}<span>${esc(e.actor || t("audit.system"))}</span></span>
      <span class="what"><b>${esc(e.action || "—")}</b>${e.resource ? `<span class="type-note">${esc(e.resource)}</span>` : ""}</span>
      <span class="badge ${e.result === "ok" ? "ok" : e.result === "denied" ? "warn" : "bad"}">${esc(e.result === "ok" ? t("audit.ok") : e.result === "denied" ? t("audit.denied") : t("audit.error"))}</span>
      <span class="type-note mono">${esc(e.ip || "")}</span>
    </button>`).join("")}</div>` : `<div class="empty">${icon("audit", "icon")}<strong>${esc(t("audit.empty"))}</strong><p>${esc(t("audit.emptyBody"))}</p></div>`;
    list.querySelectorAll("[data-i]").forEach((b) => b.addEventListener("click", () => details(events[Number(b.dataset.i)])));
  };
  const details = (e) => {
    modal({
      title: e.action || t("audit.details"),
      subtitle: dateTime(e.time),
      wide: true,
      body: `<dl class="kv">
        <dt>${esc(t("audit.when"))}</dt><dd>${esc(dateTime(e.time))} · ${esc(relative(e.time))}</dd>
        <dt>${esc(t("audit.actor"))}</dt><dd>${esc(e.actor || "—")} (${esc(e.actor_type || "—")})</dd>
        <dt>${esc(t("audit.action"))}</dt><dd class="mono">${esc(e.action || "—")}</dd>
        <dt>${esc(t("audit.resource"))}</dt><dd class="mono">${esc(e.resource || "—")}</dd>
        <dt>${esc(t("audit.result"))}</dt><dd><span class="badge ${e.result === "ok" ? "ok" : e.result === "denied" ? "warn" : "bad"}">${esc(e.result || "—")}</span></dd>
        <dt>IP</dt><dd class="mono">${esc(e.ip || "—")}</dd>
        <dt>User agent</dt><dd class="mono" style="max-width:420px">${esc(e.user_agent || "—")}</dd>
        <dt>Request ID</dt><dd class="mono">${esc(e.request_id || "—")}</dd>
      </dl>
      ${e.details && Object.keys(e.details).length ? `<div class="field" style="margin-top:14px"><label>${esc(t("common.details"))}</label><pre class="logbox">${esc(Object.entries(e.details).map(([k, v]) => `${k}: ${v}`).join("\n"))}</pre></div>` : ""}`,
      actions: [{ label: t("common.close"), onClick: (close) => close() }],
    });
  };
  const load = async () => {
    if (loading) return;
    loading = true;
    const params = new URLSearchParams({ limit: "300" });
    if (query) params.set("q", query);
    if (result) params.set("result", result);
    try {
      const res = await api.get(`/api/audit?${params.toString()}`);
      events = res.events || [];
      paint();
    } catch (err) {
      list.innerHTML = `<div class="empty">${icon("alert", "icon")}<strong>${esc(t("toast.error"))}</strong><p>${esc(errorText(err))}</p><button class="btn" data-retry>${icon("refresh")}${esc(t("common.retry"))}</button></div>`;
      list.querySelector("[data-retry]")?.addEventListener("click", load);
    }
    loading = false;
  };
  root.querySelector("[data-search]").addEventListener("input", debounce((e) => { query = e.target.value.trim(); load(); }, 260));
  root.querySelector("[data-result]").addEventListener("change", (e) => { result = e.target.value; load(); });
  root.querySelector("[data-refresh]").addEventListener("click", load);
  load();
  return () => {};
}
