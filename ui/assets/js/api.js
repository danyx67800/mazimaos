let csrf = "";
export class ApiError extends Error {
  constructor(status, code, message) {
    super(message || code || `HTTP ${status}`);
    this.status = status;
    this.code = code || "error";
  }
}
export function setCSRF(value) { csrf = value || ""; }
export function getCSRF() { return csrf; }
export function clearCSRF() { csrf = ""; }

export async function api(path, options = {}) {
  const method = (options.method || "GET").toUpperCase();
  const headers = new Headers(options.headers || {});
  headers.set("Accept", "application/json");
  let body = options.body;
  if (body !== undefined && body !== null && !(body instanceof FormData) && !(body instanceof Blob)) {
    if (typeof body !== "string") {
      headers.set("Content-Type", "application/json");
      body = JSON.stringify(body);
    }
  }
  if (method !== "GET" && method !== "HEAD") {
    if (csrf) headers.set("X-CSRF-Token", csrf);
    if (options.confirm) headers.set("X-Mazima-Confirm", options.confirm);
  }
  let res;
  try {
    res = await fetch(path, {
      method, headers, body,
      credentials: "same-origin",
      cache: "no-store",
      signal: options.signal,
    });
  } catch (err) {
    if (err.name === "AbortError") throw err;
    throw new ApiError(0, "network", "network unavailable");
  }
  if (res.status === 204) return null;
  const text = await res.text();
  let data = null;
  if (text) {
    try { data = JSON.parse(text); } catch { data = { error: text }; }
  }
  if (!res.ok) {
    if (res.status === 401 && !options.noAuthRedirect) {
      clearCSRF();
      window.dispatchEvent(new CustomEvent("mazima:auth-required"));
    }
    throw new ApiError(res.status, data?.code, data?.error || res.statusText);
  }
  return data;
}
api.get = (p, o = {}) => api(p, { ...o, method: "GET" });
api.post = (p, body, o = {}) => api(p, { ...o, method: "POST", body });
api.patch = (p, body, o = {}) => api(p, { ...o, method: "PATCH", body });
api.put = (p, body, o = {}) => api(p, { ...o, method: "PUT", body });
api.del = (p, o = {}) => api(p, { ...o, method: "DELETE" });
