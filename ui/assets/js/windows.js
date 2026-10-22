import { icon } from "./icons.js";
import { esc } from "./dom.js";
import { t } from "./i18n.js";

const MOBILE = () => window.matchMedia("(max-width: 760px)").matches;

export class WindowManager {
  constructor(layer, ctx) {
    this.layer = layer;
    this.ctx = ctx;
    this.windows = new Map();
    this.z = 20;
    this.offset = 0;
    this.active = null;
    this.restoring = false;
    layer.addEventListener("mousedown", () => this.ctx.setActive?.(this.active));
  }
  open(spec) {
    const key = spec.key || spec.appId || spec.id;
    if (spec.singleton !== false) {
      const existing = [...this.windows.values()].find((w) => w.appId === spec.appId && !spec.sessionId);
      if (existing) { this.focus(existing); return existing; }
    }
    const id = key + (spec.unique ? "-" + Math.random().toString(36).slice(2, 7) : "");
    if (this.windows.has(id)) { this.focus(this.windows.get(id)); return this.windows.get(id); }
    const layerRect = this.layer.getBoundingClientRect();
    const saved = !MOBILE() ? this.ctx.desktopState?.windows?.find((w) => w.app_id === spec.appId && !w.session_id) : null;
    const width = Math.min(spec.width || saved?.width || 860, Math.max(360, layerRect.width - 28));
    const height = Math.min(spec.height || saved?.height || 600, Math.max(300, layerRect.height - 28));
    const fallbackX = 34 + (this.offset % 5) * 28;
    const fallbackY = 26 + (this.offset % 5) * 24;
    this.offset++;
    let x = saved?.x ?? spec.x ?? fallbackX;
    let y = saved?.y ?? spec.y ?? fallbackY;
    x = Math.max(0, Math.min(x, layerRect.width - Math.min(width, 240)));
    y = Math.max(0, Math.min(y, layerRect.height - Math.min(height, 120)));
    const el = document.createElement("section");
    el.className = "window";
    el.id = `win-${id}`;
    el.dataset.app = spec.appId;
    el.dataset.key = id;
    el.style.left = `${x}px`;
    el.style.top = `${y}px`;
    el.style.width = `${width}px`;
    el.style.height = `${height}px`;
    el.style.zIndex = String(++this.z);
    el.innerHTML = `<header class="win-head">
      <div class="win-title">${icon(spec.iconName || "window")}<span>${esc(spec.title || "")}</span><span class="win-sub"></span></div>
      <div class="win-ctrl">
        <button data-min aria-label="${esc(t("common.minimize") || "Minimize")}">${icon("minimize")}</button>
        <button data-max aria-label="${esc(t("common.maximize") || "Maximize")}">${icon("maximize")}</button>
        <button class="close" data-close aria-label="${esc(t("common.close"))}">${icon("x")}</button>
      </div>
    </header>
    <div class="win-body${spec.pad === false ? "" : " pad"}"></div>
    <div class="win-resize" aria-hidden="true"></div>`;
    this.layer.appendChild(el);
    const body = el.querySelector(".win-body");
    const sub = el.querySelector(".win-sub");
    if (spec.subtitle) sub.textContent = spec.subtitle;
    const handle = {
      id, el, body, appId: spec.appId, spec,
      maxBtn: el.querySelector("[data-max]"),
      setSub: (text) => { sub.textContent = text || ""; },
      setTitle: (text) => { el.querySelector(".win-title span").textContent = text || ""; },
      setIcon: (name) => { const ic = el.querySelector(".win-title .icon"); if (ic) ic.outerHTML = icon(name); },
      close: () => this.close(id),
      focus: () => this.focus(handle),
      minimized: false,
    };
    this.windows.set(id, handle);
    if (saved?.maximized && !MOBILE()) el.classList.add("maximized");
    if (saved?.minimized) { handle.minimized = true; el.classList.add("minimized"); }
    el.querySelector("[data-close]").addEventListener("click", (e) => { e.stopPropagation(); this.close(id); });
    el.querySelector("[data-min]").addEventListener("click", (e) => { e.stopPropagation(); this.minimize(handle); });
    el.querySelector("[data-max]").addEventListener("click", (e) => { e.stopPropagation(); this.toggleMax(handle); });
    el.querySelector(".win-head").addEventListener("dblclick", (e) => {
      if (e.target.closest("button")) return;
      this.toggleMax(handle);
    });
    el.addEventListener("mousedown", () => this.focus(handle));
    this.bindDrag(handle);
    this.bindResize(handle);
    this.focus(handle);
    const view = spec.view?.default || spec.view?.mount ? spec.view : null;
    try {
      const result = (view?.mount || view?.default || spec.view)?.(body, this.ctx, handle);
      if (typeof result === "function") handle.cleanup = result;
      else if (result?.cleanup) handle.cleanup = result.cleanup;
      if (result?.onClose) handle.onClose = result.onClose;
    } catch (err) {
      console.error("window view failed", spec.appId, err);
      body.innerHTML = `<div class="empty">${icon("alert", "icon")}<strong>${esc(t("boot.failed"))}</strong><p>${esc(err.message || String(err))}</p></div>`;
    }
    if (!this.restoring) this.persist();
    return handle;
  }
  focus(handle) {
    if (!handle || !this.windows.has(handle.id)) return;
    for (const w of this.windows.values()) w.el.classList.remove("focused");
    handle.el.classList.remove("minimized");
    handle.minimized = false;
    handle.el.classList.add("focused");
    handle.el.style.zIndex = String(++this.z);
    this.active = handle.appId;
    this.ctx.setActive?.(handle.appId);
    if (typeof handle.onFocus === "function") handle.onFocus();
  }
  minimize(handle) {
    handle.minimized = true;
    handle.el.classList.add("minimized");
    if (this.active === handle.appId) {
      const next = [...this.windows.values()].filter((w) => !w.minimized && w !== handle).pop();
      if (next) this.focus(next);
      else { this.active = null; this.ctx.setActive?.(null); }
    }
    this.persist();
  }
  minimizeAll() {
    for (const w of this.windows.values()) { w.minimized = true; w.el.classList.add("minimized"); }
    this.active = null;
    this.ctx.setActive?.(null);
    this.persist();
  }
  toggleMax(handle) {
    const el = handle.el;
    if (MOBILE()) return;
    if (el.classList.contains("maximized")) {
      el.classList.remove("maximized");
      if (handle.prevRect) {
        Object.assign(el.style, handle.prevRect);
        handle.prevRect = null;
      }
      handle.maxBtn.innerHTML = icon("maximize");
    } else {
      handle.prevRect = { left: el.style.left, top: el.style.top, width: el.style.width, height: el.style.height };
      el.classList.add("maximized");
      handle.maxBtn.innerHTML = icon("restore");
    }
    this.focus(handle);
    this.persist();
  }
  close(id) {
    const handle = this.windows.get(id);
    if (!handle) return;
    if (handle.onClose) {
      try {
        const res = handle.onClose();
        if (res === false) return;
      } catch (err) { console.warn(err); }
    }
    try { handle.cleanup?.(); } catch (err) { console.warn(err); }
    handle.el.remove();
    this.windows.delete(id);
    if (this.active === handle.appId) {
      const next = [...this.windows.values()].filter((w) => !w.minimized).pop();
      if (next) this.focus(next);
      else { this.active = null; this.ctx.setActive?.(null); }
    }
    this.persist();
  }
  bindDrag(handle) {
    const head = handle.el.querySelector(".win-head");
    let start = null;
    head.addEventListener("pointerdown", (e) => {
      if (e.target.closest("button") || MOBILE()) return;
      if (handle.el.classList.contains("maximized")) return;
      start = { x: e.clientX, y: e.clientY, left: parseFloat(handle.el.style.left) || 0, top: parseFloat(handle.el.style.top) || 0 };
      head.setPointerCapture(e.pointerId);
      e.preventDefault();
    });
    head.addEventListener("pointermove", (e) => {
      if (!start) return;
      const rect = this.layer.getBoundingClientRect();
      const w = handle.el.offsetWidth, h = handle.el.offsetHeight;
      let left = start.left + e.clientX - start.x;
      let top = start.top + e.clientY - start.y;
      left = Math.max(-w + 80, Math.min(left, rect.width - 80));
      top = Math.max(0, Math.min(top, rect.height - 42));
      handle.el.style.left = `${left}px`;
      handle.el.style.top = `${top}px`;
    });
    const end = (e) => {
      if (!start) return;
      start = null;
      try { head.releasePointerCapture(e.pointerId); } catch {}
      this.persist();
    };
    head.addEventListener("pointerup", end);
    head.addEventListener("pointercancel", end);
  }
  bindResize(handle) {
    const grip = handle.el.querySelector(".win-resize");
    let start = null;
    grip.addEventListener("pointerdown", (e) => {
      if (MOBILE()) return;
      start = { x: e.clientX, y: e.clientY, w: handle.el.offsetWidth, h: handle.el.offsetHeight };
      grip.setPointerCapture(e.pointerId);
      e.preventDefault(); e.stopPropagation();
    });
    grip.addEventListener("pointermove", (e) => {
      if (!start) return;
      const rect = this.layer.getBoundingClientRect();
      const left = parseFloat(handle.el.style.left) || 0;
      const top = parseFloat(handle.el.style.top) || 0;
      const w = Math.max(340, Math.min(start.w + e.clientX - start.x, rect.width - left));
      const h = Math.max(240, Math.min(start.h + e.clientY - start.y, rect.height - top));
      handle.el.style.width = `${w}px`;
      handle.el.style.height = `${h}px`;
    });
    const end = (e) => {
      if (!start) return;
      start = null;
      try { grip.releasePointerCapture(e.pointerId); } catch {}
      this.persist();
    };
    grip.addEventListener("pointerup", end);
    grip.addEventListener("pointercancel", end);
  }
  persist() {
    if (this.restoring || !this.ctx.desktopState) return;
    const wins = [];
    for (const w of this.windows.values()) {
      if (w.spec.sessionId) continue;
      if (w.spec.persist === false) continue;
      const r = w.el.getBoundingClientRect();
      const layerRect = this.layer.getBoundingClientRect();
      wins.push({
        id: w.id,
        app_id: w.appId,
        title: w.spec.title || "",
        x: w.el.classList.contains("maximized") ? (w.prevRect ? parseFloat(w.prevRect.left) : 12) : r.left - layerRect.left,
        y: w.el.classList.contains("maximized") ? (w.prevRect ? parseFloat(w.prevRect.top) : 12) : r.top - layerRect.top,
        width: w.el.classList.contains("maximized") ? (w.prevRect ? parseFloat(w.prevRect.width) : layerRect.width - 24) : r.width,
        height: w.el.classList.contains("maximized") ? (w.prevRect ? parseFloat(w.prevRect.height) : layerRect.height - 24) : r.height,
        minimized: w.minimized,
        maximized: w.el.classList.contains("maximized"),
        desktop: 0,
        z: parseInt(w.el.style.zIndex) || 0,
      });
    }
    this.ctx.desktopState.windows = wins;
    this.ctx.saveDesktop?.();
  }
  closeAll() {
    for (const w of [...this.windows.values()]) this.close(w.id);
  }
  hasOpen() { return [...this.windows.values()].some((w) => !w.minimized); }
}
