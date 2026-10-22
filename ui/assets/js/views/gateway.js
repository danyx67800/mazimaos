import { brandMark, icon } from "../icons.js";
import { esc } from "../dom.js";
import { t } from "../i18n.js";

export function gateway(root, { kicker, title, subtitle, card, features, foot, tagline }) {
  root.innerHTML = `<main class="gateway">
    <section class="gateway-brand">
      <div class="brand">${brandMark()}<div><div class="brand-name">Mazima<span>OS</span></div><div class="brand-sub">${esc(t("app.tagline"))}</div></div></div>
      <div class="gateway-copy">
        <div class="gateway-kicker">${esc(kicker)}</div>
        <h1>${title}</h1>
        <p>${esc(subtitle || tagline || t("app.tagline"))}</p>
        <div class="gateway-features">${(features || []).map((f) => `<div class="gateway-feature">${icon(f.icon)}<strong>${esc(f.title)}</strong><span>${esc(f.body)}</span></div>`).join("")}</div>
      </div>
      <div class="gateway-foot"><span class="dot"></span>${esc(foot || t("setup.foot"))}</div>
    </section>
    <section class="gateway-auth"><div class="auth-card">
      <div class="brand">${brandMark()}<div><div class="brand-name">Mazima<span>OS</span></div><div class="brand-sub">${esc(t("app.tagline"))}</div></div></div>
      ${card || ""}
    </div></section>
  </main>`;
  return root.querySelector(".auth-card");
}

export function setupFeatures() {
  return [
    { icon: "apps", title: t("setup.feature1Title"), body: t("setup.feature1Body") },
    { icon: "terminal", title: t("setup.feature2Title"), body: t("setup.feature2Body") },
    { icon: "shield", title: t("setup.feature3Title"), body: t("setup.feature3Body") },
  ];
}

export function tzOptions(selected) {
  const list = [
    "Europe/Rome", "Europe/London", "Europe/Paris", "Europe/Berlin", "Europe/Madrid",
    "America/New_York", "America/Chicago", "America/Denver", "America/Los_Angeles",
    "America/Sao_Paulo", "Asia/Tokyo", "Asia/Shanghai", "Asia/Kolkata", "Australia/Sydney", "UTC",
  ];
  if (selected && !list.includes(selected)) list.unshift(selected);
  return list.map((z) => `<option value="${esc(z)}"${z === selected ? " selected" : ""}>${esc(z.replaceAll("_", " "))}</option>`).join("");
}

export function passwordMeter(input, meter) {
  const update = () => {
    const v = input.value || "";
    let score = 0;
    if (v.length >= 12) score++;
    if (v.length >= 16) score++;
    if (/[a-z]/.test(v) && /[A-Z]/.test(v)) score++;
    if (/\d/.test(v)) score++;
    if (/[^A-Za-z0-9]/.test(v)) score++;
    const bars = meter.querySelectorAll("i");
    bars.forEach((b, i) => {
      b.className = "";
      if (i < Math.min(4, Math.ceil(score * 0.8))) b.className = score <= 1 ? "low" : score <= 3 ? "mid" : "on";
    });
  };
  input.addEventListener("input", update);
  update();
}
