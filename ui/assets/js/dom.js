import { icon } from "./icons.js";
import { t, locale } from "./i18n.js";

export const $ = (sel, root = document) => root.querySelector(sel);
export const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];
export function esc(value) {
  return String(value ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}
export function cls(v) { return String(v || "").replace(/[^a-z0-9_-]/gi, ""); }
export function bytes(n) {
  if (n === null || n === undefined || isNaN(n)) return "—";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  let v = Number(n), i = 0;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  const digits = v >= 100 || i === 0 ? 0 : v >= 10 ? 1 : 2;
  return `${new Intl.NumberFormat(locale(), { maximumFractionDigits: digits }).format(v)} ${units[i]}`;
}
export function num(n, digits = 0) {
  if (n === null || n === undefined || isNaN(n)) return "—";
  return new Intl.NumberFormat(locale(), { maximumFractionDigits: digits }).format(n);
}
export function pct(n, digits = 0) {
  if (n === null || n === undefined || isNaN(n)) return "—";
  return `${num(n, digits)}%`;
}
export function level(n) { return n >= 90 ? "bad" : n >= 75 ? "warn" : "ok"; }
export function duration(seconds) {
  const s = Math.max(0, Math.floor(seconds || 0));
  const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60);
  if (d) return `${d}g ${h}h`;
  if (h) return `${h}h ${m}m`;
  return `${m}m`;
}
export function dateTime(value) {
  if (!value) return "—";
  const d = new Date(value);
  if (isNaN(d)) return "—";
  return new Intl.DateTimeFormat(locale(), { dateStyle: "medium", timeStyle: "short" }).format(d);
}
export function timeOnly(value) {
  const d = value ? new Date(value) : new Date();
  return new Intl.DateTimeFormat(locale(), { hour: "2-digit", minute: "2-digit" }).format(d);
}
export function relative(value) {
  if (!value) return t("common.never");
  const d = new Date(value);
  if (isNaN(d)) return "—";
  const diff = Math.round((d.getTime() - Date.now()) / 1000);
  const abs = Math.abs(diff);
  const rtf = new Intl.RelativeTimeFormat(locale(), { numeric: "auto" });
  if (abs < 60) return rtf.format(Math.round(diff), "second");
  if (abs < 3600) return rtf.format(Math.round(diff / 60), "minute");
  if (abs < 86400) return rtf.format(Math.round(diff / 3600), "hour");
  return rtf.format(Math.round(diff / 86400), "day");
}
export function debounce(fn, wait = 250) {
  let id;
  return (...args) => { clearTimeout(id); id = setTimeout(() => fn(...args), wait); };
}
export function throttle(fn, wait = 100) {
  let last = 0, timer;
  return (...args) => {
    const now = Date.now();
    if (now - last >= wait) { last = now; fn(...args); }
    else { clearTimeout(timer); timer = setTimeout(() => { last = Date.now(); fn(...args); }, wait - (now - last)); }
  };
}
export async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
    toast("ok", t("toast.copied"));
    return true;
  } catch {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    let ok = false;
    try { ok = document.execCommand("copy"); } catch {}
    ta.remove();
    toast(ok ? "ok" : "bad", ok ? t("toast.copied") : t("toast.copyFailed"));
    return ok;
  }
}
export function toast(kind, title, message = "", timeout = 5200) {
  const root = $("#toast-root");
  if (!root) return;
  while (root.children.length >= 4) root.firstElementChild.remove();
  const names = { ok: "check", bad: "alert", warn: "alert", info: "info" };
  const el = document.createElement("div");
  el.className = `toast ${cls(kind)}`;
  el.innerHTML = `${icon(names[kind] || "info")}<div><strong>${esc(title)}</strong>${message ? `<p>${esc(message)}</p>` : ""}</div><button class="close" aria-label="${esc(t("common.close"))}">${icon("x", "icon sm")}</button>`;
  const close = () => { el.style.opacity = "0"; setTimeout(() => el.remove(), 160); };
  el.querySelector(".close").addEventListener("click", close);
  root.appendChild(el);
  if (timeout) setTimeout(close, timeout);
}
export function setBusy(button, busy, label) {
  if (!button) return;
  if (busy) {
    button.dataset.original = button.innerHTML;
    button.disabled = true;
    button.innerHTML = `<span class="spinner"></span>${label ? esc(label) : ""}`;
  } else {
    button.disabled = false;
    if (button.dataset.original) { button.innerHTML = button.dataset.original; delete button.dataset.original; }
  }
}
export function modal({ title, subtitle = "", body = "", wide = false, actions = [], onClose = null, closable = true }) {
  const root = $("#modal-root");
  const backdrop = document.createElement("div");
  backdrop.className = "modal-backdrop";
  backdrop.innerHTML = `<section class="modal${wide ? " wide" : ""}" role="dialog" aria-modal="true" aria-label="${esc(title)}">
    <header class="modal-head"><div><h3>${esc(title)}</h3>${subtitle ? `<div class="sub">${esc(subtitle)}</div>` : ""}</div>${closable ? `<button class="btn icon-only ghost" data-close aria-label="${esc(t("common.close"))}">${icon("x")}</button>` : ""}</header>
    <div class="modal-body"></div>
    <footer class="modal-foot"></footer>
  </section>`;
  const bodyEl = backdrop.querySelector(".modal-body");
  const foot = backdrop.querySelector(".modal-foot");
  if (typeof body === "string") bodyEl.innerHTML = body; else if (body) bodyEl.appendChild(body);
  let closed = false;
  const close = () => {
    if (closed) return;
    closed = true;
    document.removeEventListener("keydown", onKey);
    backdrop.remove();
    if (onClose) onClose();
  };
  for (const action of actions) {
    const b = document.createElement("button");
    b.className = `btn ${action.kind || ""}`;
    b.innerHTML = `${action.icon ? icon(action.icon) : ""}${esc(action.label)}`;
    b.addEventListener("click", () => action.onClick?.(close, b, backdrop));
    foot.appendChild(b);
  }
  if (!actions.length) {
    const b = document.createElement("button");
    b.className = "btn";
    b.textContent = t("common.close");
    b.addEventListener("click", close);
    foot.appendChild(b);
  }
  backdrop.querySelector("[data-close]")?.addEventListener("click", close);
  backdrop.addEventListener("mousedown", (e) => { if (e.target === backdrop && closable) close(); });
  const onKey = (e) => {
    if (e.key === "Escape" && closable) close();
    if (e.key === "Tab") {
      const focusables = [...backdrop.querySelectorAll("button,input,select,textarea,a[href],[tabindex]:not([tabindex='-1'])")].filter((el) => !el.disabled && el.offsetParent !== null);
      if (!focusables.length) return;
      const first = focusables[0], last = focusables[focusables.length - 1];
      if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
      else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
    }
  };
  document.addEventListener("keydown", onKey);
  root.appendChild(backdrop);
  setTimeout(() => bodyEl.querySelector("input,select,textarea,button")?.focus(), 30);
  return { close, backdrop, body: bodyEl, foot, modal: backdrop.querySelector(".modal") };
}
export function confirmDialog({ title, message, confirmLabel = t("common.confirm"), danger = false, iconName = "alert" }) {
  return new Promise((resolve) => {
    let done = false;
    const m = modal({
      title,
      body: `<div class="danger-note" style="margin-bottom:4px">${icon(iconName)}<div>${esc(message)}</div></div>`,
      actions: [
        { label: t("common.cancel"), onClick: (close) => { close(); resolve(false); } },
        { label: confirmLabel, kind: danger ? "danger" : "primary", onClick: (close) => { done = true; close(); resolve(true); } },
      ],
      onClose: () => { if (!done) resolve(false); },
    });
    m.body.querySelector("button")?.focus();
  });
}
export function typedConfirm({ title, message = "", word, password = true, code = false, passwordLabel = t("modal.password"), codeLabel = t("modal.code"), confirmLabel = t("common.confirm"), danger = true, extra = "" }) {
  return new Promise((resolve, reject) => {
    const m = modal({
      title,
      subtitle: message,
      body: `<form class="form-stack" data-sensitive>
        ${danger ? `<div class="danger-note">${icon("shield")}<div>${esc(t("modal.danger"))}</div></div>` : ""}
        ${password ? `<div class="field"><label for="sc-pass">${esc(passwordLabel)}</label><input class="input" id="sc-pass" type="password" autocomplete="current-password" required></div>` : ""}
        ${code ? `<div class="field"><label for="sc-code">${esc(codeLabel)}</label><input class="input" id="sc-code" inputmode="numeric" autocomplete="one-time-code" maxlength="32" required></div>` : ""}
        ${extra}
        <div class="field"><label for="sc-word">${esc(t("modal.typeWord", { word }))}</label><input class="input mono" id="sc-word" autocomplete="off" autocapitalize="characters" required></div>
        <div class="form-error" role="alert"></div>
      </form>`,
      actions: [
        { label: t("common.cancel"), onClick: (close) => { close(); reject(new Error("cancelled")); } },
        { label: confirmLabel, kind: danger ? "danger" : "primary", onClick: async (close, btn) => {
          const form = m.body.querySelector("[data-sensitive]");
          const err = form.querySelector(".form-error");
          const data = new FormData(form);
          if (form.querySelector("#sc-word")?.value.trim() !== word) {
            err.textContent = t("modal.typeWord", { word });
            err.classList.add("show");
            return;
          }
          if (password && !form.querySelector("#sc-pass").value) {
            err.textContent = t("settings.passwordRequired");
            err.classList.add("show");
            return;
          }
          if (code && !form.querySelector("#sc-code").value.trim()) {
            err.textContent = t("settings.code");
            err.classList.add("show");
            return;
          }
          close();
          resolve({
            password: password ? form.querySelector("#sc-pass").value : "",
            code: code ? form.querySelector("#sc-code").value.trim() : "",
            confirm: word,
            form,
            data,
          });
        } },
      ],
      onClose: () => reject(new Error("cancelled")),
    });
    m.body.querySelector("#sc-word")?.addEventListener("keydown", (e) => {
      if (e.key === "Enter") { e.preventDefault(); m.foot.querySelectorAll("button")[1]?.click(); }
    });
  });
}
export function emptyState(iconName, title, body, action = "") {
  return `<div class="empty">${icon(iconName, "icon")}<strong>${esc(title)}</strong><p>${esc(body)}</p>${action}</div>`;
}
export function errorState(err, retryLabel = t("common.retry")) {
  return `<div class="empty">${icon("alert", "icon")}<strong>${esc(t("toast.error"))}</strong><p>${esc(err?.message || String(err))}</p><button class="btn" data-retry>${icon("refresh")}${esc(retryLabel)}</button></div>`;
}
export function sparkline(canvas, history) {
  if (!canvas) return;
  const rect = canvas.getBoundingClientRect();
  const dpr = window.devicePixelRatio || 1;
  const w = Math.max(80, rect.width), h = Math.max(50, rect.height);
  canvas.width = w * dpr;
  canvas.height = h * dpr;
  const ctx = canvas.getContext("2d");
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, w, h);
  const points = (history || []).slice(-60);
  const styles = getComputedStyle(document.documentElement);
  const accent = styles.getPropertyValue("--accent").trim() || "#4f8cff";
  const accent2 = styles.getPropertyValue("--accent-2").trim() || "#7c5cff";
  ctx.strokeStyle = styles.getPropertyValue("--line").trim() || "rgba(150,170,210,.15)";
  ctx.lineWidth = 1;
  for (let i = 0; i <= 3; i++) {
    const y = (h / 3) * i;
    ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(w, y); ctx.stroke();
  }
  if (points.length < 2) return;
  const draw = (key, color, fill) => {
    const vals = points.map((p) => Number(p?.[key] || 0));
    const max = Math.max(10, ...vals) * 1.15;
    ctx.beginPath();
    vals.forEach((v, i) => {
      const x = (i / (vals.length - 1)) * w;
      const y = h - (v / max) * h;
      if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
    });
    ctx.strokeStyle = color;
    ctx.lineWidth = 2;
    ctx.lineJoin = "round";
    ctx.stroke();
    if (fill) {
      ctx.lineTo(w, h); ctx.lineTo(0, h); ctx.closePath();
      const g = ctx.createLinearGradient(0, 0, 0, h);
      g.addColorStop(0, color + "44"); g.addColorStop(1, color + "00");
      ctx.fillStyle = g; ctx.fill();
    }
  };
  draw("cpu_percent", accent, true);
  draw("memory", accent2, false);
}
export function historyPoint(snap) {
  return { cpu_percent: snap?.cpu_percent || 0, memory: snap?.memory?.used_percent || 0, at: snap?.time };
}
export function applyPreferences(settings, langSetter) {
  const s = settings || {};
  const root = document.documentElement;
  let theme = s.theme || "auto";
  if (theme === "auto") theme = matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
  root.dataset.theme = theme;
  root.dataset.wallpaper = s.wallpaper || "aurora";
  root.dataset.reduceMotion = s.reduce_motion ? "true" : "false";
  root.style.setProperty("--accent", s.accent || "#4f8cff");
  const alpha = Math.max(25, Math.min(98, 100 - Number(s.transparency ?? 32)));
  root.style.setProperty("--glass-alpha", (alpha / 100).toFixed(2));
  const scale = Math.max(80, Math.min(150, Number(s.ui_scale || 100))) / 100;
  root.style.setProperty("--ui-scale", String(scale));
  document.querySelector('meta[name="theme-color"]')?.setAttribute("content", theme === "light" ? "#eef2fa" : "#070b14");
  if (langSetter) langSetter(s.language || "it");
}
