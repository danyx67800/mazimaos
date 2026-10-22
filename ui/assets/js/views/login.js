import { api, setCSRF } from "../api.js";
import { esc, setBusy } from "../dom.js";
import { errorText, setLanguage, t } from "../i18n.js";
import { icon } from "../icons.js";
import { gateway, setupFeatures } from "./gateway.js";

export function renderLogin(root) {
  const lang = (navigator.language || "it").toLowerCase().startsWith("it") ? "it" : "en";
  setLanguage(lang);
  const card = gateway(root, {
    kicker: t("login.footer"),
    title: `${esc(t("login.title"))} <em>${esc(t("app.name"))}</em>`,
    subtitle: t("app.tagline"),
    features: setupFeatures(),
    foot: t("login.remember"),
  });
  let challenge = "";
  const render = () => {
    card.querySelectorAll(".auth-head, .auth-form, .auth-help").forEach((n) => n.remove());
    if (!challenge) {
      card.insertAdjacentHTML("beforeend", `
        <div class="auth-head"><h2>${esc(t("login.title"))}</h2><p>${esc(t("login.subtitle"))}</p></div>
        <form class="auth-form" novalidate>
          <div class="field"><label for="li-user">${esc(t("common.username"))}</label>
            <input class="input" id="li-user" name="username" autocomplete="username" autocapitalize="none" required autofocus></div>
          <div class="field"><label for="li-pass">${esc(t("common.password"))}</label>
            <div class="pw-wrap"><input class="input" id="li-pass" name="password" type="password" autocomplete="current-password" required><button class="pw-toggle" type="button" data-pw>${icon("eye")}</button></div></div>
          <div class="form-error" role="alert"></div>
          <div class="auth-actions"><button class="btn primary" type="submit">${esc(t("login.submit"))}</button></div>
        </form>`);
    } else {
      card.insertAdjacentHTML("beforeend", `
        <div class="auth-head"><h2>${esc(t("login.totpTitle"))}</h2><p>${esc(t("login.totpSubtitle"))}</p></div>
        <form class="auth-form" novalidate>
          <div class="field"><label for="li-code">${esc(t("login.code"))}</label>
            <input class="input mono" id="li-code" name="code" inputmode="text" autocomplete="one-time-code" maxlength="32" required autofocus></div>
          <div class="hint">${esc(t("login.recoveryHint"))}</div>
          <div class="form-error" role="alert"></div>
          <div class="auth-actions"><button class="btn primary" type="submit">${esc(t("login.verify"))}</button>
            <button class="btn ghost" type="button" data-back>${icon("arrowLeft")}${esc(t("login.backToPassword"))}</button></div>
        </form>`);
    }
    bind();
  };
  const bind = () => {
    const form = card.querySelector("form");
    const err = form.querySelector(".form-error");
    form.querySelector("[data-pw]")?.addEventListener("click", () => {
      const input = form.querySelector("#li-pass");
      const show = input.type === "password";
      input.type = show ? "text" : "password";
      form.querySelector("[data-pw]").innerHTML = icon(show ? "eyeOff" : "eye");
    });
    form.querySelector("[data-back]")?.addEventListener("click", () => { challenge = ""; render(); });
    form.addEventListener("submit", async (e) => {
      e.preventDefault();
      err.classList.remove("show");
      const btn = form.querySelector("button[type=submit]");
      setBusy(btn, true, challenge ? t("login.verify") : t("login.submitting"));
      try {
        if (!challenge) {
          const res = await api.post("/api/auth/login", {
            username: form.username.value.trim().toLowerCase(),
            password: form.password.value,
          }, { noAuthRedirect: true });
          if (res.status === "totp_required") {
            challenge = res.challenge;
            setBusy(btn, false);
            render();
            return;
          }
          setCSRF(res.csrf || "");
          location.reload();
        } else {
          const res = await api.post("/api/auth/2fa", { challenge, code: form.code.value.trim(), kind: "totp" }, { noAuthRedirect: true });
          setCSRF(res.csrf || "");
          location.reload();
        }
      } catch (ex) {
        setBusy(btn, false);
        const code = ex.code === "invalid_credentials" && ex.status === 429 ? "rate_limited" : ex.code;
        err.textContent = errorText({ ...ex, code });
        err.classList.add("show");
        form.querySelector("input")?.focus();
      }
    });
  };
  render();
}
