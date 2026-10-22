import { api } from "../api.js";
import { bytes, dateTime, esc, setBusy, toast, typedConfirm } from "../dom.js";
import { errorText, t } from "../i18n.js";
import { icon } from "../icons.js";

export function renderUpdates(root, ctx) {
  const admin = ctx.user?.role === "admin";
  let state = null;
  let update = null;
  let available = null;
  let checked = false;
  let checking = false;

  root.innerHTML = `<div class="view-stack"><div class="section-head"><div><h2>${esc(t("updates.title"))}</h2><div class="type-note">${esc(t("updates.subtitle"))}</div></div><div class="row-actions">${admin ? `<button class="btn primary" data-check>${icon("updates")}${esc(t("updates.check"))}</button>` : ""}</div></div><div data-body><div class="skeleton" style="height:320px;border-radius:14px"></div></div></div>`;
  const body = root.querySelector("[data-body]");
  root.querySelector("[data-check]")?.addEventListener("click", (e) => check(e));

  const paint = () => {
    if (!state) return;
    const configured = state.configured !== false;
    const notes = update ? (update.manifest?.notes_i18n?.[ctx.settings?.language] || update.manifest?.notes_i18n?.en || update.manifest?.notes || update.notes || "") : "";
    const st = state.state || {};
    body.innerHTML = `<div class="view-stack">
      ${!configured ? `<div class="notice warn">${icon("alert")}<div><strong>${esc(t("updates.notConfigured"))}</strong><p>${esc(t("updates.notConfiguredBody"))}</p></div></div>` : ""}
      <div class="dash-bottom">
        <div class="panel"><div class="panel-head"><h3>${esc(t("updates.current"))}</h3><span class="chip accent">${esc(state.current || "—")}</span></div><div class="panel-body"><dl class="kv">
          <dt>${esc(t("updates.channel"))}</dt><dd>${esc(state.channel || "—")}</dd>
          <dt>${esc(t("updates.lastCheck"))}</dt><dd>${esc(st.last_checked ? dateTime(st.last_checked) : t("common.never"))}</dd>
          <dt>${esc(t("updates.state"))}</dt><dd>${esc(st.last_result || "—")}</dd>
          <dt>${esc(t("updates.rollback"))}</dt><dd>${esc(st.previous || "—")}</dd>
          <dt>${esc(t("common.status"))}</dt><dd>${st.boot_attempts ? `${st.boot_attempts} boot attempts` : "—"}</dd>
        </dl></div></div>
        <div class="panel"><div class="panel-head"><h3>${esc(t("updates.channel"))}</h3></div><div class="panel-body">
          <div class="field"><label>${esc(t("updates.channel"))}</label><select class="select" data-channel ${!admin ? "disabled" : ""}>${[["stable", t("updates.stable")], ["beta", t("updates.beta")], ["nightly", t("updates.nightly")]].map(([v, l]) => `<option value="${v}"${(state.channel || ctx.settings?.update_channel) === v ? " selected" : ""}>${esc(l)}</option>`).join("")}</select><div class="hint">${esc(t("updates.channelHint"))}</div></div>
          <div class="setting-row"><div class="txt"><strong>${esc(t("updates.auto"))}</strong><span>${esc(t("updates.autoHint"))}</span></div><div class="control"><label class="switch"><input type="checkbox" data-auto ${(ctx.settings?.auto_update ?? true) ? "checked" : ""} ${!admin ? "disabled" : ""}><i></i></label></div></div>
          ${admin ? `<div class="row-actions" style="margin-top:12px"><button class="btn" data-save-config>${icon("check")}${esc(t("common.save"))}</button></div>` : ""}
        </div></div>
      </div>
      ${checked || update ? `<div class="panel"><div class="panel-head"><h3>${update ? esc(t("updates.available")) : esc(t("updates.upToDate"))}</h3><span class="chip ${update ? "warn" : "ok"}">${icon(update ? "download" : "check")}${esc(update ? update.manifest?.version : state.current)}</span></div><div class="panel-body">
        ${update ? `<p class="type-note">${esc(t("updates.availableBody", { version: update.manifest?.version || "" }))}</p><div class="tiles" style="padding:12px 0"><div class="tile"><div class="k">${esc(t("common.version"))}</div><div class="v">${esc(update.manifest?.version || "—")}</div></div><div class="tile"><div class="k">${esc(t("common.size"))}</div><div class="v">${esc(bytes(update.size || update.manifest?.bundle?.size))}</div></div><div class="tile"><div class="k">${esc(t("common.created"))}</div><div class="v" style="font-size:13px">${esc(dateTime(update.manifest?.released_at))}</div></div></div>
          ${notes ? `<div class="field"><label>${esc(t("updates.releaseNotes"))}</label><pre class="logbox" style="max-height:180px">${esc(notes)}</pre></div>` : ""}
          <div class="row-actions" style="margin-top:12px">${admin ? `<button class="btn primary" data-install>${icon("download")}${esc(t("updates.install"))}</button>` : ""}</div>
        ` : `<p class="type-note">${esc(t("updates.upToDate"))}</p>`}
      </div></div>` : ""}
      ${st.last_error ? `<div class="notice bad">${icon("alert")}<div><strong>${esc(t("common.error"))}</strong><p>${esc(st.last_error)}</p></div></div>` : ""}
      ${st.update_log?.length ? `<div class="panel"><div class="panel-head"><h3>${esc(t("updates.state"))}</h3></div><div class="panel-body"><pre class="logbox">${esc(st.update_log.join("\n"))}</pre></div></div>` : ""}
      ${st.previous && admin ? `<div class="panel"><div class="panel-head"><h3>${esc(t("updates.rollback"))}</h3></div><div class="panel-body"><p class="type-note">${esc(t("updates.rollbackBody"))}</p><button class="btn danger" style="margin-top:12px" data-rollback>${icon("restore")}${esc(t("updates.rollback"))}</button></div></div>` : ""}
    </div>`;
    body.querySelector("[data-install]")?.addEventListener("click", install);
    body.querySelector("[data-rollback]")?.addEventListener("click", rollback);
    body.querySelector("[data-save-config]")?.addEventListener("click", saveConfig);
  };
  const load = async () => {
    try {
      state = await api.get("/api/updates");
      paint();
    } catch (err) {
      body.innerHTML = `<div class="panel"><div class="empty">${icon("alert", "icon")}<strong>${esc(t("toast.error"))}</strong><p>${esc(errorText(err))}</p><button class="btn" data-retry>${icon("refresh")}${esc(t("common.retry"))}</button></div></div>`;
      body.querySelector("[data-retry]")?.addEventListener("click", load);
    }
  };
  const check = async (e) => {
    if (checking) return;
    checking = true;
    const btn = e?.currentTarget || root.querySelector("[data-check]");
    setBusy(btn, true, t("updates.checking"));
    try {
      const res = await api.post("/api/updates/check", {});
      available = res;
      update = res.available ? res.update : null;
      checked = true;
      await load();
      toast(update ? "info" : "ok", update ? t("updates.available") : t("updates.upToDate"));
    } catch (err) { toast("bad", t("toast.error"), errorText(err)); }
    setBusy(btn, false);
    checking = false;
  };
  const install = async () => {
    if (!update) return;
    try {
      const data = await typedConfirm({ title: t("updates.installTitle", { version: update.manifest?.version || "" }), message: t("updates.installBody"), word: "INSTALL", password: true, confirmLabel: t("updates.install") });
      const res = await api.post("/api/updates/install", { password: data.password, confirm: "INSTALL", version: update.manifest?.version || "" });
      toast("ok", t("updates.installDone"), res.restart_required ? t("updates.installDoneBody") : "");
      update = null;
      setTimeout(() => load(), 3000);
    } catch (err) { if (err?.message !== "cancelled") toast("bad", t("toast.error"), errorText(err)); }
  };
  const rollback = async () => {
    try {
      const data = await typedConfirm({ title: t("updates.rollbackTitle"), message: t("updates.rollbackBody"), word: "ROLLBACK", password: true, confirmLabel: t("updates.rollback") });
      await api.post("/api/updates/rollback", { password: data.password, confirm: "ROLLBACK" });
      toast("warn", t("updates.rollbackDone"));
      setTimeout(() => load(), 3000);
    } catch (err) { if (err?.message !== "cancelled") toast("bad", t("toast.error"), errorText(err)); }
  };
  const saveConfig = async (e) => {
    const btn = e.currentTarget;
    const channel = body.querySelector("[data-channel]").value;
    const auto = body.querySelector("[data-auto]").checked;
    setBusy(btn, true, t("common.saving"));
    try {
      const res = await api.patch("/api/settings", { update_channel: channel, auto_update: auto });
      ctx.settings = res.settings;
      if (ctx.applySettings) await ctx.applySettings(res.settings);
      toast("ok", t("settings.saved"));
      load();
    } catch (err) { toast("bad", t("toast.error"), errorText(err)); }
    setBusy(btn, false);
  };
  load();
  return () => {};
}
