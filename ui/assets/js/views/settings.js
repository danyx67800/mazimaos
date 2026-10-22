import { api } from "../api.js";
import { applyPreferences, copyText, esc, modal, setBusy, toast, typedConfirm } from "../dom.js";
import { errorText, setLanguage, t } from "../i18n.js";
import { icon } from "../icons.js";

const ACCENTS = ["#4f8cff", "#7c5cff", "#10b981", "#f97316", "#ec4899", "#06b6d4", "#eab308", "#ef4444"];
const WALLS = ["aurora", "dusk", "ocean", "graphite"];

export function renderSettings(root, ctx) {
  const admin = ctx.user?.role === "admin";
  let draft = { ...(ctx.settings || {}) };
  let user = { ...(ctx.user || {}) };
  let active = "appearance";
  let dirty = false;

  const sections = [
    ["appearance", t("settings.appearance"), "palette"],
    ["account", t("settings.account"), "user"],
    ["security", t("settings.security"), "shield"],
    ["network", t("settings.network"), "network"],
    ["about", t("settings.about"), "info"],
  ];
  root.innerHTML = `<div class="settings-layout"><nav class="settings-nav">${sections.map(([id, label, ic]) => `<button data-sec="${id}" class="${id === active ? "on" : ""}">${icon(ic)}${esc(label)}</button>`).join("")}</nav><div class="settings-main" data-main></div></div>`;
  const main = root.querySelector("[data-main]");
  const markDirty = () => { dirty = true; root.querySelector("[data-savebar]")?.classList.add("show"); };
  const clearDirty = () => { dirty = false; root.querySelector("[data-savebar]")?.classList.remove("show"); };
  const nav = (id) => {
    active = id;
    root.querySelectorAll("[data-sec]").forEach((b) => b.classList.toggle("on", b.dataset.sec === id));
    paint();
  };
  root.querySelectorAll("[data-sec]").forEach((b) => b.addEventListener("click", () => nav(b.dataset.sec)));

  const savebar = (label = t("common.save"), handler) => `<div class="settings-savebar" data-savebar><span class="dirty">${icon("info")}${esc(t("settings.unsaved"))}</span><button class="btn" data-reset>${esc(t("common.reset"))}</button><button class="btn primary" data-save>${esc(label)}</button></div>`;
  const bindSavebar = (handler, reset) => {
    root.querySelector("[data-save]")?.addEventListener("click", handler);
    root.querySelector("[data-reset]")?.addEventListener("click", reset);
  };

  const paint = () => {
    if (active === "appearance") paintAppearance();
    if (active === "account") paintAccount();
    if (active === "security") paintSecurity();
    if (active === "network") paintNetwork();
    if (active === "about") paintAbout();
    if (dirty) root.querySelector("[data-savebar]")?.classList.add("show");
  };

  const paintAppearance = () => {
    main.innerHTML = `<section class="settings-section on">
      <div><h2>${esc(t("settings.appearance"))}</h2><p class="type-note">${esc(t("settings.subtitle"))}</p></div>
      ${!admin ? `<div class="notice warn">${icon("shield")}<div>${esc(t("settings.adminOnlyHint"))}</div></div>` : ""}
      <div class="panel"><div class="panel-body">
        <div class="setting-row"><div class="txt"><strong>${esc(t("settings.theme"))}</strong></div><div class="control"><div class="seg" data-theme>
          ${[["auto", t("settings.themeAuto")], ["dark", t("settings.themeDark")], ["light", t("settings.themeLight")], ["amoled", t("settings.themeAmoled")]].map(([v, l]) => `<button data-v="${v}" class="${draft.theme === v ? "on" : ""}" ${!admin ? "disabled" : ""}>${esc(l)}</button>`).join("")}
        </div></div></div>
        <div class="setting-row"><div class="txt"><strong>${esc(t("settings.accent"))}</strong></div><div class="control"><div class="swatches" data-accent>${ACCENTS.map((c) => `<button class="swatch ${draft.accent === c ? "on" : ""}" style="background:${c}" data-v="${c}" aria-label="${c}" ${!admin ? "disabled" : ""}></button>`).join("")}</div></div></div>
        <div class="setting-row"><div class="txt"><strong>${esc(t("settings.wallpaper"))}</strong></div><div class="control"><div class="wall-grid" data-wall>${WALLS.map((w) => `<button class="wall ${w} ${draft.wallpaper === w ? "on" : ""}" data-v="${w}" aria-label="${w}" ${!admin ? "disabled" : ""}></button>`).join("")}</div></div></div>
        <div class="setting-row"><div class="txt"><strong>${esc(t("settings.transparency"))}</strong><span>${esc(draft.transparency)}%</span></div><div class="control"><input type="range" min="0" max="100" value="${Number(draft.transparency) || 0}" data-range="transparency" ${!admin ? "disabled" : ""}></div></div>
        <div class="setting-row"><div class="txt"><strong>${esc(t("settings.scale"))}</strong><span>${esc(draft.ui_scale)}%</span></div><div class="control"><input type="range" min="80" max="150" step="5" value="${Number(draft.ui_scale) || 100}" data-range="ui_scale" ${!admin ? "disabled" : ""}></div></div>
        <div class="setting-row"><div class="txt"><strong>${esc(t("settings.reduceMotion"))}</strong><span>${esc(t("settings.reduceMotionHint"))}</span></div><div class="control"><label class="switch"><input type="checkbox" data-check="reduce_motion" ${draft.reduce_motion ? "checked" : ""} ${!admin ? "disabled" : ""}><i></i></label></div></div>
      </div></div>
      ${admin ? savebar() : ""}
    </section>`;
    main.querySelectorAll("[data-theme] button").forEach((b) => b.addEventListener("click", () => {
      draft.theme = b.dataset.v;
      main.querySelectorAll("[data-theme] button").forEach((x) => x.classList.toggle("on", x === b));
      markDirty();
    }));
    main.querySelectorAll("[data-accent] .swatch").forEach((b) => b.addEventListener("click", () => {
      draft.accent = b.dataset.v;
      main.querySelectorAll("[data-accent] .swatch").forEach((x) => x.classList.toggle("on", x === b));
      markDirty();
    }));
    main.querySelectorAll("[data-wall] .wall").forEach((b) => b.addEventListener("click", () => {
      draft.wallpaper = b.dataset.v;
      main.querySelectorAll("[data-wall] .wall").forEach((x) => x.classList.toggle("on", x === b));
      markDirty();
    }));
    main.querySelectorAll("[data-range]").forEach((r) => r.addEventListener("input", () => {
      draft[r.dataset.range] = Number(r.value);
      const note = r.closest(".setting-row").querySelector(".txt span");
      if (note) note.textContent = `${r.value}%`;
      markDirty();
    }));
    main.querySelectorAll("[data-check]").forEach((c) => c.addEventListener("change", () => {
      draft[c.dataset.check] = c.checked;
      markDirty();
    }));
    if (admin) bindSavebar(saveAppearance, () => { draft = { ...ctx.settings }; dirty = false; paintAppearance(); });
  };

  const paintAccount = () => {
    main.innerHTML = `<section class="settings-section on">
      <div><h2>${esc(t("settings.account"))}</h2><p class="type-note">${esc(t("settings.profile"))}</p></div>
      <div class="panel"><div class="panel-head"><h3>${esc(t("settings.profile"))}</h3></div><div class="panel-body">
        <div class="form-grid">
          <div class="field"><label>${esc(t("common.username"))}</label><input class="input" value="${esc(user.username || "")}" disabled></div>
          <div class="field"><label>${esc(t("settings.role"))}</label><input class="input" value="${esc(user.role === "admin" ? t("common.admin") : t("common.user"))}" disabled></div>
        </div>
        <div class="field" style="margin-top:12px"><label for="st-name">${esc(t("settings.displayName"))}</label><input class="input" id="st-name" maxlength="80" value="${esc(user.display_name || "")}"></div>
        <div class="field"><label for="st-lang">${esc(t("settings.language"))}</label><select class="select" id="st-lang"><option value="it"${user.language !== "en" ? " selected" : ""}>Italiano</option><option value="en"${user.language === "en" ? " selected" : ""}>English</option></select><div class="hint">${esc(t("settings.languageHint"))}</div></div>
        <div class="form-error" role="alert" data-profile-error></div>
        <div class="row-actions" style="margin-top:12px"><button class="btn primary" data-save-profile>${icon("check")}${esc(t("common.save"))}</button><button class="btn ghost" data-logout>${icon("logout")}${esc(t("common.logout"))}</button></div>
      </div></div>
      <div class="panel"><div class="panel-head"><h3>${esc(t("settings.changePassword"))}</h3></div><div class="panel-body">
        <div class="field"><label for="pw-old">${esc(t("settings.currentPassword"))}</label><input class="input" type="password" id="pw-old" autocomplete="current-password"></div>
        <div class="form-grid"><div class="field"><label for="pw-new">${esc(t("settings.newPassword"))}</label><input class="input" type="password" id="pw-new" minlength="12" autocomplete="new-password"></div><div class="field"><label for="pw-rep">${esc(t("settings.confirmPassword"))}</label><input class="input" type="password" id="pw-rep" minlength="12" autocomplete="new-password"></div></div>
        <div class="form-error" role="alert" data-pw-error></div>
        <div style="margin-top:12px"><button class="btn" data-change-pw>${icon("key")}${esc(t("settings.changePassword"))}</button></div>
      </div></div>
    </section>`;
    main.querySelector("[data-save-profile]").addEventListener("click", async (e) => {
      const err = main.querySelector("[data-profile-error]");
      err.classList.remove("show");
      const display = main.querySelector("#st-name").value.trim();
      const language = main.querySelector("#st-lang").value;
      if (!display) { err.textContent = t("common.required"); err.classList.add("show"); return; }
      setBusy(e.currentTarget, true, t("common.saving"));
      const previousLang = user.language || ctx.settings?.language;
      try {
        const res = await api.patch("/api/me", { display_name: display, language });
        user = res.user || user;
        if (ctx.setUser) ctx.setUser(user);
        toast("ok", t("settings.profileSaved"));
        if (language !== previousLang) {
          setLanguage(language);
          if (ctx.applySettings) await ctx.applySettings({ ...(ctx.settings || {}), language });
        }
      } catch (ex) { err.textContent = errorText(ex); err.classList.add("show"); }
      setBusy(e.currentTarget, false);
    });
    main.querySelector("[data-change-pw]").addEventListener("click", async (e) => {
      const err = main.querySelector("[data-pw-error]");
      const oldPw = main.querySelector("#pw-old").value;
      const newPw = main.querySelector("#pw-new").value;
      const rep = main.querySelector("#pw-rep").value;
      err.classList.remove("show");
      if (!oldPw || newPw.length < 12) { err.textContent = t("setup.weak"); err.classList.add("show"); return; }
      if (newPw !== rep) { err.textContent = t("setup.mismatch"); err.classList.add("show"); return; }
      setBusy(e.currentTarget, true, t("common.saving"));
      try {
        await api.post("/api/me/password", { old_password: oldPw, new_password: newPw });
        toast("ok", t("settings.passwordChanged"));
        setTimeout(() => location.reload(), 900);
      } catch (ex) {
        setBusy(e.currentTarget, false);
        err.textContent = errorText(ex); err.classList.add("show");
      }
    });
    main.querySelector("[data-logout]").addEventListener("click", () => ctx.logout?.());
  };

  const paintSecurity = () => {
    main.innerHTML = `<section class="settings-section on">
      <div><h2>${esc(t("settings.security"))}</h2><p class="type-note">${esc(t("settings.twoFactorBody"))}</p></div>
      <div class="panel"><div class="panel-head"><h3>${esc(t("settings.twoFactor"))}</h3><span class="badge ${user.totp_enabled ? "ok" : "warn"}">${esc(user.totp_enabled ? t("settings.twoFactorOn") : t("settings.twoFactorOff"))}</span></div><div class="panel-body">
        <p class="type-note">${esc(t("settings.twoFactorBody"))}</p>
        <div class="row-actions" style="margin-top:14px">${user.totp_enabled ? `<button class="btn danger" data-disable2fa>${icon("shield")}${esc(t("settings.disable2fa"))}</button>` : `<button class="btn primary" data-enable2fa>${icon("shield")}${esc(t("settings.enable2fa"))}</button>`}</div>
      </div></div>
      <div class="panel"><div class="panel-head"><h3>${esc(t("settings.logoutAll"))}</h3></div><div class="panel-body"><p class="type-note">${esc(t("settings.logoutAllBody"))}</p><button class="btn ghost" style="margin-top:12px" data-logout>${icon("logout")}${esc(t("common.logout"))}</button></div></div>
      <div class="notice info">${icon("info")}<div>${esc(t("settings.auditHint"))} <button class="btn ghost sm" data-audit>${esc(t("settings.openAudit"))}</button></div></div>
    </section>`;
    main.querySelector("[data-audit]")?.addEventListener("click", () => ctx.open("audit"));
    main.querySelector("[data-logout]")?.addEventListener("click", () => ctx.logout?.());
    main.querySelector("[data-enable2fa]")?.addEventListener("click", enable2FA);
    main.querySelector("[data-disable2fa]")?.addEventListener("click", disable2FA);
  };

  const enable2FA = () => {
    const m = modal({
      title: t("settings.enable2fa"),
      body: `<div class="field"><label for="tf-pass">${esc(t("settings.currentPassword"))}</label><input class="input" type="password" id="tf-pass" autocomplete="current-password"></div><div class="form-error" role="alert"></div>`,
      actions: [
        { label: t("common.cancel"), onClick: (close) => close() },
        { label: t("common.next"), kind: "primary", onClick: async (close, btn) => {
          const err = m.body.querySelector(".form-error");
          const pass = m.body.querySelector("#tf-pass").value;
          if (!pass) { err.textContent = t("settings.passwordRequired"); err.classList.add("show"); return; }
          setBusy(btn, true, t("common.loading"));
          try {
            const res = await api.post("/api/me/totp/start", { password: pass });
            close();
            showTOTP(res);
          } catch (ex) { setBusy(btn, false); err.textContent = errorText(ex); err.classList.add("show"); }
        } },
      ],
    });
  };
  const showTOTP = (res) => {
    const m = modal({
      title: t("settings.enable2fa"),
      wide: true,
      body: `<div class="qrbox"><img src="${res.qr}" alt="QR"><div><p class="type-note">${esc(t("settings.scanQr"))}</p><div class="field"><label>${esc(t("settings.secret"))}</label><div class="secret">${esc(res.secret)}</div></div><div class="field"><label>${esc(t("settings.recoveryCodes"))}</label><div class="recovery">${(res.recovery_codes || []).map((c) => `<span>${esc(c)}</span>`).join("")}</div><div class="hint">${esc(t("settings.recoveryHint"))}</div></div><div class="row-actions"><button class="btn ghost sm" data-copy-recovery>${icon("copy")}${esc(t("common.copy"))}</button></div></div></div>
      <div class="field" style="margin-top:12px"><label for="tf-code">${esc(t("settings.code"))}</label><input class="input mono" id="tf-code" inputmode="numeric" autocomplete="one-time-code" maxlength="6"></div><div class="form-error" role="alert"></div>`,
      actions: [
        { label: t("common.cancel"), onClick: (close) => close() },
        { label: t("settings.confirm2fa"), kind: "primary", onClick: async (close, btn) => {
          const code = m.body.querySelector("#tf-code").value.trim();
          const err = m.body.querySelector(".form-error");
          if (code.length < 6) { err.textContent = t("settings.code"); err.classList.add("show"); return; }
          setBusy(btn, true, t("common.saving"));
          try {
            await api.post("/api/me/totp/confirm", { code });
            user.totp_enabled = true;
            if (ctx.setUser) ctx.setUser(user);
            close();
            toast("ok", t("settings.2faEnabled"));
            paintSecurity();
          } catch (ex) { setBusy(btn, false); err.textContent = errorText(ex); err.classList.add("show"); }
        } },
      ],
    });
    m.body.querySelector("[data-copy-recovery]").addEventListener("click", () => copyText((res.recovery_codes || []).join("\n")));
  };
  const disable2FA = async () => {
    try {
      const data = await typedConfirm({ title: t("settings.disable2fa"), word: "DISABLE", password: true, code: true, confirmLabel: t("settings.disable2fa") });
      await api.post("/api/me/totp/disable", { password: data.password, code: data.code });
      user.totp_enabled = false;
      if (ctx.setUser) ctx.setUser(user);
      toast("ok", t("settings.2faDisabled"));
      paintSecurity();
    } catch (err) { if (err?.message !== "cancelled") toast("bad", t("toast.error"), errorText(err)); }
  };

  const paintNetwork = () => {
    if (!admin) {
      main.innerHTML = `<section class="settings-section on"><div><h2>${esc(t("settings.network"))}</h2></div><div class="notice warn">${icon("shield")}<div>${esc(t("settings.adminOnlyHint"))}</div></div></section>`;
      return;
    }
    main.innerHTML = `<section class="settings-section on">
      <div><h2>${esc(t("settings.network"))}</h2><p class="type-note">${esc(t("settings.subtitle"))}</p></div>
      <div class="panel"><div class="panel-body">
        <div class="field"><label for="st-host">${esc(t("settings.hostname"))}</label><input class="input" id="st-host" value="${esc(draft.hostname || "")}" pattern="[a-z0-9][a-z0-9-]{0,62}"><div class="hint">${esc(t("settings.hostnameHint"))}</div></div>
        <div class="field"><label for="st-tz">${esc(t("settings.timezone"))}</label><input class="input" id="st-tz" value="${esc(draft.timezone || "")}"></div>
        <div class="form-error" role="alert"></div>
        <div class="row-actions" style="margin-top:12px"><button class="btn primary" data-save-net>${icon("check")}${esc(t("common.save"))}</button><button class="btn ghost" data-remote>${icon("globe")}${esc(t("settings.network"))}</button></div>
      </div></div>
    </section>`;
    main.querySelector("[data-remote]").addEventListener("click", () => ctx.open("remote"));
    main.querySelector("[data-save-net]").addEventListener("click", async (e) => {
      const err = main.querySelector(".form-error");
      const hostname = main.querySelector("#st-host").value.trim().toLowerCase();
      const timezone = main.querySelector("#st-tz").value.trim();
      if (!/^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/.test(hostname)) { err.textContent = t("error.invalid_hostname"); err.classList.add("show"); return; }
      setBusy(e.currentTarget, true, t("common.saving"));
      try {
        const res = await api.patch("/api/settings", { hostname, timezone });
        ctx.settings = res.settings;
        if (ctx.applySettings) await ctx.applySettings(res.settings);
        toast("ok", t("settings.saved"));
      } catch (ex) { err.textContent = errorText(ex); err.classList.add("show"); }
      setBusy(e.currentTarget, false);
    });
  };

  const paintAbout = () => {
    main.innerHTML = `<section class="settings-section on">
      <div><h2>${esc(t("settings.about"))}</h2></div>
      <div class="panel"><div class="panel-head"><h3>${esc(t("settings.aboutProduct"))}</h3><span class="chip accent">${icon("shield")}MazimaOS</span></div><div class="panel-body"><dl class="kv">
        <dt>${esc(t("settings.aboutVersion"))}</dt><dd>${esc(ctx.version || "0.1.0-dev")}</dd>
        <dt>${esc(t("settings.aboutCommit"))}</dt><dd>—</dd>
        <dt>${esc(t("settings.aboutBuild"))}</dt><dd>—</dd>
        <dt>${esc(t("settings.aboutGo"))}</dt><dd>Go daemon</dd>
        <dt>${esc(t("settings.hostname"))}</dt><dd>${esc(draft.hostname || "—")}</dd>
      </dl></div></div>
      <div class="panel"><div class="panel-head"><h3>${esc(t("nav.updates"))}</h3></div><div class="panel-body"><p class="type-note">${esc(t("updates.subtitle"))}</p><div class="row-actions" style="margin-top:12px"><button class="btn" data-updates>${icon("updates")}${esc(t("nav.updates"))}</button><button class="btn ghost" data-audit>${icon("audit")}${esc(t("nav.audit"))}</button></div></div></div>
      <div class="type-note">${esc(t("app.tagline"))} · ${esc(t("setup.foot"))}</div>
    </section>`;
    main.querySelector("[data-updates]")?.addEventListener("click", () => ctx.open("updates"));
    main.querySelector("[data-audit]")?.addEventListener("click", () => ctx.open("audit"));
  };

  const saveAppearance = async (e) => {
    const btn = e.currentTarget;
    setBusy(btn, true, t("common.saving"));
    const payload = {
      theme: draft.theme,
      accent: draft.accent,
      wallpaper: draft.wallpaper,
      transparency: Number(draft.transparency),
      ui_scale: Number(draft.ui_scale),
      reduce_motion: !!draft.reduce_motion,
    };
    try {
      const res = await api.patch("/api/settings", payload);
      ctx.settings = res.settings;
      if (ctx.applySettings) await ctx.applySettings(res.settings);
      else applyPreferences(res.settings, setLanguage);
      clearDirty();
      toast("ok", t("settings.appearanceSaved"));
    } catch (ex) { toast("bad", t("toast.error"), errorText(ex)); }
    setBusy(btn, false);
  };

  paint();
  return () => {};
}
