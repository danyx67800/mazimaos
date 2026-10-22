import { expect, test } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";

const dataDir = path.resolve(process.env.E2E_DATA_DIR || ".e2e-data");
const password = "MazimaE2E!2026";
const username = "admin";

function setupToken() {
  return fs.readFileSync(path.join(dataDir, "setup.token"), "utf8").trim();
}

test("setup, desktop apps, command palette, theme and login", async ({ page }) => {
  const pageErrors = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));

  await page.goto("/");
  await expect(page.locator("#su-host")).toBeVisible();
  await page.fill("#su-host", "mazima-e2e");
  await page.locator('form[data-step="1"] button[type="submit"]').click();

  await expect(page.locator("#su-user")).toBeVisible();
  await page.fill("#su-user", username);
  await page.fill("#su-pass", password);
  await page.fill("#su-pass2", password);
  await page.fill("#su-token", setupToken());
  await page.locator('form[data-step="2"] button[type="submit"]').click();

  await expect(page.locator(".desktop")).toBeVisible({ timeout: 20_000 });
  await expect(page.locator(".dock-btn")).toHaveCount(9);
  await expect(page.locator("[data-username]")).toContainText(username);

  for (const app of ["apps", "system", "docker", "remote", "updates", "audit", "settings"]) {
    await page.locator(`[data-dock="${app}"]`).click();
    await expect(page.locator(`.window[data-app="${app}"]`)).toBeVisible();
    await page.locator(`.window[data-app="${app}"] [data-close]`).click();
    await expect(page.locator(`.window[data-app="${app}"]`)).toHaveCount(0);
  }

  await page.locator("[data-command]").click();
  await expect(page.locator(".cmdk-panel")).toBeVisible();
  await page.locator(".cmdk-input input").click();
  await page.keyboard.press("Escape");
  await expect(page.locator(".cmdk")).toHaveCount(0);

  const themeBefore = await page.locator("html").getAttribute("data-theme");
  await page.locator("button[data-theme]").click();
  await expect(page.locator("html")).not.toHaveAttribute("data-theme", themeBefore);
  await page.locator("button[data-theme]").click();

  await page.locator("[data-user]").click();
  await page.locator('[data-menu="logout"]').click();
  await expect(page.locator(".modal")).toBeVisible();
  await page.locator(".modal-foot .btn.danger").click();
  await expect(page.locator("#li-user")).toBeVisible({ timeout: 20_000 });

  await page.fill("#li-user", username);
  await page.fill("#li-pass", password);
  await page.locator(".auth-form button[type=submit]").click();
  await expect(page.locator(".desktop")).toBeVisible({ timeout: 20_000 });

  const serviceWorker = await page.evaluate(async () => {
    const registration = await navigator.serviceWorker.ready;
    const keys = await caches.keys();
    return { state: registration.active?.state, scope: registration.scope, keys };
  });
  expect(serviceWorker.state).toBe("activated");
  expect(serviceWorker.scope).toContain("127.0.0.1");
  expect(serviceWorker.keys).toContain("mazima-shell-v2");

  expect(pageErrors).toEqual([]);
});
