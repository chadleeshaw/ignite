// Crop the original Ignite flame and write the small brand assets.
//
// Source: assets/brand/Ignite_small.png (the illustrated multi-tongue flame).
// The faint circle and swirl lines are dropped. The flame and the solid
// sparks stay. Nothing written here is the 1.3 MB original.
//
//   node scripts/build-flame.mjs
//
// Requires sharp (scripts/package.json).

import { mkdir, stat, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import sharp from "sharp";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const source = path.join(root, "assets/brand/Ignite_small.png");
const outDir = path.join(root, "public/http/img");
const MAX_BYTES = 30 * 1024;

const NAV_H = 32;
const NAV_2X = 64;

async function loadMask() {
  const { data, info } = await sharp(source).ensureAlpha().raw().toBuffer({ resolveWithObject: true });
  const w = info.width;
  const h = info.height;
  const seen = new Uint8Array(w * h);
  const keep = [];
  const at = (x, y) => y * w + x;
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const p = at(x, y);
      if (seen[p]) continue;
      const a = data[p * 4 + 3];
      const r = data[p * 4];
      if (a < 100 || r < 100) {
        seen[p] = 1;
        continue;
      }
      let n = 0;
      let sumA = 0;
      let minX = x;
      let maxX = x;
      let minY = y;
      let maxY = y;
      const stack = [[x, y]];
      seen[p] = 1;
      while (stack.length) {
        const [cx, cy] = stack.pop();
        n++;
        sumA += data[at(cx, cy) * 4 + 3];
        if (cx < minX) minX = cx;
        if (cx > maxX) maxX = cx;
        if (cy < minY) minY = cy;
        if (cy > maxY) maxY = cy;
        for (const [nx, ny] of [[cx + 1, cy], [cx - 1, cy], [cx, cy + 1], [cx, cy - 1]]) {
          if (nx < 0 || ny < 0 || nx >= w || ny >= h) continue;
          const q = at(nx, ny);
          if (seen[q]) continue;
          const qa = data[q * 4 + 3];
          const qr = data[q * 4];
          if (qa < 100 || qr < 100) {
            seen[q] = 1;
            continue;
          }
          seen[q] = 1;
          stack.push([nx, ny]);
        }
      }
      const avg = sumA / n;
      const bw = maxX - minX + 1;
      const bh = maxY - minY + 1;
      const aspect = bw / bh;
      // Solid sparks and tongues. Thin, faint swirls stay out.
      const solid = avg >= 200 && n >= 40;
      const spark = avg >= 210 && n >= 18 && aspect > 0.25 && aspect < 4;
      if (solid || spark) keep.push({ minX, minY, maxX, maxY });
    }
  }
  if (!keep.length) throw new Error("no flame pixels found");
  let minX = w;
  let minY = h;
  let maxX = 0;
  let maxY = 0;
  for (const c of keep) {
    minX = Math.min(minX, c.minX);
    minY = Math.min(minY, c.minY);
    maxX = Math.max(maxX, c.maxX);
    maxY = Math.max(maxY, c.maxY);
  }
  const pad = 18;
  minX = Math.max(0, minX - pad);
  minY = Math.max(0, minY - pad);
  maxX = Math.min(w - 1, maxX + pad);
  maxY = Math.min(h - 1, maxY + pad);
  return { left: minX, top: minY, width: maxX - minX + 1, height: maxY - minY + 1 };
}

function squareContain(size) {
  return async (cropped) => {
    const meta = await cropped.metadata();
    const scale = size / Math.max(meta.width, meta.height);
    const w = Math.max(1, Math.round(meta.width * scale));
    const h = Math.max(1, Math.round(meta.height * scale));
    const resized = await cropped.clone().resize(w, h, { fit: "fill", kernel: "lanczos3" }).png().toBuffer();
    return sharp({
      create: { width: size, height: size, channels: 4, background: { r: 0, g: 0, b: 0, alpha: 0 } },
    }).composite([{ input: resized, gravity: "centre" }]);
  };
}

const FLAME_PALETTE = [
  [252, 38, 0], [254, 76, 0], [254, 94, 0], [255, 116, 0], [255, 142, 0], [255, 180, 1],
  [255, 196, 40], [255, 220, 80], [255, 244, 194], [230, 40, 0], [200, 20, 0], [255, 90, 20],
  [255, 150, 30], [180, 30, 0], [255, 210, 120],
];

function nearestFlame(r, g, b) {
  let best = FLAME_PALETTE[0];
  let bestD = Infinity;
  for (const p of FLAME_PALETTE) {
    const d = (r - p[0]) ** 2 + (g - p[1]) ** 2 + (b - p[2]) ** 2;
    if (d < bestD) {
      bestD = d;
      best = p;
    }
  }
  return best;
}

