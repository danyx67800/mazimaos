import { api, setCSRF } from "./api.js";
import { errorText, getLanguage, setLanguage, t } from "./i18n.js";
import { brandMark } from "./icons.js";
import { renderDesktop } from "./desktop.js";
import { renderLogin } from "./views/login.js";
import { renderSetup } from "./views/setup.js";

const root = document.getElementById("app");
const storedLanguage = (() => {
  try { return localStorage.getItem("mazima.language"); } catch { return ""; }
})();
setLanguage(storedLanguage || ((navigator.language || "it").toLowerCase().startsWith("it") ? "it" : "en"));

function loading() {
  root.innerHTML = `<div class="boot-screen">
    <div class="boot-brand">${brandMark()}</div>
    <span class="spinner"></span>
    <p>${t("boot.loading")}</p>
  </div>`;
}

function fatal(err, retry) {
  root.innerHTML = `<div class="boot-screen boot-error">
    <div class="boot-brand">${brandMark()}</div>
    <h1>${t("boot.failed")}</h1>
    <p>${t("boot.failedHint")}</p>
    <pre>${escapeText(errorText(err) || String(err?.message || err || ""))}</pre>
    ${navigator.onLine ? "" : `<div class="notice warn">${t("boot.offline")}</div>`}
    <button class="btn primary" type="button">${t("common.retry")}</button>
  </div>`;
  root.querySelector("button")?.addEventListener("click", retry);
  if (!navigator.onLine) {
    window.addEventListener("online", () => retry(), { once: true });
  }
}

function escapeText(value) {
  return String(value).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

async function boot() {
  loading();
  try {
    const status = await api.get("/api/setup/status");
    if (!status.setup_complete) {
      renderSetup(root, {}, status);
      return;
    }

    let session;
    try {
      session = await api.get("/api/auth/session", { noAuthRedirect: true });
    } catch (err) {
      if (err?.status === 401) {
        renderLogin(root);
        return;
      }
      throw err;
    }
    setCSRF(session.csrf);

    let settingsRes;
    let desktopRes;
    try {
      [settingsRes, desktopRes] = await Promise.all([
        api.get("/api/settings"),
        api.get("/api/desktop"),
      ]);
    } catch (err) {
      if (err?.status === 401) {
        setCSRF("");
        renderLogin(root);
        return;
      }
      throw err;
    }

    const settings = settingsRes?.settings || {};
    setLanguage(settings.language || getLanguage());
    renderDesktop(root, {
      user: session.user,
      settings,
      desktop: desktopRes?.desktop || { windows: [], virtual_desktops: 2, active_desktop: 0 },
      version: status.version,
      status,
    });

    if ("serviceWorker" in navigator && window.isSecureContext) {
      navigator.serviceWorker.register("/sw.js").catch((err) => console.warn("service worker", err));
    }
  } catch (err) {
    console.error("MazimaOS boot failed", err);
    fatal(err, boot);
  }
}

boot();
