import { api } from "../api.js";
import { esc, modal, relative, toast } from "../dom.js";
import { errorText, t } from "../i18n.js";
import { icon } from "../icons.js";

export function renderSession(root, ctx, win) {
  const spec = win?.spec || {};
  const params = spec.params || win?.params || {};
  const requestedSession = params.sessionId || spec.sessionId || win?.sessionId || "";
  const requestedApp = params.appId || spec.appId || "";
  let alive = true;
  let ws = null;
  let session = null;
  let video = null;
  let apps = [];
  let sessions = [];
  let supported = true;
  let unsupportedReason = "";
  let overlayAction = null;
  let lastFrameAt = 0;
  let frameCount = 0;
  let fps = 0;
  let framesTotal = 0;
  let pendingBlob = null;
  let decoding = false;
  let intentionalStop = false;
  let reconnectAttempts = 0;
  let reconnectTimer = null;
  let pingTimer = null;
  let fpsTimer = null;
  let pointerRaf = 0;
  let pendingPointer = null;
  let lastWheelAt = 0;
  let lastPointerButton = 0;
  let stats = {};
  let latency = null;
  let canvas = null;
  let ctx2d = null;
  let stage = null;
  let overlay = null;

  root.innerHTML = `<div class="session-view" data-shell><div class="empty"><span class="spinner"></span></div></div>`;
  const shell = root.querySelector("[data-shell]");

  const setWindowInfo = (title, sub) => {
    win?.setTitle?.(title);
    win?.setSub?.(sub);
  };

  const showError = (message, retry) => {
    shell.innerHTML = `<div class="panel"><div class="empty">${icon("alert", "icon")}<strong>${esc(t("session.streamError"))}</strong><p>${esc(message)}</p><div class="row-actions"><button class="btn" data-retry>${icon("refresh")}${esc(t("common.retry"))}</button></div></div></div>`;
    shell.querySelector("[data-retry]")?.addEventListener("click", retry);
  };

  const appName = (id) => apps.find((a) => a.id === id)?.name || id || t("session.app");
  const statusLabel = (state) => state === "running" ? t("session.running") : state === "starting" ? t("session.starting") : state || "—";

  const renderManage = async () => {
    if (!alive) return;
    closeStream();
    session = null;
    setWindowInfo(t("session.title"), t("session.subtitle"));
    shell.innerHTML = `<div class="view-stack">
      <div class="section-head"><div><h2>${esc(t("session.title"))}</h2><div class="type-note">${esc(t("session.subtitle"))}</div></div><button class="btn ghost" data-refresh>${icon("refresh")}${esc(t("common.refresh"))}</button></div>
      <div data-notice></div>
      <div class="panel"><div class="panel-head"><h3>${esc(t("session.apps"))}</h3><span class="chip">${icon("monitor")}${esc(t("session.streamed"))}</span></div><div class="panel-body" data-apps><div class="empty"><span class="spinner"></span></div></div></div>
      <div class="panel"><div class="panel-head"><h3>${esc(t("session.active"))}</h3><button class="btn ghost sm" data-refresh-sessions>${icon("refresh")}</button></div><div class="panel-body" data-sessions></div></div>
    </div>`;
    shell.querySelector("[data-refresh]")?.addEventListener("click", loadManage);
    shell.querySelector("[data-refresh-sessions]")?.addEventListener("click", loadManage);
    await loadManage();
  };

  const loadManage = async () => {
    const appsBox = shell.querySelector("[data-apps]");
    const sessionsBox = shell.querySelector("[data-sessions]");
    const notice = shell.querySelector("[data-notice]");
    try {
      const [appsRes, sessionsRes] = await Promise.all([
        api.get("/api/sessions/apps"),
        api.get("/api/sessions"),
      ]);
      if (!alive) return;
      apps = appsRes.apps || [];
      supported = !!appsRes.supported;
      unsupportedReason = appsRes.reason || "";
      video = appsRes.video || video;
      sessions = sessionsRes.sessions || [];
      notice.innerHTML = !supported ? `<div class="notice warn">${icon("alert")}<div><strong>${esc(t("session.unsupported"))}</strong><p>${esc(unsupportedReason || t("session.unsupportedBody"))}</p></div></div>` : "";
      if (appsBox) {
        appsBox.innerHTML = apps.length ? `<div class="stream-apps">${apps.map((a) => `<div class="stream-app ${a.available ? "" : "disabled"}"><div class="stream-app-icon">${icon(a.id === "browser" ? "globe" : "terminal")}</div><div class="grow"><strong>${esc(a.name)}</strong><p>${esc(a.description)}</p>${a.available ? "" : `<span class="badge warn">${esc(t("session.missing"))}: ${esc(a.missing || "")}</span>`}</div><button class="btn primary sm" data-start="${esc(a.id)}" ${!supported || !a.available ? "disabled" : ""}>${icon("play")}${esc(t("session.start"))}</button></div>`).join("")}</div>` : `<div class="empty">${icon("monitor", "icon")}<strong>${esc(t("session.noApps"))}</strong><p>${esc(t("session.noAppsBody"))}</p></div>`;
        appsBox.querySelectorAll("[data-start]").forEach((btn) => btn.addEventListener("click", () => startApp(btn.dataset.start, btn)));
      }
      if (sessionsBox) {
        sessionsBox.innerHTML = sessions.length ? `<div class="session-list">${sessions.map((s) => `<div class="session-item"><div class="stream-app-icon">${icon("monitor")}</div><div class="grow"><strong>${esc(s.title || appName(s.app_id))}</strong><span class="type-note">${esc(appName(s.app_id))} · ${esc(s.width || video?.width || 1280)}×${esc(s.height || video?.height || 800)} · ${esc(relative(s.created_at))}</span></div><span class="badge ${s.state === "running" ? "ok" : "warn"}">${esc(statusLabel(s.state))}</span><div class="row-actions"><button class="btn sm" data-resume="${esc(s.id)}">${icon("play")}${esc(t("session.resume"))}</button><button class="btn danger sm" data-stop="${esc(s.id)}">${icon("stop")}${esc(t("session.stop"))}</button></div></div>`).join("")}</div>` : `<div class="empty"><p>${esc(t("session.noActive"))}</p></div>`;
        sessionsBox.querySelectorAll("[data-resume]").forEach((b) => b.addEventListener("click", () => attach(b.dataset.resume)));
        sessionsBox.querySelectorAll("[data-stop]").forEach((b) => b.addEventListener("click", () => stopSession(b.dataset.stop)));
      }
    } catch (err) {
      if (!alive) return;
      notice.innerHTML = `<div class="notice bad">${icon("alert")}<div><strong>${esc(t("toast.error"))}</strong><p>${esc(errorText(err))}</p></div></div>`;
    }
  };

  const startApp = async (appId, btn) => {
    if (!supported) return;
    btn.disabled = true;
    try {
      const res = await api.post("/api/sessions", { app_id: appId, title: appName(appId) });
      if (!alive) return;
      attach(res.session?.id || res.session, res.session);
    } catch (err) {
      btn.disabled = false;
      toast("bad", t("session.startFailed"), errorText(err));
    }
  };

  const stopSession = async (id) => {
    const target = sessions.find((s) => s.id === id);
    const closeModal = modal({
      title: t("session.stopTitle"),
      body: `<p class="type-note">${esc(t("session.stopBody", { app: target ? (target.title || appName(target.app_id)) : "" }))}</p>`,
      actions: [
        { label: t("common.cancel"), onClick: (close) => close() },
        { label: t("session.stop"), kind: "danger", onClick: async (close, button) => {
          button.disabled = true;
          try {
            await api.del(`/api/sessions/${encodeURIComponent(id)}`);
            closeModal.close();
            toast("ok", t("session.stopped"));
            if (session?.id === id) {
              intentionalStop = true;
              closeStream();
              await renderManage();
            } else {
              loadManage();
            }
          } catch (err) {
            button.disabled = false;
            toast("bad", t("toast.error"), errorText(err));
          }
        } },
      ],
    });
  };

  const attach = async (id, info) => {
    if (!id) return;
    closeStream();
    intentionalStop = false;
    reconnectAttempts = 0;
    session = info || sessions.find((s) => s.id === id) || null;
    setWindowInfo(session?.title || appName(session?.app_id) || t("session.title"), t("session.connecting"));
    renderStage();
    await connect();
    if (!alive) return;
  };

  const renderStage = () => {
    const title = session?.title || appName(session?.app_id) || t("session.app");
    shell.innerHTML = `<div class="session-shell">
      <div class="session-toolbar">
        <button class="btn ghost sm" data-back title="${esc(t("session.back"))}">${icon("arrowLeft")}<span>${esc(t("session.sessions"))}</span></button>
        <div class="session-title"><strong>${esc(title)}</strong><span data-conn>${esc(t("session.connecting"))}</span></div>
        <div class="session-stats"><span>${icon("cpu")}<b data-fps>0</b> FPS</span><span data-latency>— ms</span><span data-clients>0 ${esc(t("session.clients"))}</span><span data-res>${esc(session ? `${session.width || video?.width || ""}×${session.height || video?.height || ""}` : "—")}</span></div>
        <div class="row-actions">
          <button class="btn ghost sm" data-keyboard title="${esc(t("session.keyboard"))}">${icon("keyboard")}</button>
          <button class="btn ghost sm" data-paste title="${esc(t("session.paste"))}">${icon("clipboard")}</button>
          <button class="btn ghost sm" data-copy title="${esc(t("session.copy"))}">${icon("copy")}</button>
          <button class="btn ghost sm" data-cad title="Ctrl+Alt+Del">${icon("power")}</button>
          <button class="btn ghost sm" data-fullscreen title="${esc(t("session.fullscreen"))}">${icon("maximize")}</button>
          <button class="btn danger sm" data-stop title="${esc(t("session.stop"))}">${icon("stop")}</button>
        </div>
      </div>
      <div class="session-stage" data-stage>
        <canvas data-canvas tabindex="0" aria-label="${esc(title)}"></canvas>
        <div class="session-overlay" data-overlay><div class="session-overlay-card">${icon("monitor", "icon")}<div class="spinner"></div><strong data-overlay-title>${esc(t("session.connecting"))}</strong><span data-overlay-text>${esc(t("session.connectingBody"))}</span><button class="btn primary sm" data-overlay-action hidden></button></div></div>
        <textarea class="session-ime" data-ime autocapitalize="off" autocomplete="off" spellcheck="false" aria-hidden="true"></textarea>
      </div>
    </div>`;
    canvas = shell.querySelector("[data-canvas]");
    ctx2d = canvas.getContext("2d", { alpha: false, desynchronized: true });
    stage = shell.querySelector("[data-stage]");
    overlay = shell.querySelector("[data-overlay]");
    setOverlay("loading", t("session.connecting"), t("session.connectingBody"));
    bindStage();
    updateStats();
  };

  const setOverlay = (kind, title, text, actionLabel = "", action = null) => {
    if (!overlay) return;
    overlay.hidden = kind === "hidden";
    overlay.className = `session-overlay ${kind}`;
    const titleEl = overlay.querySelector("[data-overlay-title]");
    const textEl = overlay.querySelector("[data-overlay-text]");
    const btn = overlay.querySelector("[data-overlay-action]");
    const spinner = overlay.querySelector(".spinner");
    if (titleEl) titleEl.textContent = title || "";
    if (textEl) textEl.textContent = text || "";
    if (spinner) spinner.hidden = kind !== "loading" && kind !== "reconnecting";
    if (btn) {
      btn.hidden = !actionLabel;
      btn.textContent = actionLabel || "";
      overlayAction = action;
    }
  };

  const updateConn = (text, cls = "") => {
    const el = shell.querySelector("[data-conn]");
    if (el) { el.textContent = text; el.className = cls; }
    win?.setSub?.(text);
  };

  const updateStats = () => {
    const set = (sel, val) => { const el = shell.querySelector(sel); if (el) el.textContent = val; };
    set("[data-fps]", String(fps || Math.round(stats.fps || 0)));
    set("[data-clients]", `${stats.clients ?? 1} ${t("session.clients")}`);
    set("[data-latency]", latency == null ? "— ms" : `${latency} ms`);
    if (session || video) set("[data-res]", `${session?.width || video?.width || 0}×${session?.height || video?.height || 0}`);
  };

  const closeStream = () => {
    clearTimeout(reconnectTimer);
    clearInterval(pingTimer);
    clearInterval(fpsTimer);
    reconnectTimer = null;
    pingTimer = null;
    fpsTimer = null;
    if (pointerRaf) cancelAnimationFrame(pointerRaf);
    pointerRaf = 0;
    if (ws) {
      try { ws.onclose = null; ws.close(); } catch {}
      ws = null;
    }
    pendingBlob = null;
    if (stage) stage.classList.remove("connected");
  };

  const connect = async () => {
    if (!alive || !session) return;
    closeStream();
    setOverlay("loading", t("session.connecting"), t("session.connectingBody"));
    updateConn(t("session.connecting"));
    let tokenRes;
    try {
      tokenRes = await api.post(`/api/sessions/${encodeURIComponent(session.id)}/token`, {});
    } catch (err) {
      if (!alive) return;
      if (err.status === 404) {
        setOverlay("error", t("session.ended"), t("session.endedBody"), t("session.back"), () => renderManage());
        updateConn(t("session.ended"), "bad");
        return;
      }
      scheduleReconnect(errorText(err));
      return;
    }
    if (!alive) return;
    const url = new URL("/api/ws/session", window.location.href);
    url.protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    url.searchParams.set("id", session.id);
    url.searchParams.set("token", tokenRes.token);
    let socket;
    try {
      socket = new WebSocket(url);
    } catch (err) {
      scheduleReconnect(errorText(err));
      return;
    }
    ws = socket;
    socket.binaryType = "arraybuffer";
    socket.onopen = () => {
      if (!alive || ws !== socket) return;
      reconnectAttempts = 0;
      updateConn(t("session.connected"), "ok");
      setOverlay("loading", t("session.connecting"), t("session.connectingBody"));
      pingTimer = setInterval(() => send({ type: "ping", t: Date.now() }), 25000);
      fpsTimer = setInterval(() => {
        fps = frameCount;
        frameCount = 0;
        updateStats();
      }, 1000);
    };
    socket.onmessage = (event) => {
      if (!alive || ws !== socket) return;
      if (typeof event.data === "string") {
        let msg;
        try { msg = JSON.parse(event.data); } catch { return; }
        handleServerMessage(msg);
        return;
      }
      const buf = event.data;
      if (!(buf instanceof ArrayBuffer) || buf.byteLength < 13) return;
      const bytes = new Uint8Array(buf);
      if (bytes[0] !== 77 || bytes[1] !== 90 || bytes[2] !== 77 || bytes[3] !== 70) return;
      lastFrameAt = Date.now();
      receiveFrame(new Blob([bytes.subarray(12)], { type: "image/jpeg" }));
    };
    socket.onerror = () => {};
    socket.onclose = (event) => {
      if (!alive || ws !== socket) return;
      ws = null;
      if (intentionalStop) return;
      if (event.code === 1000) {
        setOverlay("ended", t("session.ended"), event.reason || t("session.endedBody"), t("session.back"), () => renderManage());
        updateConn(t("session.ended"), "bad");
        return;
      }
      scheduleReconnect(t("session.connectionLost"));
    };
  };

  const scheduleReconnect = (reason) => {
    if (!alive || intentionalStop) return;
    if (reconnectAttempts >= 5) {
      setOverlay("error", t("session.streamError"), reason || t("session.connectionLost"), t("common.retry"), () => { reconnectAttempts = 0; connect(); });
      updateConn(t("session.disconnected"), "bad");
      return;
    }
    const delay = Math.min(8000, 600 * 2 ** reconnectAttempts);
    reconnectAttempts++;
    updateConn(t("session.reconnecting"), "warn");
    setOverlay("reconnecting", t("session.reconnecting"), t("session.reconnectBody"), t("common.retry"), () => { reconnectAttempts = 0; connect(); });
    reconnectTimer = setTimeout(connect, delay);
  };

  const handleServerMessage = (msg) => {
    switch (msg.type) {
      case "hello":
        if (msg.session) {
          session = { ...session, ...msg.session };
          setWindowInfo(session.title || appName(session.app_id), t("session.connected"));
          shell.querySelector(".session-title strong")?.replaceChildren(document.createTextNode(session.title || appName(session.app_id)));
        }
        if (msg.video) video = msg.video;
        if (msg.session?.stats) stats = msg.session.stats;
        updateStats();
        break;
      case "stats":
        if (msg.stats) stats = msg.stats;
        updateStats();
        break;
      case "clipboard":
        showClipboardText(msg.text || "");
        break;
      case "pong":
        if (typeof msg.t === "number") { latency = Math.max(0, Date.now() - msg.t); updateStats(); }
        break;
      case "error":
        toast("bad", t("session.streamError"), msg.error || "");
        setOverlay("error", t("session.streamError"), msg.error || "", t("session.back"), () => renderManage());
        break;
    }
  };

  const receiveFrame = (blob) => {
    pendingBlob = blob;
    if (decoding) return;
    drainFrames();
  };

  const drainFrames = async () => {
    decoding = true;
    while (pendingBlob && alive) {
      const blob = pendingBlob;
      pendingBlob = null;
      let bitmap;
      try {
        bitmap = await createImageBitmap(blob);
      } catch {
        continue;
      }
      if (!alive) { bitmap.close?.(); break; }
      if (canvas.width !== bitmap.width || canvas.height !== bitmap.height) {
        canvas.width = bitmap.width;
        canvas.height = bitmap.height;
      }
      ctx2d.drawImage(bitmap, 0, 0);
      bitmap.close?.();
      frameCount++;
      framesTotal++;
      if (overlay && !overlay.hidden && !overlay.classList.contains("reconnecting")) {
        overlay.hidden = true;
        stage?.classList.add("connected");
        canvas.focus({ preventScroll: true });
      }
    }
    decoding = false;
  };

  const toSessionPoint = (clientX, clientY) => {
    if (!canvas || !canvas.width || !canvas.height) return null;
    const rect = canvas.getBoundingClientRect();
    if (rect.width === 0 || rect.height === 0) return null;
    const scale = Math.min(rect.width / canvas.width, rect.height / canvas.height);
    const drawW = canvas.width * scale;
    const drawH = canvas.height * scale;
    const left = rect.left + (rect.width - drawW) / 2;
    const top = rect.top + (rect.height - drawH) / 2;
    if (clientX < left || clientY < top || clientX > left + drawW || clientY > top + drawH) return null;
    return {
      x: Math.min(canvas.width - 1, Math.max(0, (clientX - left) / scale)),
      y: Math.min(canvas.height - 1, Math.max(0, (clientY - top) / scale)),
    };
  };

  const send = (payload) => {
    if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(payload));
  };
  const sendEvent = (event) => send({ type: "input", event });

  let pointerDown = false;
  const bindStage = () => {
    const canvasEl = canvas;
    const ime = shell.querySelector("[data-ime]");
    shell.querySelector("[data-overlay-action]")?.addEventListener("click", () => overlayAction?.());
    shell.querySelector("[data-back]")?.addEventListener("click", () => { closeStream(); renderManage(); });
    shell.querySelector("[data-stop]")?.addEventListener("click", () => { if (session) stopSession(session.id); });
    shell.querySelector("[data-keyboard]")?.addEventListener("click", () => { ime?.focus(); });
    shell.querySelector("[data-paste]")?.addEventListener("click", pasteToServer);
    shell.querySelector("[data-copy]")?.addEventListener("click", copyFromServer);
    shell.querySelector("[data-cad]")?.addEventListener("click", () => sendEvent({ type: "key", key: "Delete", code: "Delete", down: true, ctrl: true, alt: true }));
    shell.querySelector("[data-fullscreen]")?.addEventListener("click", toggleFullscreen);

    canvasEl.addEventListener("pointermove", (e) => {
      const p = toSessionPoint(e.clientX, e.clientY);
      if (!p) return;
      pendingPointer = p;
      if (!pointerRaf) {
        pointerRaf = requestAnimationFrame(() => {
          pointerRaf = 0;
          if (pendingPointer) sendEvent({ type: "pointer", x: pendingPointer.x, y: pendingPointer.y });
        });
      }
    });
    canvasEl.addEventListener("pointerdown", (e) => {
      const p = toSessionPoint(e.clientX, e.clientY);
      if (!p) return;
      e.preventDefault();
      canvasEl.focus({ preventScroll: true });
      pointerDown = true;
      lastPointerButton = e.button === 1 ? 2 : e.button === 2 ? 3 : 1;
      canvasEl.setPointerCapture?.(e.pointerId);
      sendEvent({ type: "button", x: p.x, y: p.y, button: lastPointerButton, down: true });
    });
    const release = (e) => {
      if (!pointerDown) return;
      pointerDown = false;
      const p = toSessionPoint(e.clientX, e.clientY);
      if (p) sendEvent({ type: "button", x: p.x, y: p.y, button: lastPointerButton, down: false });
    };
    canvasEl.addEventListener("pointerup", release);
    canvasEl.addEventListener("pointercancel", release);
    canvasEl.addEventListener("contextmenu", (e) => e.preventDefault());
    canvasEl.addEventListener("wheel", (e) => {
      e.preventDefault();
      const now = performance.now();
      if (now - lastWheelAt < 50) return;
      lastWheelAt = now;
      const delta = e.deltaMode === 1 ? e.deltaY * 24 : e.deltaY;
      sendEvent({ type: "wheel", delta_y: delta });
    }, { passive: false });
    canvasEl.addEventListener("keydown", (e) => {
      if (e.metaKey) return;
      e.preventDefault();
      const text = !e.ctrlKey && !e.altKey && !e.isComposing && e.key.length === 1 ? e.key : "";
      sendEvent({ type: "key", key: e.key, code: e.code, text, down: true, ctrl: e.ctrlKey, alt: e.altKey, shift: e.shiftKey, meta: false });
    });
    canvasEl.addEventListener("keyup", (e) => {
      if (e.metaKey) return;
      sendEvent({ type: "key", key: e.key, code: e.code, down: false, ctrl: e.ctrlKey, alt: e.altKey, shift: e.shiftKey, meta: false });
    });
    if (ime) {
      ime.addEventListener("keydown", (e) => {
        if (["Backspace", "Delete", "Enter", "Tab", "Escape", "ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight"].includes(e.key)) {
          e.preventDefault();
          sendEvent({ type: "key", key: e.key, code: e.code, down: true, ctrl: e.ctrlKey, alt: e.altKey, shift: e.shiftKey });
        }
      });
      ime.addEventListener("input", () => {
        const text = ime.value;
        ime.value = "";
        if (text) sendEvent({ type: "text", text });
      });
    }
    canvasEl.focus({ preventScroll: true });
  };

  const pasteToServer = async () => {
    let text = "";
    try {
      text = await navigator.clipboard.readText();
    } catch {
      const m = modal({
        title: t("session.paste"),
        body: `<div class="field"><label>${esc(t("session.pasteHint"))}</label><textarea class="input" rows="6" data-clip></textarea></div>`,
        actions: [
          { label: t("common.cancel"), onClick: (close) => close() },
          { label: t("session.send"), kind: "primary", onClick: async (close) => {
            const val = m.body.querySelector("[data-clip]").value;
            if (!val) return close();
            try { await api.put(`/api/sessions/${encodeURIComponent(session.id)}/clipboard`, { text: val }); close(); toast("ok", t("session.clipboardSent")); }
            catch (err) { toast("bad", t("toast.error"), errorText(err)); }
          } },
        ],
      });
      return;
    }
    if (!text) return;
    try {
      await api.put(`/api/sessions/${encodeURIComponent(session.id)}/clipboard`, { text });
      toast("ok", t("session.clipboardSent"));
    } catch (err) { toast("bad", t("toast.error"), errorText(err)); }
  };

  const showClipboardText = async (text) => {
    try {
      await navigator.clipboard.writeText(text);
      toast("ok", t("session.clipboardCopied"));
    } catch {
      modal({
        title: t("session.copy"),
        body: `<p class="type-note">${esc(t("session.copyHint"))}</p><textarea class="input mono" rows="6" readonly>${esc(text)}</textarea>`,
        actions: [{ label: t("common.close"), onClick: (close) => close() }],
      });
    }
  };

  const copyFromServer = async () => {
    try {
      const res = await api.get(`/api/sessions/${encodeURIComponent(session.id)}/clipboard`);
      await showClipboardText(res.text || "");
    } catch (err) { toast("bad", t("toast.error"), errorText(err)); }
  };

  const toggleFullscreen = async () => {
    try {
      if (document.fullscreenElement) await document.exitFullscreen();
      else await stage.requestFullscreen();
    } catch (err) { toast("warn", t("session.fullscreen"), errorText(err)); }
  };

  const cleanup = () => {
    alive = false;
    closeStream();
    stage = null;
    canvas = null;
    overlay = null;
  };

  (async () => {
    await renderManage();
    if (!alive) return;
    if (requestedSession) {
      const found = sessions.find((s) => s.id === requestedSession);
      if (found) attach(found.id, found);
      else toast("warn", t("session.ended"));
    } else if (requestedApp) {
      const found = apps.find((a) => a.id === requestedApp);
      if (found && found.available && supported) {
        const card = shell.querySelector(`[data-start="${CSS.escape(found.id)}"]`);
        if (card) startApp(found.id, card);
      }
    }
  })();

  return cleanup;
}
