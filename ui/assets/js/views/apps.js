import { api } from "../api.js";
import { esc, modal, setBusy, toast, typedConfirm } from "../dom.js";
import { errorText, t } from "../i18n.js";
import { icon } from "../icons.js";

export function renderApps(root, ctx) {
  const admin = ctx.user?.role === "admin";
  const lang = ctx.settings?.language || "it";
  let apps = [];
  let docker = { available: false, compose: false };
  let query = "";
  let category = "";
  let alive = true;

  const loc = (app, field) => app[`${field}_i18n`]?.[lang] || app[field] || "";
  const gradient = (name) => {
    let h = 0;
    for (const c of String(name)) h = (h * 31 + c.charCodeAt(0)) % 360;
    return `linear-gradient(135deg,hsl(${h} 62% 48%),hsl(${(h + 55) % 360} 68% 38%))`;
  };
  const appIcon = (app) => {
    const name = loc(app, "name");
    if (app.icon && app.icon.startsWith("/")) return `<img src="${esc(app.icon)}" alt="" loading="lazy">`;
    return `<span>${esc((name || app.id || "?").trim().charAt(0).toUpperCase())}</span>`;
  };
  const status = (app) => {
    if (!app.installed) return { cls: "muted", label: t("apps.notInstalled") };
    if (app.status === "running") return { cls: "ok", label: t("apps.running") };
    if (app.status) return { cls: "warn", label: app.status };
    return { cls: "ok", label: t("apps.installed") };
  };
  root.innerHTML = `<div class="view-stack">
    <div class="section-head"><div><h2>${esc(t("apps.title"))}</h2><div class="type-note">${esc(t("apps.subtitle"))}</div></div><button class="btn ghost" data-refresh>${icon("refresh")}${esc(t("common.refresh"))}</button></div>
    <div data-banner></div>
    <div class="panel"><div class="toolbar">
      <input class="input" data-search placeholder="${esc(t("apps.search"))}" autocomplete="off">
      <select class="select" data-cat><option value="">${esc(t("common.all"))}</option></select>
      <span class="count" data-count></span>
    </div><div class="card-grid" data-grid></div></div>
  </div>`;
  const grid = root.querySelector("[data-grid]");
  const catSel = root.querySelector("[data-cat]");
  const count = root.querySelector("[data-count]");
  const banner = root.querySelector("[data-banner]");

  const showBanner = () => {
    if (docker.available && docker.compose) { banner.innerHTML = ""; return; }
    banner.innerHTML = `<div class="notice ${docker.available ? "warn" : "bad"}">${icon("alert")}<div><strong>${esc(docker.available ? t("apps.composeMissing") : t("apps.dockerMissing"))}</strong><p>${esc(docker.available ? t("apps.composeMissingBody") : (docker.error || t("apps.dockerMissingBody")))}</p></div></div>`;
  };
  const paint = () => {
    showBanner();
    const q = query.toLowerCase();
    const list = apps.filter((a) => {
      if (category && a.category !== category) return false;
      if (!q) return true;
      return `${a.name} ${a.description} ${loc(a, "name")} ${loc(a, "description")} ${a.category} ${a.id}`.toLowerCase().includes(q);
    });
    count.textContent = `${list.length} / ${apps.length}`;
    grid.innerHTML = list.length ? list.map((a) => {
      const st = status(a);
      return `<article class="app-card" data-id="${esc(a.id)}">
        <div class="app-icon" style="background:${gradient(a.name)}">${appIcon(a)}</div>
        <div class="app-main">
          <div class="app-name">${esc(loc(a, "name"))}<span class="badge ${st.cls}">${esc(st.label)}</span></div>
          <div class="app-desc">${esc(loc(a, "description"))}</div>
          <div class="app-tags"><span class="chip">${esc(a.category || "app")}</span><span class="chip">v${esc(a.version || "—")}</span>${a.gpu ? `<span class="chip">${icon("cpu")}GPU</span>` : ""}${a.min_ram_mb ? `<span class="chip">${icon("memory")}${esc(a.min_ram_mb)} MB</span>` : ""}</div>
        </div>
        <div class="app-actions">
          ${a.installed ? `<button class="btn primary sm" data-open>${icon("external")}${esc(t("apps.open"))}</button>` : `<button class="btn primary sm" data-install ${(!admin || !docker.available || !docker.compose) ? "disabled" : ""}>${icon("download")}${esc(t("apps.install"))}</button>`}
          <button class="btn icon-only ghost" data-details title="${esc(t("common.details"))}">${icon("info")}</button>
        </div>
      </article>`;
    }).join("") : `<div class="empty" style="grid-column:1/-1">${icon("apps", "icon")}<strong>${esc(t("apps.empty"))}</strong><p>${esc(t("apps.emptyBody"))}</p></div>`;
    grid.querySelectorAll(".app-card").forEach((card) => bindCard(card));
  };
  const byId = (id) => apps.find((a) => a.id === id);
  const bindCard = (card) => {
    const app = byId(card.dataset.id);
    if (!app) return;
    card.querySelector("[data-details]")?.addEventListener("click", () => details(app));
    card.querySelector("[data-install]")?.addEventListener("click", (e) => { e.stopPropagation(); install(app, e.currentTarget); });
    card.querySelector("[data-open]")?.addEventListener("click", (e) => { e.stopPropagation(); open(app); });
  };
  const open = (app) => {
    if (!app.url) { toast("warn", t("apps.openFailed"), t("apps.openFailed")); return; }
    window.open(app.url, "_blank", "noopener");
  };
  const load = async () => {
    try {
      const res = await api.get("/api/apps");
      apps = res.apps || [];
      docker = res.docker || { available: false, compose: false };
      const cats = [...new Set(apps.map((a) => a.category).filter(Boolean))].sort();
      const prev = category;
      catSel.innerHTML = `<option value="">${esc(t("common.all"))}</option>${cats.map((c) => `<option value="${esc(c)}">${esc(c)}</option>`).join("")}`;
      catSel.value = cats.includes(prev) ? prev : "";
      category = catSel.value;
      paint();
    } catch (err) {
      grid.innerHTML = `<div class="empty" style="grid-column:1/-1">${icon("alert", "icon")}<strong>${esc(t("apps.catalogUnavailable"))}</strong><p>${esc(errorText(err))}</p><button class="btn" data-retry>${icon("refresh")}${esc(t("common.retry"))}</button></div>`;
      grid.querySelector("[data-retry]")?.addEventListener("click", load);
    }
  };
  const install = async (app, button = null) => {
    const m = modal({
      title: t("apps.installTitle", { name: loc(app, "name") }),
      body: `<div class="app-detail-head"><div class="app-icon" style="background:${gradient(app.name)}">${appIcon(app)}</div><div><b>${esc(loc(app, "name"))}</b><div class="type-note">${esc(loc(app, "description"))}</div></div></div><p class="type-note" style="margin-top:12px">${esc(t("apps.installBody"))}</p>`,
      actions: [
        { label: t("common.cancel"), onClick: (close) => close() },
        { label: t("apps.install"), kind: "primary", icon: "download", onClick: async (close, btn) => {
          setBusy(btn, true, t("apps.installing"));
          setBusy(button, true, t("apps.installing"));
          try {
            const res = await api.post(`/api/apps/${encodeURIComponent(app.id)}/install`, {});
            toast("ok", t("apps.installedToast", { name: loc(res.app || app, "name") }));
            close();
            await load();
          } catch (err) {
            setBusy(btn, false);
            setBusy(button, false);
            toast("bad", t("apps.actionFailed"), errorText(err));
          }
        } },
      ],
    });
  };
  const update = async (app) => {
    try {
      await api.post(`/api/apps/${encodeURIComponent(app.id)}/update`, {});
      toast("ok", t("common.success"), `${loc(app, "name")} · ${t("common.update")}`);
      load();
    } catch (err) { toast("bad", t("apps.actionFailed"), errorText(err)); }
  };
  const remove = async (app) => {
    try {
      const data = await typedConfirm({
        title: t("apps.removeTitle", { name: loc(app, "name") }),
        message: t("apps.removeBody"),
        word: loc(app, "name"),
        extra: `<label class="check"><input type="checkbox" id="app-purge"> ${esc(t("apps.removePurge"))}</label>`,
        confirmLabel: t("apps.remove"),
      });
      const purge = data.form.querySelector("#app-purge")?.checked;
      await api.del(`/api/apps/${encodeURIComponent(app.id)}?purge=${purge ? "true" : "false"}`, { confirm: app.id });
      toast("ok", t("apps.removedToast", { name: loc(app, "name") }));
      load();
    } catch (err) {
      if (err?.message !== "cancelled") toast("bad", t("apps.actionFailed"), errorText(err));
    }
  };
  const logs = async (app) => {
    const m = modal({
      title: t("apps.logsTitle", { name: loc(app, "name") }),
      wide: true,
      body: `<pre class="logbox" data-log>${esc(t("common.loading"))}</pre>`,
      actions: [{ label: t("common.close"), onClick: (close) => close() }],
    });
    try {
      const res = await api.get(`/api/apps/${encodeURIComponent(app.id)}/logs?tail=500`);
      m.body.querySelector("[data-log]").textContent = res.logs || t("system.logsEmpty");
    } catch (err) { m.body.querySelector("[data-log]").textContent = errorText(err); }
  };
  const details = (app) => {
    const locName = loc(app, "name");
    const m = modal({
      title: locName,
      wide: true,
      body: `<div class="app-detail-head"><div class="app-icon" style="background:${gradient(app.name)}">${appIcon(app)}</div><div><div class="app-name">${esc(locName)} <span class="chip">v${esc(app.version || "—")}</span></div><div class="type-note">${esc(loc(app, "description"))}</div><div class="app-tags"><span class="chip">${esc(app.category || "")}</span>${app.gpu ? `<span class="chip">GPU</span>` : ""}${app.min_ram_mb ? `<span class="chip">${esc(t("apps.ram"))} ${esc(app.min_ram_mb)} MB</span>` : ""}${app.installed ? `<span class="badge ok">${esc(t("apps.installed"))}</span>` : ""}</div></div></div>
        <dl class="kv" style="margin-top:16px">
          <dt>${esc(t("common.version"))}</dt><dd>${esc(app.version || "—")}</dd>
          <dt>${esc(t("apps.category"))}</dt><dd>${esc(app.category || "—")}</dd>
          <dt>${esc(t("apps.port"))}</dt><dd>${esc(app.port || "—")}${app.host_port ? ` → ${esc(app.host_port)}` : ""}</dd>
          <dt>${esc(t("apps.source"))}</dt><dd>${esc(app.maintainer || app.source || "—")}</dd>
          <dt>${esc(t("common.name"))}</dt><dd>${esc(app.id)}</dd>
          ${app.website ? `<dt>${esc(t("apps.website"))}</dt><dd>${esc(app.website)}</dd>` : ""}
          <dt>${esc(t("apps.dataSize"))}</dt><dd>${esc(app.data_size ? `${Math.round(app.data_size / 1048576)} MB` : "—")}</dd>
          ${app.installed_at ? `<dt>${esc(t("apps.installedAt"))}</dt><dd>${esc(new Date(app.installed_at).toLocaleDateString())}</dd>` : ""}
        </dl>
        ${app.notes?.[lang] || app.notes?.en ? `<div class="notice info" style="margin-top:12px">${icon("info")}<div>${esc(app.notes[lang] || app.notes.en)}</div></div>` : ""}`,
      actions: [
        ...(app.installed ? [{ label: t("apps.open"), kind: "primary", icon: "external", onClick: (close) => { close(); open(app); } }] : []),
        ...(app.installed && admin ? [{ label: t("common.update"), icon: "updates", onClick: (close) => { close(); update(app); } }] : []),
        ...(app.installed && admin ? [{ label: t("apps.logs"), icon: "file", onClick: (close) => { close(); logs(app); } }] : []),
        ...(app.installed && admin ? [{ label: t("apps.remove"), kind: "danger", icon: "trash", onClick: (close) => { close(); remove(app); } }] : []),
        ...(!app.installed ? [{ label: t("apps.install"), kind: "primary", icon: "download", onClick: (close) => { close(); install(app); } }] : []),
        { label: t("common.close"), onClick: (close) => close() },
      ],
    });
  };
  root.querySelector("[data-search]").addEventListener("input", (e) => { query = e.target.value.trim(); paint(); });
  catSel.addEventListener("change", () => { category = catSel.value; paint(); });
  root.querySelector("[data-refresh]").addEventListener("click", load);
  load();
  return () => { alive = false; };
}
