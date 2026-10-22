import { api } from "../api.js";
import { bytes, dateTime, esc, modal, num, pct, setBusy, toast, typedConfirm } from "../dom.js";
import { errorText, t } from "../i18n.js";
import { icon } from "../icons.js";

export function renderDocker(root, ctx) {
  const admin = ctx.user?.role === "admin";
  let active = "containers";
  let containers = [];
  let images = [];
  let volumes = [];
  let networks = [];
  let df = null;
  let showStopped = true;
  let alive = true;
  let timer = null;

  const tabs = [
    ["containers", t("docker.containers"), "docker"],
    ["images", t("docker.images"), "disk"],
    ["volumes", t("docker.volumes"), "folder"],
    ["networks", t("docker.networks"), "network"],
    ["storage", t("docker.df"), "server"],
  ];
  root.innerHTML = `<div class="view-stack">
    <div class="section-head"><div><h2>${esc(t("docker.title"))}</h2><div class="type-note">${esc(t("docker.subtitle"))}</div></div><div class="row-actions"><button class="btn ghost" data-refresh>${icon("refresh")}${esc(t("common.refresh"))}</button><button class="btn danger" data-prune>${icon("trash")}${esc(t("docker.prune"))}</button></div></div>
    <div class="win-tabs">${tabs.map(([id, label, ic]) => `<button class="win-tab${id === active ? " on" : ""}" data-tab="${id}">${icon(ic)}${esc(label)}</button>`).join("")}</div>
    <div data-panel></div>
  </div>`;
  const panel = root.querySelector("[data-panel]");
  const pruneBtn = root.querySelector("[data-prune]");
  if (!admin) {
    root.querySelector(".section-head").insertAdjacentHTML("afterend", `<div class="notice warn">${icon("shield")}<div><strong>${esc(t("common.adminOnly"))}</strong><p>${esc(t("common.permissionDenied"))}</p></div></div>`);
    pruneBtn.remove();
    panel.innerHTML = `<div class="panel"><div class="empty">${icon("shield", "icon")}<strong>${esc(t("common.adminOnly"))}</strong><p>${esc(t("common.permissionDenied"))}</p></div></div>`;
    return () => {};
  }
  const setTab = (id) => {
    active = id;
    root.querySelectorAll("[data-tab]").forEach((b) => b.classList.toggle("on", b.dataset.tab === id));
    paint();
    load();
  };
  root.querySelectorAll("[data-tab]").forEach((b) => b.addEventListener("click", () => setTab(b.dataset.tab)));
  root.querySelector("[data-refresh]").addEventListener("click", () => load());
  pruneBtn.addEventListener("click", () => prune());

  const notice = (err) => `<div class="panel"><div class="empty">${icon("alert", "icon")}<strong>${esc(t("docker.unavailable"))}</strong><p>${esc(errorText(err) || t("docker.unavailableBody"))}</p><button class="btn" data-retry>${icon("refresh")}${esc(t("common.retry"))}</button></div></div>`;
  const paint = () => {
    if (active === "containers") return paintContainers();
    if (active === "images") return paintImages();
    if (active === "volumes") return paintVolumes();
    if (active === "networks") return paintNetworks();
    return paintDF();
  };
  const bindRetry = () => panel.querySelector("[data-retry]")?.addEventListener("click", load);
  const paintContainers = () => {
    panel.innerHTML = `<div class="panel"><div class="toolbar"><label class="check"><input type="checkbox" data-all ${showStopped ? "checked" : ""}> ${esc(t("docker.all"))}</label><span class="count" data-c>${containers.length}</span><button class="btn ghost" data-refresh-c>${icon("refresh")}${esc(t("common.refresh"))}</button></div><div class="table-wrap"><table class="tbl"><thead><tr><th>${esc(t("docker.container"))}</th><th>${esc(t("docker.image"))}</th><th>${esc(t("docker.state"))}</th><th>${esc(t("docker.ports"))}</th><th></th></tr></thead><tbody>${containers.length ? containers.map((c) => {
      const name = (c.Names?.[0] || c.Id || "").replace(/^\//, "");
      const st = (c.State || "").toLowerCase();
      return `<tr><td><b>${esc(name)}</b>${c.AppID ? `<div class="type-note">app · ${esc(c.AppID)}</div>` : ""}<div class="type-note mono">${esc((c.Id || "").slice(0, 12))}</div></td><td class="mono">${esc(c.Image || "—")}</td><td><span class="badge ${st === "running" ? "ok" : st === "exited" ? "muted" : "warn"}">${esc(c.State || "—")}</span><div class="type-note">${esc(c.Status || "")}</div></td><td class="mono type-note">${esc((c.Ports || []).map((p) => `${p.PublicPort ? `${p.PublicPort}→` : ""}${p.PrivatePort}/${p.Type}`).join(", ") || "—")}</td><td class="num"><div class="row-actions">
        <button class="btn icon-only ghost" data-detail title="${esc(t("common.details"))}">${icon("info")}</button>
        ${st === "running" ? `<button class="btn icon-only ghost" data-act="stop" title="${esc(t("common.stop"))}">${icon("stop")}</button><button class="btn icon-only ghost" data-act="restart" title="${esc(t("common.restart"))}">${icon("restart")}</button>` : `<button class="btn icon-only ghost" data-act="start" title="${esc(t("common.start"))}">${icon("play")}</button>`}
        <button class="btn icon-only ghost" data-logs title="${esc(t("docker.logs"))}">${icon("file")}</button>
        <button class="btn icon-only ghost danger-text" data-act="remove" title="${esc(t("docker.remove"))}">${icon("trash")}</button>
      </div></td></tr>`;
    }).join("") : `<tr><td colspan="5"><div class="empty"><p>${esc(t("docker.noContainers"))}</p></div></td></tr>`}</tbody></table></div></div>`;
    panel.querySelector("[data-all]").addEventListener("change", (e) => { showStopped = e.target.checked; load(); });
    panel.querySelector("[data-refresh-c]").addEventListener("click", load);
    panel.querySelectorAll("tbody tr").forEach((tr, i) => {
      const c = containers[i];
      if (!c) return;
      tr.querySelector("[data-detail]")?.addEventListener("click", () => detail(c));
      tr.querySelector("[data-logs]")?.addEventListener("click", () => logs(c));
      tr.querySelectorAll("[data-act]").forEach((b) => b.addEventListener("click", () => action(c, b.dataset.act, b)));
    });
  };
  const paintImages = () => {
    panel.innerHTML = `<div class="panel"><div class="toolbar"><span class="count">${images.length}</span><button class="btn ghost" data-refresh-i>${icon("refresh")}${esc(t("common.refresh"))}</button></div><div class="table-wrap"><table class="tbl"><thead><tr><th>${esc(t("docker.image"))}</th><th>${esc(t("common.size"))}</th><th>${esc(t("common.created"))}</th><th></th></tr></thead><tbody>${images.length ? images.map((img) => {
      const tags = (img.RepoTags || []).filter((x) => x && !x.includes("<none>"));
      return `<tr><td><b>${esc(tags[0] || (img.Id || "").slice(0, 19))}</b>${tags.length > 1 ? `<div class="type-note">${esc(tags.slice(1).join(", "))}</div>` : ""}<div class="type-note mono">${esc((img.Id || "").replace("sha256:", "").slice(0, 12))}</div></td><td>${esc(bytes(img.Size))}</td><td>${esc(dateTime((img.Created || 0) * 1000))}</td><td class="num"><div class="row-actions"><button class="btn icon-only ghost" data-rm title="${esc(t("docker.remove"))}">${icon("trash")}</button></div></td></tr>`;
    }).join("") : `<tr><td colspan="4"><div class="empty"><p>${esc(t("docker.noImages"))}</p></div></td></tr>`}</tbody></table></div></div>`;
    panel.querySelector("[data-refresh-i]").addEventListener("click", load);
    panel.querySelectorAll("[data-rm]").forEach((b, i) => b.addEventListener("click", () => removeImage(images[i])));
  };
  const paintVolumes = () => {
    panel.innerHTML = `<div class="panel"><div class="toolbar"><span class="count">${volumes.length}</span><button class="btn ghost" data-refresh-v>${icon("refresh")}${esc(t("common.refresh"))}</button></div><div class="table-wrap"><table class="tbl"><thead><tr><th>${esc(t("common.name"))}</th><th>${esc(t("docker.driver"))}</th><th>${esc(t("docker.mountpoint"))}</th><th>${esc(t("docker.scope"))}</th></tr></thead><tbody>${volumes.length ? volumes.map((v) => `<tr><td><b>${esc(v.Name)}</b></td><td>${esc(v.Driver || "—")}</td><td class="mono type-note">${esc(v.Mountpoint || "—")}</td><td>${esc(v.Scope || "—")}</td></tr>`).join("") : `<tr><td colspan="4"><div class="empty"><p>${esc(t("docker.noVolumes"))}</p></div></td></tr>`}</tbody></table></div></div>`;
    panel.querySelector("[data-refresh-v]").addEventListener("click", load);
  };
  const paintNetworks = () => {
    panel.innerHTML = `<div class="panel"><div class="toolbar"><span class="count">${networks.length}</span><button class="btn ghost" data-refresh-n>${icon("refresh")}${esc(t("common.refresh"))}</button></div><div class="table-wrap"><table class="tbl"><thead><tr><th>${esc(t("common.name"))}</th><th>${esc(t("docker.driver"))}</th><th>${esc(t("docker.scope"))}</th><th>Internal</th></tr></thead><tbody>${networks.length ? networks.map((n) => `<tr><td><b>${esc(n.Name)}</b><div class="type-note mono">${esc((n.Id || "").slice(0, 12))}</div></td><td>${esc(n.Driver || "—")}</td><td>${esc(n.Scope || "—")}</td><td>${n.Internal ? esc(t("common.yes")) : esc(t("common.no"))}</td></tr>`).join("") : `<tr><td colspan="4"><div class="empty"><p>${esc(t("docker.noNetworks"))}</p></div></td></tr>`}</tbody></table></div></div>`;
    panel.querySelector("[data-refresh-n]").addEventListener("click", load);
  };
  const paintDF = () => {
    if (!df) { panel.innerHTML = `<div class="skeleton" style="height:200px;border-radius:14px"></div>`; return; }
    const items = [
      [t("docker.dfImages"), df.Images],
      [t("docker.dfContainers"), df.Containers],
      [t("docker.dfVolumes"), df.Volumes],
      [t("docker.dfBuild"), df.BuildCache],
    ];
    panel.innerHTML = `<div class="panel"><div class="panel-head"><h3>${esc(t("docker.storage"))}</h3><button class="btn ghost" data-refresh-d>${icon("refresh")}${esc(t("common.refresh"))}</button></div><div class="panel-body"><div class="tiles">${items.map(([label, v]) => `<div class="tile"><div class="k">${esc(label)}</div><div class="v">${esc(bytes(v?.TotalSize))}</div><div class="s">${esc(t("docker.count", { n: v?.TotalCount || 0 }))}</div></div>`).join("")}</div></div></div>`;
    panel.querySelector("[data-refresh-d]").addEventListener("click", load);
  };

  const detail = async (c) => {
    const name = (c.Names?.[0] || c.Id || "").replace(/^\//, "");
    const m = modal({
      title: name,
      wide: true,
      body: `<dl class="kv"><dt>${esc(t("docker.container"))}</dt><dd class="mono">${esc(c.Id || "—")}</dd><dt>${esc(t("docker.image"))}</dt><dd>${esc(c.Image || "—")}</dd><dt>${esc(t("docker.state"))}</dt><dd>${esc(c.State || "—")} · ${esc(c.Status || "")}</dd><dt>${esc(t("docker.ports"))}</dt><dd>${esc((c.Ports || []).map((p) => `${p.IP || ""}:${p.PublicPort || "—"}→${p.PrivatePort}/${p.Type}`).join(", ") || "—")}</dd><dt>Command</dt><dd class="mono">${esc(c.Command || "—")}</dd><dt>${esc(t("common.created"))}</dt><dd>${esc(dateTime((c.Created || 0) * 1000))}</dd></dl><div data-stats class="tiles" style="margin-top:12px"><div class="tile"><div class="k">${esc(t("common.loading"))}</div></div></div>`,
      actions: [
        { label: t("docker.logs"), icon: "file", onClick: (close) => { close(); logs(c); } },
        { label: t("common.close"), onClick: (close) => close() },
      ],
    });
    try {
      const s = await api.get(`/api/docker/containers/${encodeURIComponent(c.Id)}/stats`);
      m.body.querySelector("[data-stats]").innerHTML = `<div class="tile"><div class="k">${esc(t("docker.cpu"))}</div><div class="v">${pct(s.cpu_percent, 1)}</div></div><div class="tile"><div class="k">${esc(t("docker.mem"))}</div><div class="v">${esc(bytes(s.memory_bytes))}</div><div class="s">${esc(bytes(s.memory_limit_bytes))}</div></div><div class="tile"><div class="k">RX</div><div class="v">${esc(bytes(s.rx_bytes))}</div></div><div class="tile"><div class="k">TX</div><div class="v">${esc(bytes(s.tx_bytes))}</div></div>`;
    } catch (err) {
      m.body.querySelector("[data-stats]").innerHTML = `<div class="type-note">${esc(errorText(err))}</div>`;
    }
  };
  const logs = async (c) => {
    const name = (c.Names?.[0] || c.Id || "").replace(/^\//, "");
    const m = modal({
      title: `${t("docker.logs")} · ${name}`,
      wide: true,
      body: `<pre class="logbox" data-log>${esc(t("common.loading"))}</pre>`,
      actions: [{ label: t("common.close"), onClick: (close) => close() }],
    });
    try {
      const res = await api.get(`/api/docker/containers/${encodeURIComponent(c.Id)}/logs?tail=400`);
      m.body.querySelector("[data-log]").textContent = res.logs || t("system.logsEmpty");
    } catch (err) { m.body.querySelector("[data-log]").textContent = errorText(err); }
  };
  const action = async (c, act, button = null) => {
    const name = (c.Names?.[0] || c.Id || "").replace(/^\//, "");
    if (act === "remove") {
      try {
        await typedConfirm({ title: t("docker.removeTitle"), message: t("docker.removeBody", { name }), word: name, password: false, confirmLabel: t("docker.remove") });
        await api.post(`/api/docker/containers/${encodeURIComponent(c.Id)}/remove`, {}, { confirm: c.Id });
        toast("ok", t("docker.removed"));
        load();
      } catch (err) { if (err?.message !== "cancelled") toast("bad", t("toast.error"), errorText(err)); }
      return;
    }
    setBusy(button, true);
    try {
      await api.post(`/api/docker/containers/${encodeURIComponent(c.Id)}/${act}`, {});
      toast("ok", act === "stop" ? t("docker.stopped") : act === "start" ? t("docker.started") : t("common.success"), name);
      load();
    } catch (err) { toast("bad", t("toast.error"), errorText(err)); }
    setBusy(button, false);
  };
  const removeImage = async (img) => {
    const name = (img.RepoTags || []).find((x) => x && !x.includes("<none>")) || img.Id;
    try {
      await typedConfirm({ title: t("docker.removeImageTitle"), message: t("docker.removeImageBody", { name }), word: name, password: false, confirmLabel: t("docker.remove") });
      await api.del(`/api/docker/images/${encodeURIComponent(img.Id)}`, { confirm: img.Id });
      toast("ok", t("docker.imageRemoved"));
      load();
    } catch (err) { if (err?.message !== "cancelled") toast("bad", t("toast.error"), errorText(err)); }
  };
  const prune = async () => {
    try {
      await typedConfirm({ title: t("docker.pruneTitle"), message: t("docker.pruneBody"), word: "PRUNE", password: false, confirmLabel: t("docker.prune") });
      const res = await api.post("/api/docker/prune", { confirm: "PRUNE" });
      const reclaimed = res?.result?.SpaceReclaimed ?? res?.result?.space_reclaimed ?? res?.result?.space;
      toast("ok", t("docker.pruneDone"), reclaimed ? t("docker.reclaimed", { size: bytes(reclaimed) }) : "");
      load();
    } catch (err) { if (err?.message !== "cancelled") toast("bad", t("toast.error"), errorText(err)); }
  };
  const load = async () => {
    try {
      if (active === "containers") {
        const res = await api.get(`/api/docker/containers?all=${showStopped}`);
        containers = res.containers || [];
        paintContainers();
      } else if (active === "images") {
        const res = await api.get("/api/docker/images");
        images = res.images || [];
        paintImages();
      } else if (active === "volumes") {
        const res = await api.get("/api/docker/volumes");
        volumes = res.volumes || [];
        paintVolumes();
      } else if (active === "networks") {
        const res = await api.get("/api/docker/networks");
        networks = res.networks || [];
        paintNetworks();
      } else {
        df = await api.get("/api/docker/df");
        paintDF();
      }
    } catch (err) {
      panel.innerHTML = notice(err);
      bindRetry();
    }
  };
  paint();
  load();
  timer = setInterval(() => { if (alive && active === "containers") load(); }, 8000);
  return () => { alive = false; clearInterval(timer); };
}
