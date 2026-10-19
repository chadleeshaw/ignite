// Capture the README screenshots from a running Ignite server.
//
//   IGNITE_URL=http://127.0.0.1:8080 node scripts/screenshots.mjs
//
// Chrome is taken from CHROME_PATH, or google-chrome / chromium on PATH.
// The server should already be logged-in-able as admin/admin and seeded
// (scripts/seed-demo.go). Writes PNGs under public/http/img/.

import { spawnSync } from "node:child_process";
import { mkdirSync, statSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import puppeteer from "puppeteer-core";
import sharp from "sharp";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const outDir = resolve(root, "public/http/img");
const base = process.env.IGNITE_URL || "http://127.0.0.1:8080";

function findChrome() {
  if (process.env.CHROME_PATH) return process.env.CHROME_PATH;
  for (const name of ["google-chrome-stable", "google-chrome", "chromium", "chromium-browser"]) {
    const found = spawnSync("which", [name], { encoding: "utf8" });
    if (found.status === 0 && found.stdout.trim()) return found.stdout.trim();
  }
  throw new Error("Chrome not found. Set CHROME_PATH.");
}

async function save(page, filename) {
  const png = await page.screenshot({ type: "png" });
  const file = resolve(outDir, filename);
  await sharp(png).png({ compressionLevel: 9, effort: 10 }).toFile(file);
  const kb = Math.round(statSync(file).size / 1024);
  console.log(`${filename} ${kb}KB`);
}

async function settle(page) {
  await page.waitForNetworkIdle({ idleTime: 400, timeout: 8000 }).catch(() => {});
  await new Promise((r) => setTimeout(r, 250));
}

const browser = await puppeteer.launch({
  executablePath: findChrome(),
  headless: true,
  args: ["--no-sandbox", "--disable-dev-shm-usage", "--hide-scrollbars", "--force-device-scale-factor=1"],
  defaultViewport: { width: 1280, height: 800, deviceScaleFactor: 1 },
});

try {
  mkdirSync(outDir, { recursive: true });
  const page = await browser.newPage();
  page.setDefaultTimeout(20000);
  await page.goto(`${base}/login`, { waitUntil: "networkidle0" });
  await page.type("#username", "admin");
  await page.type("#password", "admin");
  await page.click('#login-form button[type="submit"]');
  await page.waitForFunction(() => location.pathname === "/");
  await settle(page);

  await save(page, "main_page.png");
  await page.evaluate(() => igniteSetMode("console"));
  await new Promise((r) => setTimeout(r, 200));
  await save(page, "console_page.png");

  const pages = [
    ["/dhcp", "dhcp_page.png"],
    ["/tftp", "tftp_page.png"],
    ["/osimages", "osimage_page.png"],
    ["/syslinux", "syslinux_page.png"],
  ];
  for (const [path, file] of pages) {
    await page.goto(base + path, { waitUntil: "networkidle0" });
    await settle(page);
    await save(page, file);
  }

  await page.goto(`${base}/provision`, { waitUntil: "networkidle0" });
  await settle(page);
  await page.evaluate(() => {
    const item = document.querySelector(".file-item");
    if (item && typeof selectFile === "function") selectFile(item);
  });
  await page.waitForFunction(() => {
    const editor = document.getElementById("editor-textarea");
    return editor && editor.value.length > 0;
  });
  await save(page, "provision_page.png");
} finally {
  await browser.close();
}
