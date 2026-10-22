import { api } from "./api.js";
import { applyPreferences, confirmDialog, esc, toast } from "./dom.js";
import { t, setLanguage, getLanguage, locale } from "./i18n.js";
import { brandMark, icon } from "./icons.js";
import { WindowManager } from "./windows.js";
import { renderDashboard } from "./views/dashboard.js";
import { renderApps } from "./views/apps.js";
import { renderDocker } from "./views/docker.js";
import { renderSystem } from "./views/system.js";
import { renderSettings } from "./views/settings.js";
import { renderRemote } from "./views/remote.js";
import { renderUpdates } from "./views/updates.js";
import { renderAudit } from "./views/audit.js";
import { renderSession } from "./views/session.js";

const APP_DEFS = {
  apps: { view: renderApps, title: "nav.apps", icon: "apps", width: 980, height: 680 },
  docker: { view: renderDocker, title: "nav.docker", icon: "docker", width: 1080, height: 720, admin: true },
  system: { view: renderSystem, title: "nav.system", icon: "system", width: 1000, height: 700 },
  settings: { view: renderSettings, title: "nav.settings", icon: "settings", width: 920, height: 660, pad: false },
  remote: { view: renderRemote, title: "nav.remote", icon: "globe", width: 940, height: 700, admin: true },
  updates: { view: renderUpdates, title: "nav.updates", icon: "updates", width: 840, height: 650 },
  audit: { view: renderAudit, title: "nav.audit", icon: "audit", width: 1000, height: 680, admin: true },
};

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

