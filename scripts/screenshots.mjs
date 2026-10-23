// Capture the README screenshots from a running Ignite server.
//
//   IGNITE_URL=http://127.0.0.1:8080 node scripts/screenshots.mjs
//
// Chrome is taken from CHROME_PATH, or google-chrome / chromium on PATH.
// The server should already be logged-in-able as admin/admin and seeded
// (scripts/seed-demo.go). Writes a Cozy and a Console PNG for each nav
// page under public/http/img/, plus the login page and the DHCP host
// detail. Each navigation reloads Cozy from the server, then the script
// switches to Console before the second capture. Viewport width is 1280.

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
  await page.waitForFunction(() => {
    const nodes = [...document.querySelectorAll("[data-time]")];
    if (!nodes.length) return true;
    return nodes.some((node) => /Today\s+\d|\b(?:AM|PM)\b|[A-Za-z]{3}\s+\d{1,2},/.test(node.textContent || ""));
  }, { timeout: 8000 }).catch(() => {
    throw new Error(`${filename}: timestamps were not rewritten with a short zone`);
  });
  const png = await page.screenshot({ type: "png" });
  const file = resolve(outDir, filename);
  await sharp(png).png({ compressionLevel: 9, effort: 10 }).toFile(file);
  const meta = await sharp(file).metadata();
  if (meta.width !== 1280) throw new Error(`${filename} is ${meta.width}px wide`);
  const kb = Math.round(statSync(file).size / 1024);
  console.log(`${filename} ${meta.width}x${meta.height} ${kb}KB`);
}

async function assertCurrentOverview(page) {
  const report = await page.evaluate(() => {
    const nav = document.querySelector(".br img");
    const text = document.body.innerText;
    return {
      navSrc: nav ? nav.getAttribute("src") : "",
      activeLeases: text.includes("Active leases"),
      inFlight: text.includes("In flight"),
      freeAddresses: text.includes("Free addresses"),
      serverId: text.includes("Server ID"),
      lastUpdated: text.includes("Last Updated"),
    };
  });
  if (!report.navSrc.includes("flame-nav.png")) {
    throw new Error(`overview nav is not the current flame (${report.navSrc || "missing"})`);
  }
  if (!report.activeLeases || !report.inFlight || !report.freeAddresses) {
    throw new Error("overview is missing the six-cell strip");
  }
  if (report.serverId || report.lastUpdated) {
    throw new Error("overview still has the old status copy");
  }
}

async function settle(page) {
  await page.waitForNetworkIdle({ idleTime: 400, timeout: 8000 }).catch(() => {});
  await new Promise((r) => setTimeout(r, 250));
}

async function setMode(page, mode) {
  const saved = page.waitForResponse(
    (r) => r.url().includes("/api/preferences/mode") && r.request().method() === "POST",
    { timeout: 8000 },
  );
  await page.evaluate((next) => igniteSetMode(next), mode);
  const response = await saved;
  if (!response.ok()) throw new Error(`saving display mode ${mode} returned ${response.status()}`);
  await page.waitForFunction((next) => document.documentElement.getAttribute("data-theme") === next, {}, mode);
  await new Promise((r) => setTimeout(r, 200));
}

async function openProvisionFile(page) {
  await page.evaluate(() => {
    const item = document.querySelector(".file-item");
    if (item && typeof selectFile === "function") selectFile(item);
  });
  await page.waitForFunction(() => {
    const editor = document.getElementById("editor-textarea");
    return editor && editor.value.length > 0;
  });
}

async function selectHost(page) {
  const picked = await page.evaluate(() => {
    const theme = document.documentElement.getAttribute("data-theme");
    const sel = theme === "console" ? ".lease-rows .lease-pick" : ".cozy-view .lease-pick";
    const el = document.querySelector(sel);
    if (!el || typeof selectLease !== "function") return "";
    selectLease(el);
    const body = document.getElementById("lease-detail-body");
    return body && !body.hidden ? el.getAttribute("data-ip") || "selected" : "";
  });
  if (!picked) throw new Error("host detail did not open");
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
  await page.setCacheEnabled(false);
  page.setDefaultTimeout(20000);
  await page.goto(`${base}/login`, { waitUntil: "networkidle0" });
  await settle(page);
  await save(page, "login_page.png");
  await page.type("#username", "admin");
  await page.type("#password", "admin");
  await page.click('#login-form button[type="submit"]');
  await page.waitForFunction(() => location.pathname === "/");
  await settle(page);
  await assertCurrentOverview(page);

  await save(page, "main_page.png");
  await setMode(page, "console");
  await save(page, "console_page.png");

  const pages = [
    ["/dhcp", "dhcp_page.png", "dhcp_console.png"],
    ["/tftp", "tftp_page.png", "tftp_console.png"],
    ["/osimages", "osimage_page.png", "osimage_console.png"],
    ["/syslinux", "syslinux_page.png", "syslinux_console.png"],
    ["/provision", "provision_page.png", "provision_console.png"],
  ];
  for (const [path, cozyFile, consoleFile] of pages) {
    // DHCP draws both servers, their lease tables, and the detail panel.
    // A short viewport scrolls the second table under the status bar.
    if (path === "/dhcp") {
      await page.setViewport({ width: 1280, height: 1280, deviceScaleFactor: 1 });
    }
    // The download column is taller than an 800px stage once the catalog
    // status, version list, and end-of-life toggle are on the form.
    if (path === "/osimages") {
      await page.setViewport({ width: 1280, height: 900, deviceScaleFactor: 1 });
    }
    await page.goto(base + path, { waitUntil: "networkidle0" });
    await settle(page);
    // Mode is stored on the server, so a navigation keeps the last choice.
    await setMode(page, "cozy");
    if (path === "/provision") await openProvisionFile(page);
    await save(page, cozyFile);
    await setMode(page, "console");
    if (path === "/provision") {
      const stillOpen = await page.$eval("#editor-textarea", (el) => el.value.length > 0).catch(() => false);
      if (!stillOpen) await openProvisionFile(page);
    }
    await save(page, consoleFile);
    if (path === "/dhcp") {
      await setMode(page, "cozy");
      await selectHost(page);
      await save(page, "dhcp_detail.png");
      await setMode(page, "console");
      await selectHost(page);
      await save(page, "dhcp_detail_console.png");
      await page.setViewport({ width: 1280, height: 800, deviceScaleFactor: 1 });
    }
    if (path === "/osimages") {
      await page.setViewport({ width: 1280, height: 800, deviceScaleFactor: 1 });
    }
  }

  const client = await page.createCDPSession();
  await client.send("Network.clearBrowserCookies");
  await page.goto(`${base}/login`, { waitUntil: "networkidle0" });
  await settle(page);
  const loginTheme = await page.$eval("html", (el) => el.getAttribute("data-theme"));
  if (loginTheme !== "console") throw new Error(`console login rendered as ${loginTheme}`);
  await save(page, "login_console.png");
} finally {
  await browser.close();
}
