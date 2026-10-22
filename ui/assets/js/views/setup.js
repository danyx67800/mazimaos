import { api, setCSRF } from "../api.js";
import { esc, setBusy, toast } from "../dom.js";
import { errorText, setLanguage, t } from "../i18n.js";
import { icon } from "../icons.js";
import { gateway, setupFeatures, tzOptions, passwordMeter } from "./gateway.js";

export function renderSetup(root, ctx = {}, status = {}) {
  const browserTz = Intl.DateTimeFormat().resolvedOptions().timeZone || "Europe/Rome";
  const browserLang = (navigator.language || "it").toLowerCase().startsWith("it") ? "it" : "en";
  setLanguage(browserLang);
  const state = {
    step: 0,
    hostname: "mazima",
    timezone: browserTz,
    language: browserLang,
    displayName: "",
    username: "",
    password: "",
    token: "",
  };
  const card = gateway(root, {
    kicker: t("setup.kicker"),
    title: `${esc(t("setup.title"))} <em>${esc(t("app.name"))}</em>`,
    subtitle: t("setup.subtitle"),
    features: setupFeatures(),
    foot: t("setup.foot"),
  });
  const render = () => {
    card.querySelectorAll(".auth-head, .auth-form, .auth-step, .auth-alt, .auth-help").forEach((n) => n.remove());
    card.insertAdjacentHTML("beforeend", `
      <div class="auth-step"><i class="${state.step >= 0 ? "on" : ""}"></i><i class="${state.step >= 1 ? "on" : ""}"></i></div>
      <div class="auth-head"><h2>${esc(state.step === 0 ? t("setup.step1") : t("setup.step2"))}</h2>
        <p>${esc(state.step === 0 ? t("setup.subtitle") : t("setup.secure"))}</p></div>
      ${state.step === 0 ? step1() : step2(status)}
    `);
    bind();
  };
  const step1 = () => `<form class="auth-form" data-step="1" novalidate>
    <div class="field"><label for="su-host">${esc(t("setup.hostname"))}</label>
      <input class="input" id="su-host" name="hostname" value="${esc(state.hostname)}" pattern="[a-z0-9][a-z0-9-]{0,62}" required autocomplete="off">
      <div class="hint">${esc(t("setup.hostnameHint"))}</div></div>
    <div class="form-grid">
      <div class="field"><label for="su-tz">${esc(t("setup.timezone"))}</label>
        <select class="select" id="su-tz" name="timezone">${tzOptions(state.timezone)}</select></div>
      <div class="field"><label for="su-lang">${esc(t("setup.language"))}</label>
        <select class="select" id="su-lang" name="language">
          <option value="it"${state.language === "it" ? " selected" : ""}>Italiano</option>
          <option value="en"${state.language === "en" ? " selected" : ""}>English</option>
        </select></div>
    </div>
    <div class="form-error" role="alert"></div>
    <div class="auth-actions"><button class="btn primary" type="submit">${esc(t("common.next"))}${icon("chevronRight")}</button></div>
  </form>`;
  const step2 = (status) => `<form class="auth-form" data-step="2" novalidate>
    <div class="form-grid">
      <div class="field"><label for="su-display">${esc(t("setup.displayName"))} <span class="muted">(${esc(t("common.optional"))})</span></label>
        <input class="input" id="su-display" name="display_name" value="${esc(state.displayName)}" maxlength="80" autocomplete="name"></div>
      <div class="field"><label for="su-user">${esc(t("setup.username"))}</label>
        <input class="input" id="su-user" name="username" value="${esc(state.username)}" pattern="[a-z0-9._-]{3,32}" required autocomplete="username" autocapitalize="none">
      </div>
    </div>
    <div class="field"><label for="su-pass">${esc(t("setup.password"))}</label>
      <div class="pw-wrap"><input class="input" id="su-pass" name="password" type="password" minlength="12" required autocomplete="new-password"><button class="pw-toggle" type="button" data-pw>${icon("eye")}</button></div>
      <div class="pw-meter"><i></i><i></i><i></i><i></i></div>
      <div class="hint">${esc(t("setup.passwordHint"))}</div></div>
    <div class="field"><label for="su-pass2">${esc(t("setup.passwordConfirm"))}</label>
      <input class="input" id="su-pass2" name="password2" type="password" minlength="12" required autocomplete="new-password"></div>
    <div class="field"><label for="su-token">${esc(t("setup.token"))}</label>
      <input class="input mono" id="su-token" name="setup_token" value="${esc(state.token)}" autocomplete="off" spellcheck="false">
      <div class="hint">${esc(t("setup.tokenHint", { path: "/var/lib/mazima/setup.token" }))}</div></div>
    <div class="form-error" role="alert"></div>
    <div class="auth-actions">
      <button class="btn primary" type="submit">${esc(t("setup.submit"))}</button>
      <button class="btn ghost" type="button" data-back>${icon("arrowLeft")}${esc(t("common.back"))}</button>
    </div>
  </form>`;
  const bind = () => {
    const form = card.querySelector("form");
    const err = form.querySelector(".form-error");
    const fail = (msg) => { err.textContent = msg; err.classList.add("show"); };
    form.addEventListener("submit", async (e) => {
      e.preventDefault();
      err.classList.remove("show");
      if (state.step === 0) {
        const host = form.hostname.value.trim().toLowerCase();
        if (!/^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/.test(host) || host.length > 63) return fail(t("error.invalid_hostname"));
        state.hostname = host;
        state.timezone = form.timezone.value;
        state.language = form.language.value;
        setLanguage(state.language);
        state.step = 1;
        render();
        return;
      }
      state.displayName = form.display_name.value.trim();
      state.username = form.username.value.trim().toLowerCase();
      state.password = form.password.value;
      state.token = form.setup_token.value.trim();
      if (!/^[a-z0-9._-]{3,32}$/.test(state.username)) return fail(t("setup.usernameHint"));
      if (state.password.length < 12) return fail(t("setup.weak"));
      if (state.password !== form.password2.value) return fail(t("setup.mismatch"));
      if (status.setup_token_required !== false && !state.token) return fail(t("setup.tokenRequired"));
      const btn = form.querySelector("button[type=submit]");
      setBusy(btn, true, t("setup.submitting"));
      try {
        const res = await api.post("/api/setup", {
          setup_token: state.token,
          username: state.username,
          display_name: state.displayName,
          password: state.password,
          language: state.language,
          timezone: state.timezone,
          hostname: state.hostname,
        }, { noAuthRedirect: true });
        setCSRF(res?.csrf || "");
        toast("ok", t("setup.success"), t("setup.successBody"));
        setTimeout(() => location.reload(), 700);
      } catch (ex) {
        setBusy(btn, false);
        fail(errorText(ex));
      }
    });
    form.querySelector("[data-back]")?.addEventListener("click", () => { state.step = 0; render(); });
    form.querySelector("[data-pw]")?.addEventListener("click", () => {
      const input = form.querySelector("#su-pass");
      const show = input.type === "password";
      input.type = show ? "text" : "password";
      form.querySelector("[data-pw]").innerHTML = icon(show ? "eyeOff" : "eye");
    });
    const pw = form.querySelector("#su-pass");
    const meter = form.querySelector(".pw-meter");
    if (pw && meter) passwordMeter(pw, meter);
  };
  render();
}
