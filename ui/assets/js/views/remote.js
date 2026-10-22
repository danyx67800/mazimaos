import { api } from "../api.js";
import { dateTime, esc, modal, setBusy, toast, typedConfirm } from "../dom.js";
import { errorText, t } from "../i18n.js";
import { icon } from "../icons.js";

export function renderRemote(root, ctx) {
  const admin = ctx.user?.role === "admin";
  let status = null;
  let alive = true;
  let timer = null;
  let busy = false;

  if (!admin) {
    root.innerHTML = `<div class="view-stack"><div class="section-head"><h2>${esc(t("remote.title"))}</h2></div><div class="panel"><div class="empty">${icon("shield", "icon")}<strong>${esc(t("common.adminOnly"))}</strong><p>${esc(t("common.permissionDenied"))}</p></div></div></div>`;
    return () => {};
  }
  root.innerHTML = `<div class="view-stack"><div class="section-head"><div><h2>${esc(t("remote.title"))}</h2><div class="type-note">${esc(t("remote.subtitle"))}</div></div><button class="btn ghost" data-refresh>${icon("refresh")}${esc(t("remote.refresh"))}</button></div><div data-body><div class="skeleton" style="height:300px;border-radius:14px"></div></div></div>`;
  const body = root.querySelector("[data-body]");
  const refresh = root.querySelector("[data-refresh]");
  refresh.addEventListener("click", () => load(true));

  const connected = () => status?.backend_state === "Running" && (status?.ips?.length || 0) > 0;
  const paint = () => {
    if (!status) return;
    if (!status.available && status.error) {
      body.innerHTML = `<div class="panel"><div class="empty">${icon("alert", "icon")}<strong>${esc(t("remote.notInstalled"))}</strong><p>${esc(status.error || t("remote.notInstalledBody"))}</p></div></div>`;
      return;
    }
    const on = connected();
    const peers = status.peers || [];
    body.innerHTML = `<div class="view-stack">
      <div class="remote-hero"><span class="dot ${on ? "" : "bad"}"></span>${icon("globe")}<div class="grow"><strong>${esc(on ? t("remote.connected") : status.backend_state === "NeedsLogin" ? t("remote.signedOut") : t("remote.signedOut"))}</strong><span>${esc(on ? (status.tailnet_name || status.magic_dns_suffix || "") : t("remote.signedOutBody"))}</span></div><span class="chip ${on ? "ok" : "warn"}">${esc(status.backend_state || "unknown")}</span></div>
      ${(status.health || []).length ? `<div class="notice warn">${icon("alert")}<div>${status.health.map((h) => `<div>${esc(h)}</div>`).join("")}</div></div>` : ""}
      <div class="panel"><div class="panel-head"><h3>${esc(t("remote.status"))}</h3><span class="chip">v${esc(status.version || "—")}</span></div><div class="panel-body"><dl class="kv">
        <dt>${esc(t("remote.hostname"))}</dt><dd>${esc(status.hostname || status.dns_name || "—")}</dd>
        <dt>${esc(t("remote.tailnet"))}</dt><dd>${esc(status.tailnet_name || status.magic_dns_suffix || "—")}</dd>
        <dt>${esc(t("remote.ip"))}</dt><dd class="mono">${esc((status.ips || []).join(", ") || "—")}</dd>
        <dt>${esc(t("remote.httpsRequired"))}</dt><dd>${status.https_enabled ? esc(t("common.enabled")) : esc(t("common.disabled"))}</dd>
        <dt>${esc(t("remote.peers"))}</dt><dd>${esc(status.online_peers || 0)} / ${esc(status.peer_count || 0)}</dd>
        <dt>${esc(t("remote.version"))}</dt><dd>${esc(status.version || "—")}</dd>
      </dl></div></div>
      <div class="dash-bottom">
        <div class="panel"><div class="panel-head"><h3>${esc(t("remote.login"))}</h3></div><div class="panel-body">
          ${on ? `<p class="type-note">${esc(t("remote.connected"))} · ${esc(status.dns_name || status.hostname || "")}</p><button class="btn danger" style="margin-top:12px" data-logout>${icon("logout")}${esc(t("remote.logout"))}</button>` : `<p class="type-note">${esc(t("remote.signedOutBody"))}</p><div class="row-actions" style="margin-top:12px"><button class="btn primary" data-login>${icon("external")}${esc(t("remote.loginInteractive"))}</button><button class="btn" data-authkey>${icon("key")}${esc(t("remote.loginKey"))}</button></div>${status.auth_url ? `<a class="btn ghost sm" style="margin-top:10px" href="${esc(status.auth_url)}" target="_blank" rel="noopener">${icon("external")}${esc(t("remote.openLogin"))}</a>` : ""}`}
        </div></div>
        <div class="panel"><div class="panel-head"><h3>${esc(t("remote.serve"))}</h3><span class="chip ${status.funnel ? "ok" : ""}">${esc(status.funnel ? t("remote.funnelBadge") : t("remote.tailnetBadge"))}</span></div><div class="panel-body">
          <p class="type-note">${esc(t("remote.serveBody"))}</p>
          <div class="row-actions" style="margin-top:12px">
            <button class="btn ${status.services?.length ? "ghost" : "primary"}" data-serve ${!on ? "disabled" : ""}>${icon("globe")}${esc(t("remote.enableServe"))}</button>
            <button class="btn ${status.funnel ? "ghost" : ""}" data-funnel ${!on ? "disabled" : ""}>${icon("wifi")}${esc(t("remote.enableFunnel"))}</button>
            ${status.services?.length || status.funnel ? `<button class="btn danger" data-disable>${icon("x")}${esc(t("remote.disableServe"))}</button>` : ""}
          </div>
          ${!ctx.user?.totp_enabled ? `<div class="type-note" style="margin-top:10px">${icon("shield")} ${esc(t("remote.totpRequired"))}</div>` : ""}
        </div></div>
      </div>
      <div class="panel"><div class="panel-head"><h3>${esc(t("remote.peers"))}</h3><span class="chip">${peers.length}</span></div><div class="table-wrap"><table class="tbl"><thead><tr><th>${esc(t("remote.hostname"))}</th><th>${esc(t("remote.ip"))}</th><th>OS</th><th>${esc(t("common.status"))}</th><th>Relay</th></tr></thead><tbody>${peers.length ? peers.map((p) => `<tr><td><b>${esc(p.hostname || p.dns_name || p.id)}</b>${p.tags?.length ? `<div class="type-note">${esc(p.tags.join(", "))}</div>` : ""}</td><td class="mono">${esc((p.ips || []).join(", ") || "—")}</td><td>${esc(p.os || "—")}</td><td><span class="badge ${p.online ? "ok" : "muted"}">${esc(p.online ? t("common.online") : t("common.offline"))}</span>${p.last_seen ? `<div class="type-note">${esc(dateTime(p.last_seen))}</div>` : ""}</td><td class="type-note">${esc(p.relay || (p.direct ? "direct" : "—"))}</td></tr>`).join("") : `<tr><td colspan="5"><div class="empty"><p>${esc(t("remote.noPeers"))}</p></div></td></tr>`}</tbody></table></div></div>
    </div>`;
    body.querySelector("[data-login]")?.addEventListener("click", () => loginInteractive());
    body.querySelector("[data-authkey]")?.addEventListener("click", () => authKey());
    body.querySelector("[data-logout]")?.addEventListener("click", () => logout());
    body.querySelector("[data-serve]")?.addEventListener("click", () => publish(false));
    body.querySelector("[data-funnel]")?.addEventListener("click", () => publish(true));
    body.querySelector("[data-disable]")?.addEventListener("click", () => disable());
  };
  const load = async (notify = false) => {
    try {
      const res = await api.get("/api/tailscale");
      status = res.status || null;
      paint();
      if (notify) toast("info", t("common.refresh"));
    } catch (err) {
      body.innerHTML = `<div class="panel"><div class="empty">${icon("alert", "icon")}<strong>${esc(t("remote.notInstalled"))}</strong><p>${esc(errorText(err))}</p><button class="btn" data-retry>${icon("refresh")}${esc(t("common.retry"))}</button></div></div>`;
      body.querySelector("[data-retry]")?.addEventListener("click", () => load());
    }
  };
  const runLogin = async (payload) => {
    busy = true;
    try {
      const res = await api.post("/api/tailscale/login", payload);
      status = res.status || status;
      paint();
      toast("ok", t("common.success"));
    } catch (err) { toast("bad", t("toast.error"), errorText(err)); }
    busy = false;
  };
  const loginInteractive = async () => {
    if (busy) return;
    await runLogin({});
  };
  const authKey = () => {
    const m = modal({
      title: t("remote.loginKey"),
      body: `<div class="field"><label for="ts-key">${esc(t("remote.authKey"))}</label><input class="input mono" id="ts-key" autocomplete="off" spellcheck="false"></div><div class="hint">${esc(t("remote.authKeyHint"))}</div><div class="form-error" role="alert"></div>`,
      actions: [
        { label: t("common.cancel"), onClick: (close) => close() },
        { label: t("remote.login"), kind: "primary", onClick: async (close, btn) => {
          const key = m.body.querySelector("#ts-key").value.trim();
          const err = m.body.querySelector(".form-error");
          if (!key) { err.textContent = t("common.required"); err.classList.add("show"); return; }
          setBusy(btn, true, t("common.loading"));
          try {
            const res = await api.post("/api/tailscale/login", { auth_key: key });
            status = res.status || status;
            close(); paint();
            toast("ok", t("common.success"));
          } catch (ex) { setBusy(btn, false); err.textContent = errorText(ex); err.classList.add("show"); }
        } },
      ],
    });
  };
  const logout = async () => {
    try {
      const data = await typedConfirm({ title: t("remote.logout"), message: t("remote.logoutBody"), word: "LOGOUT", password: true, confirmLabel: t("remote.logout") });
      await api.post("/api/tailscale/logout", { password: data.password, confirm: "LOGOUT" });
      toast("ok", t("remote.serveDisabled"));
      load();
    } catch (err) { if (err?.message !== "cancelled") toast("bad", t("toast.error"), errorText(err)); }
  };
  const publish = async (funnel) => {
    if (funnel && !ctx.user?.totp_enabled) { toast("warn", t("remote.totpRequired")); return; }
    const word = funnel ? "ENABLE FUNNEL" : "ENABLE SERVE";
    try {
      const data = await typedConfirm({
        title: funnel ? t("remote.enableFunnel") : t("remote.enableServe"),
        message: funnel ? t("remote.funnelWarning") : t("remote.serveBody"),
        word,
        password: true,
        code: funnel,
        confirmLabel: funnel ? t("remote.enableFunnel") : t("remote.enableServe"),
      });
      const res = await api.post("/api/tailscale/serve", { funnel, password: data.password, code: data.code, confirm: word });
      toast("ok", t("remote.serveEnabled"), res.funnel ? t("remote.funnelBadge") : t("remote.tailnetBadge"));
      load();
    } catch (err) { if (err?.message !== "cancelled") toast("bad", t("toast.error"), errorText(err)); }
  };
  const disable = async () => {
    try {
      const data = await typedConfirm({ title: t("remote.disableServe"), message: t("remote.serveBody"), word: "DISABLE", password: true, confirmLabel: t("remote.disableServe") });
      await api.post("/api/tailscale/serve/disable", { password: data.password, confirm: "DISABLE" });
      toast("ok", t("remote.serveDisabled"));
      load();
    } catch (err) { if (err?.message !== "cancelled") toast("bad", t("toast.error"), errorText(err)); }
  };
  load();
  timer = setInterval(() => { if (alive && !busy) load(); }, 15000);
  return () => { alive = false; clearInterval(timer); };
}