export function renderDesktop(root, bootstrap) {
  const { user, settings, desktop: bootstrapDesktop, version, status } = bootstrap;
  const desktopState = bootstrapDesktop || { windows: [], virtual_desktops: 2, active_desktop: 0 };
  if (!Array.isArray(desktopState.windows)) desktopState.windows = [];
  if (!desktopState.virtual_desktops) desktopState.virtual_desktops = 2;
  let active = "dashboard";
  let saveTimer = null;
  let snap = null;
  let metricsTimer = null;
  let clockTimer = null;
  let loggingOut = false;
  let palette = null;
  let menu = null;
  let dashboardCleanup = null;
  let lastLanguage = settings?.language || getLanguage();

  applyPreferences(settings, setLanguage);
  document.title = `MazimaOS · ${status?.hostname || settings?.hostname || "MazimaOS"}`;

  root.innerHTML = `<div class="desktop">
    <header class="topbar">
      <div class="brand">${brandMark()}<div><div class="brand-name">Mazima<span>OS</span></div><div class="brand-sub">${esc(t("app.tagline"))}</div></div></div>
      <div class="topbar-center">
        <div class="topbar-clock"><span data-clock>--:--</span><small data-date></small></div>
        <div class="topbar-metrics" data-metrics>
          <button class="metric-mini" data-metric="cpu" title="CPU">${icon("cpu")}<span>CPU</span><div class="bar"><i></i></div><b>--</b></button>
          <button class="metric-mini" data-metric="memory" title="RAM">${icon("memory")}<span>RAM</span><div class="bar"><i></i></div><b>--</b></button>
          <button class="metric-mini" data-metric="disk" title="Disk">${icon("disk")}<span>DISK</span><div class="bar"><i></i></div><b>--</b></button>
        </div>
      </div>
      <div class="topbar-actions">
        <button class="topbar-btn" data-command title="${esc(t("nav.command"))}" aria-label="${esc(t("nav.command"))}">${icon("search")}</button>
        ${user.role === "admin" ? `<button class="topbar-btn" data-theme title="${esc(t("settings.theme"))}" aria-label="${esc(t("settings.theme"))}">${icon("sun")}</button>` : ""}
        <span class="topbar-sep"></span>
        <button class="user-btn" data-user aria-haspopup="menu">
          <span class="avatar" data-avatar>${esc(initials(user.display_name || user.username))}</span>
          <span class="who"><strong data-username>${esc(user.display_name || user.username)}</strong><span data-role>${esc(user.role || "")}</span></span>
          ${icon("chevronDown")}
        </button>
      </div>
    </header>
    <nav class="dock" data-dock aria-label="MazimaOS"></nav>
    <main class="workspace">
      <div class="workspace-inner" data-dashboard></div>
      <div class="window-layer" data-windows></div>
    </main>
    <div class="offline-banner" data-offline>${icon("wifi")} ${esc(t("boot.offline"))}</div>
  </div>`;

  const dashRoot = root.querySelector("[data-dashboard]");
  const dock = root.querySelector("[data-dock]");
  const layer = root.querySelector("[data-windows]");

  const ctx = {
    user, settings, api, t, icons: { icon },
    status, version, desktopState, root,
    open: (id) => openApp(id),
    openSession: (appId) => openSession(appId),
    toast,
    confirm: confirmDialog,
    setLanguage,
    setActive: (id) => setActive(id),
    saveDesktop,
    applySettings,
    refreshSettings,
    setUser,
    logout,
  };
  const manager = new WindowManager(layer, ctx);
  ctx.windows = manager;

  function initials(name) {
    return String(name || "?").trim().split(/\s+/).slice(0, 2).map((p) => p[0] || "").join("").toUpperCase() || "?";
  }

  function dockItems() {
    const items = [
      { id: "dashboard", icon: "dashboard", label: t("nav.dashboard") },
      { id: "apps", icon: "apps", label: t("nav.apps") },
      { id: "session", icon: "terminal", label: t("nav.session") },
      { id: "system", icon: "system", label: t("nav.system") },
      { id: "docker", icon: "docker", label: t("nav.docker"), admin: true },
      { id: "remote", icon: "globe", label: t("nav.remote"), admin: true },
      { id: "updates", icon: "updates", label: t("nav.updates") },
      { id: "audit", icon: "audit", label: t("nav.audit"), admin: true },
      { id: "settings", icon: "settings", label: t("nav.settings") },
    ];
    return items.filter((i) => !i.admin || user.role === "admin");
  }

  function paintDock() {
    const items = dockItems();
    dock.innerHTML = `<div class="dock-group">${items.slice(0, 5).map(dockButton).join("")}</div><div class="dock-sep"></div><div class="dock-group">${items.slice(5).map(dockButton).join("")}</div>`;
    dock.querySelectorAll("[data-dock]").forEach((b) => b.addEventListener("click", () => onDock(b.dataset.dock)));
    markActive();
  }
  function dockButton(item) {
    return `<button class="dock-btn${active === item.id ? " active" : ""}" data-dock="${item.id}" data-label="${esc(item.label)}" title="${esc(item.label)}" aria-label="${esc(item.label)}">${icon(item.icon)}</button>`;
  }
  function onDock(id) {
    if (id === "dashboard") {
      manager.minimizeAll();
      setActive("dashboard");
      return;
    }
    if (id === "session") { openSession(); return; }
    openApp(id);
  }
  function markActive() {
    dock.querySelectorAll("[data-dock]").forEach((b) => {
      const target = b.dataset.dock;
      b.classList.toggle("active", target === active || (target === "session" && active === "session"));
    });
  }
  function setActive(id) {
    active = id || "dashboard";
    markActive();
  }

  function openApp(id, params = {}) {
    if (id === "dashboard") { onDock("dashboard"); return; }
    if (id === "session") { openSession(params.appId); return; }
    const def = APP_DEFS[id];
    if (!def) return;
    if (def.admin && user.role !== "admin") {
      toast("warn", t("common.permissionDenied"));
      return;
    }
    manager.open({
      appId: id,
      key: id,
      title: t(def.title),
      iconName: def.icon,
      width: def.width,
      height: def.height,
      pad: def.pad !== false,
      view: def.view,
      params,
    });
  }

  function openSession(appId = "") {
    const key = `session-${appId || "launcher"}`;
    manager.open({
      appId: "session",
      key,
      title: appId ? t("nav.session") : t("session.title"),
      iconName: "terminal",
      width: 1180,
      height: 760,
      pad: false,
      singleton: false,
      persist: false,
      view: renderSession,
      params: { appId },
    });
  }

  function saveDesktop() {
    clearTimeout(saveTimer);
    saveTimer = setTimeout(async () => {
      try {
        await api.put("/api/desktop", desktopState);
      } catch (err) {
        console.warn("desktop save failed", err);
      }
    }, 700);
  }

  async function refreshSettings() {
    try {
      const res = await api.get("/api/settings");
      await applySettings(res.settings || {});
      return res.settings;
    } catch (err) {
      toast("bad", t("toast.error"), err.message || String(err));
      return null;
    }
  }

  async function applySettings(next) {
    ctx.settings = { ...(next || {}) };
    const languageChanged = (ctx.settings.language || "it") !== lastLanguage;
    applyPreferences(ctx.settings, setLanguage);
    lastLanguage = ctx.settings.language || getLanguage();
    try { localStorage.setItem("mazima.language", lastLanguage); } catch {}
    updateUserChrome();
    paintDock();
    updateThemeButton();
    if (languageChanged) {
      document.title = `MazimaOS · ${ctx.settings.hostname || status?.hostname || "MazimaOS"}`;
      mountDashboard();
    }
  }

  function setUser(next) {
    if (!next) return;
    ctx.user = { ...ctx.user, ...next };
    user.display_name = ctx.user.display_name;
    user.role = ctx.user.role;
    user.totp_enabled = ctx.user.totp_enabled;
    updateUserChrome();
    paintDock();
  }

  function updateUserChrome() {
    const name = ctx.user.display_name || ctx.user.username;
    const avatar = root.querySelector("[data-avatar]");
    const username = root.querySelector("[data-username]");
    const role = root.querySelector("[data-role]");
    if (avatar) avatar.textContent = initials(name);
    if (username) username.textContent = name;
    if (role) role.textContent = ctx.user.role || "";
  }

  function updateThemeButton() {
    const btn = root.querySelector("[data-theme]");
    if (!btn) return;
    const theme = document.documentElement.dataset.theme || "dark";
    btn.innerHTML = icon(theme === "light" ? "moon" : "sun");
    btn.title = t("settings.theme");
    btn.setAttribute("aria-label", t("settings.theme"));
  }

  function mountDashboard() {
    dashboardCleanup?.();
    dashRoot.innerHTML = "";
    dashboardCleanup = renderDashboard(dashRoot, ctx) || null;
  }

  function updateClock() {
    const now = new Date();
    const time = root.querySelector("[data-clock]");
    const date = root.querySelector("[data-date]");
    if (time) time.textContent = new Intl.DateTimeFormat(locale(), { hour: "2-digit", minute: "2-digit" }).format(now);
    if (date) date.textContent = new Intl.DateTimeFormat(locale(), { weekday: "short", day: "2-digit", month: "short" }).format(now);
  }

  function paintMetrics() {
    const mem = snap?.memory || {};
    const disk = (snap?.disks || []).find((d) => d.mountpoint === "/") || (snap?.disks || [])[0] || {};
    const values = {
      cpu: { pct: snap?.cpu_percent || 0, label: `${Math.round(snap?.cpu_percent || 0)}%` },
      memory: { pct: mem.used_percent || 0, label: `${Math.round(mem.used_percent || 0)}%` },
      disk: { pct: disk.used_percent || 0, label: `${Math.round(disk.used_percent || 0)}%` },
    };
    for (const [key, v] of Object.entries(values)) {
      const el = root.querySelector(`[data-metric="${key}"]`);
      if (!el) continue;
      el.querySelector("b").textContent = v.label;
      const bar = el.querySelector(".bar i");
      if (bar) bar.style.width = `${Math.min(100, Math.max(0, v.pct))}%`;
      el.classList.toggle("warn", v.pct >= 80 && v.pct < 92);
      el.classList.toggle("bad", v.pct >= 92);
    }
  }

  async function loadMetrics() {
    try {
      snap = await api.get("/api/system");
      paintMetrics();
    } catch {}
  }

  async function logout() {
    if (loggingOut) return;
    const ok = await confirmDialog({
      title: t("common.logout"),
      message: t("settings.logoutAllBody"),
      confirmLabel: t("common.logout"),
      danger: true,
      iconName: "logout",
    });
    if (!ok) return;
    loggingOut = true;
    try { await api.post("/api/auth/logout", {}); } catch {}
    try { localStorage.removeItem("mazima.language"); } catch {}
    location.replace("/");
  }

  function openUserMenu() {
    closeMenu();
    const btn = root.querySelector("[data-user]");
    if (!btn) return;
    const rect = btn.getBoundingClientRect();
    menu = document.createElement("div");
    menu.className = "popmenu";
    menu.style.position = "fixed";
    menu.style.top = `${rect.bottom + 6}px`;
    menu.style.right = `${Math.max(8, window.innerWidth - rect.right)}px`;
    menu.innerHTML = `
      <div class="pop-head"><span class="avatar">${esc(initials(ctx.user.display_name || ctx.user.username))}</span><div><strong>${esc(ctx.user.display_name || ctx.user.username)}</strong><span>${esc(ctx.user.username)} · ${esc(ctx.user.role || "")}</span></div></div>
      <div class="sep"></div>
      <button data-menu="account">${icon("user")}${esc(t("settings.account"))}</button>
      <button data-menu="appearance">${icon("palette")}${esc(t("settings.appearance"))}</button>
      ${ctx.user.role === "admin" ? `<button data-menu="audit">${icon("audit")}${esc(t("nav.audit"))}</button>` : ""}
      <div class="sep"></div>
      <button data-menu="logout">${icon("logout")}${esc(t("common.logout"))}</button>`;
    document.body.appendChild(menu);
    menu.querySelector('[data-menu="account"]')?.addEventListener("click", () => { closeMenu(); openApp("settings"); });
    menu.querySelector('[data-menu="appearance"]')?.addEventListener("click", () => { closeMenu(); openApp("settings"); });
    menu.querySelector('[data-menu="audit"]')?.addEventListener("click", () => { closeMenu(); openApp("audit"); });
    menu.querySelector('[data-menu="logout"]')?.addEventListener("click", () => { closeMenu(); logout(); });
    setTimeout(() => {
      window.addEventListener("click", onDocClick, { once: true, capture: true });
      window.addEventListener("keydown", onMenuKey, { once: true });
    }, 0);
  }
  function closeMenu() {
    menu?.remove();
    menu = null;
  }
  function onDocClick(e) {
    if (menu && !menu.contains(e.target) && !e.target.closest("[data-user]")) closeMenu();
  }
  function onMenuKey(e) {
    if (e.key === "Escape") closeMenu();
  }

  function commandItems() {
    const items = [
      { id: "dashboard", icon: "dashboard", label: t("nav.dashboard"), run: () => onDock("dashboard") },
      { id: "apps", icon: "apps", label: t("nav.apps"), run: () => openApp("apps") },
      { id: "session", icon: "terminal", label: t("session.title"), run: () => openSession() },
      { id: "terminal", icon: "terminal", label: t("nav.terminal"), run: () => openSession("terminal") },
      { id: "browser", icon: "browser", label: t("nav.browser"), run: () => openSession("browser") },
      { id: "system", icon: "system", label: t("nav.system"), run: () => openApp("system") },
      { id: "updates", icon: "updates", label: t("nav.updates"), run: () => openApp("updates") },
      { id: "settings", icon: "settings", label: t("nav.settings"), run: () => openApp("settings") },
      { id: "theme", icon: "sun", label: t("settings.theme"), run: () => toggleTheme() },
      { id: "logout", icon: "logout", label: t("common.logout"), run: () => logout() },
    ];
    if (user.role === "admin") {
      items.splice(4, 0,
        { id: "docker", icon: "docker", label: t("nav.docker"), run: () => openApp("docker") },
        { id: "remote", icon: "globe", label: t("nav.remote"), run: () => openApp("remote") },
        { id: "audit", icon: "audit", label: t("nav.audit"), run: () => openApp("audit") },
      );
    }
    return items;
  }

  function openPalette() {
    if (palette) return;
    const items = commandItems();
    let filtered = items;
    let selected = 0;
    const overlay = document.createElement("div");
    overlay.className = "cmdk";
    overlay.innerHTML = `<div class="cmdk-panel" role="dialog" aria-modal="true" aria-label="${esc(t("nav.command"))}">
      <div class="cmdk-input">${icon("search")}<input type="search" placeholder="${esc(t("common.search"))}…" autocomplete="off" spellcheck="false"><span class="kbd">ESC</span></div>
      <div class="cmdk-list"></div>
    </div>`;
    document.body.appendChild(overlay);
    palette = overlay;
    const input = overlay.querySelector("input");
    const list = overlay.querySelector(".cmdk-list");
    const paint = () => {
      list.innerHTML = filtered.length ? filtered.map((item, i) => `<button class="cmdk-item${i === selected ? " sel" : ""}" data-i="${i}">${icon(item.icon)}<span>${esc(item.label)}</span>${item.id === "dashboard" ? `<span class="k">⌘K</span>` : ""}</button>`).join("") : `<div class="cmdk-empty">${esc(t("common.noResults"))}</div>`;
      list.querySelectorAll("[data-i]").forEach((b) => b.addEventListener("click", () => run(Number(b.dataset.i))));
      list.querySelector(".sel")?.scrollIntoView({ block: "nearest" });
    };
    const run = (i) => {
      const item = filtered[i];
      closePalette();
      item?.run();
    };
    const close = () => closePalette();
    const onInput = () => {
      const q = input.value.trim().toLowerCase();
      filtered = q ? items.filter((item) => `${item.label} ${item.id}`.toLowerCase().includes(q)) : items;
      selected = 0;
      paint();
    };
    const onKey = (e) => {
      if (e.key === "Escape") { e.preventDefault(); close(); }
      else if (e.key === "ArrowDown") { e.preventDefault(); selected = Math.min(filtered.length - 1, selected + 1); paint(); }
      else if (e.key === "ArrowUp") { e.preventDefault(); selected = Math.max(0, selected - 1); paint(); }
      else if (e.key === "Enter") { e.preventDefault(); run(selected); }
    };
    const onBackdrop = (e) => { if (e.target === overlay) close(); };
    input.addEventListener("input", onInput);
    input.addEventListener("keydown", onKey);
    overlay.addEventListener("mousedown", onBackdrop);
    paint();
    setTimeout(() => input.focus(), 20);
    palette._cleanup = () => {
      input.removeEventListener("input", onInput);
      input.removeEventListener("keydown", onKey);
      overlay.removeEventListener("mousedown", onBackdrop);
    };
  }
  function closePalette() {
    if (!palette) return;
    palette._cleanup?.();
    palette.remove();
    palette = null;
  }

  async function toggleTheme() {
    if (ctx.user.role !== "admin") { toast("warn", t("common.permissionDenied")); return; }
    const current = document.documentElement.dataset.theme === "light" ? "light" : "dark";
    const next = current === "light" ? "dark" : "light";
    try {
      const res = await api.patch("/api/settings", { theme: next });
      await applySettings(res.settings || { ...ctx.settings, theme: next });
      toast("ok", t("settings.saved"));
    } catch (err) { toast("bad", t("toast.error"), err.message || String(err)); }
  }

  function restoreWindows() {
    if (window.matchMedia("(max-width: 760px)").matches) return;
    const saved = [...desktopState.windows].sort((a, b) => (a.z || 0) - (b.z || 0)).slice(-8);
    manager.restoring = true;
    try {
      for (const w of saved) {
        const def = APP_DEFS[w.app_id];
        if (!def) continue;
        if (def.admin && user.role !== "admin") continue;
        const handle = manager.open({
          appId: w.app_id, key: w.app_id, title: t(def.title), iconName: def.icon,
          width: w.width, height: w.height, pad: def.pad !== false, view: def.view,
        });
        if (w.minimized) manager.minimize(handle);
      }
    } finally {
      manager.restoring = false;
    }
    manager.persist();
  }

  paintDock();
  mountDashboard();
  updateClock();
  updateThemeButton();
  loadMetrics();
  clockTimer = setInterval(updateClock, 15000);
  metricsTimer = setInterval(() => { if (!document.hidden) loadMetrics(); }, 12000);
  restoreWindows();
  setActive(active);

  root.querySelector("[data-command]")?.addEventListener("click", openPalette);
  root.querySelector("[data-theme]")?.addEventListener("click", toggleTheme);
  root.querySelector("[data-user]")?.addEventListener("click", openUserMenu);
  root.querySelectorAll("[data-metric]").forEach((b) => b.addEventListener("click", () => openApp("system")));

  const onKey = (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k" && !e.defaultPrevented) {
      const tag = e.target?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return;
      e.preventDefault();
      openPalette();
    }
  };
  window.addEventListener("keydown", onKey);

  const offline = root.querySelector("[data-offline]");
  const updateOnline = () => {
    offline.classList.toggle("show", !navigator.onLine);
    if (navigator.onLine) loadMetrics();
  };
  window.addEventListener("online", updateOnline);
  window.addEventListener("offline", updateOnline);
  updateOnline();

  let authHandled = false;
  const onAuthRequired = async () => {
    if (authHandled) return;
    authHandled = true;
    toast("warn", t("boot.offline"));
    await sleep(600);
    location.reload();
  };
  window.addEventListener("mazima:auth-required", onAuthRequired);

  ctx.cleanup = () => {
    dashboardCleanup?.();
    manager.closeAll();
    clearInterval(metricsTimer);
    clearInterval(clockTimer);
    clearTimeout(saveTimer);
    closePalette();
    closeMenu();
    window.removeEventListener("keydown", onKey);
    window.removeEventListener("online", updateOnline);
    window.removeEventListener("offline", updateOnline);
    window.removeEventListener("mazima:auth-required", onAuthRequired);
  };
  return ctx;
}