// The 512px canvas does not fit under 30KB with a full gradient. Snap it to
// the same red-to-gold colors so the PNG stays a small indexed image.
async function quantizeFlame(pipeline) {
  const { data, info } = await pipeline.clone().ensureAlpha().raw().toBuffer({ resolveWithObject: true });
  for (let i = 0; i < data.length; i += 4) {
    const a = data[i + 3];
    if (a < 32) {
      data[i] = data[i + 1] = data[i + 2] = data[i + 3] = 0;
      continue;
    }
    const [r, g, b] = nearestFlame(data[i], data[i + 1], data[i + 2]);
    data[i] = r;
    data[i + 1] = g;
    data[i + 2] = b;
    data[i + 3] = a < 120 ? 96 : a < 200 ? 180 : 255;
  }
  return sharp(data, { raw: info });
}

async function writePNG(pipeline, file, colors) {
  let buf = await pipeline.clone().png({
    compressionLevel: 9,
    palette: true,
    quality: 90,
    effort: 10,
    colors,
  }).toBuffer();
  if (buf.length >= MAX_BYTES) {
    const reduced = await quantizeFlame(pipeline);
    buf = await reduced.png({
      compressionLevel: 9,
      palette: true,
      quality: 80,
      effort: 10,
      colors: 48,
    }).toBuffer();
  }
  if (buf.length >= MAX_BYTES) {
    throw new Error(`${file} is ${buf.length} bytes`);
  }
  await writeFile(file, buf);
  return buf.length;
}

async function writeWebP(pipeline, file, quality, alphaQuality = 90) {
  let buf = await pipeline.clone().webp({ quality, alphaQuality, effort: 6 }).toBuffer();
  if (buf.length >= MAX_BYTES) {
    const reduced = await quantizeFlame(pipeline);
    buf = await reduced.webp({ quality: Math.min(quality, 60), alphaQuality: 70, effort: 6 }).toBuffer();
  }
  if (buf.length >= MAX_BYTES) {
    throw new Error(`${file} is ${buf.length} bytes`);
  }
  await writeFile(file, buf);
  return buf.length;
}

const crop = await loadMask();
console.log("crop", crop);
const cropped = sharp(source).extract(crop);
await mkdir(outDir, { recursive: true });

const nav1 = cropped.clone().resize({ height: NAV_H, kernel: "lanczos3" });
const nav2 = cropped.clone().resize({ height: NAV_2X, kernel: "lanczos3" });
const navMeta = await sharp(path.join(outDir, "flame-nav.png")).metadata();

const jobs = [
  ["flame-nav.png", () => writePNG(nav1, path.join(outDir, "flame-nav.png"), 48)],
  ["flame-nav.webp", () => writeWebP(nav1, path.join(outDir, "flame-nav.webp"), 78)],
  ["flame-nav@2x.png", () => writePNG(nav2, path.join(outDir, "flame-nav@2x.png"), 64)],
  ["flame-nav@2x.webp", () => writeWebP(nav2, path.join(outDir, "flame-nav@2x.webp"), 78)],
  ["favicon-32.png", async () => writePNG(await squareContain(32)(cropped.clone()), path.join(outDir, "favicon-32.png"), 48)],
  ["apple-touch-icon.png", async () => writePNG(await squareContain(180)(cropped.clone()), path.join(outDir, "apple-touch-icon.png"), 64)],
  ["flame-512.png", async () => writePNG(await squareContain(512)(cropped.clone()), path.join(outDir, "flame-512.png"), 96)],
  ["flame-512.webp", async () => writeWebP(await squareContain(512)(cropped.clone()), path.join(outDir, "flame-512.webp"), 32, 20)],
];

for (const [name, run] of jobs) {
  const bytes = await run();
  const info = await stat(path.join(outDir, name));
  console.log(`${name}\t${info.size}\t(${bytes})`);
}

console.log(`nav file ${navMeta.width}x${navMeta.height}`);

// Review sheet: 28px and 64px on the navy nav color.
const navy = { r: 11, g: 16, b: 38, alpha: 1 };
async function onNavy(height) {
  const mark = await cropped.clone().resize({ height, kernel: "lanczos3" }).png().toBuffer();
  const meta = await sharp(mark).metadata();
  return sharp({
    create: { width: meta.width + 24, height: meta.height + 16, channels: 4, background: navy },
  }).composite([{ input: mark, left: 12, top: 8 }]).png().toBuffer();
}
const a = await onNavy(28);
const b = await onNavy(32);
const c = await onNavy(64);
const sheet = await sharp({
  create: { width: 280, height: 160, channels: 4, background: navy },
})
  .composite([
    { input: a, left: 16, top: 16 },
    { input: b, left: 80, top: 16 },
    { input: c, left: 150, top: 16 },
  ])
  .png()
  .toFile("/tmp/flame-nav-preview.png");
console.log("preview", sheet);
