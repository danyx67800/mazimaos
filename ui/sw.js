const CACHE = "mazima-shell-v2";
const SHELL = [
  "/", "/index.html", "/manifest.webmanifest", "/icon.svg",
  "/assets/icon-192.png", "/assets/icon-512.png",
  "/assets/css/app.css",
  "/assets/js/api.js", "/assets/js/i18n.js", "/assets/js/icons.js", "/assets/js/dom.js",
  "/assets/js/windows.js", "/assets/js/boot.js", "/assets/js/desktop.js",
  "/assets/js/views/setup.js", "/assets/js/views/login.js", "/assets/js/views/gateway.js",
  "/assets/js/views/dashboard.js", "/assets/js/views/system.js", "/assets/js/views/apps.js",
  "/assets/js/views/docker.js", "/assets/js/views/settings.js", "/assets/js/views/remote.js",
  "/assets/js/views/updates.js", "/assets/js/views/audit.js", "/assets/js/views/session.js"
];
self.addEventListener("install", (event) => {
  event.waitUntil(caches.open(CACHE).then((cache) => cache.addAll(SHELL)).then(() => self.skipWaiting()));
});
self.addEventListener("activate", (event) => {
  event.waitUntil(caches.keys().then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k)))).then(() => self.clients.claim()));
});
self.addEventListener("fetch", (event) => {
  const url = new URL(event.request.url);
  if (event.request.method !== "GET" || url.origin !== self.location.origin) return;
  if (url.pathname.startsWith("/api/") || url.pathname.startsWith("/healthz")) return;
  if (event.request.mode === "navigate") {
    event.respondWith(fetch(event.request).catch(() => caches.match("/index.html")));
    return;
  }
  event.respondWith(caches.match(event.request).then((hit) => hit || fetch(event.request)));
});
