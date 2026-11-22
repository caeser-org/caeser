// Deterministic Playwright capture for the Ceaser product demo.
//
// Captures two short, scripted shots of a real PR against a real Ceaser
// investigation: the failed CI check, then the bot's investigation comment.
//
// Frames are rendered deterministically: for every output frame we compute the
// camera state for that exact virtual timestamp (scroll position + zoom scale
// with cubic easing), apply it, and screenshot. That yields true constant
// frame-rate output with perfectly smooth, reproducible camera moves and no
// dependence on screencast timing.
//
// Usage:
//   node capture.mjs --pr https://github.com/ceaser-org/ceaser-demo/pull/1
//   node capture.mjs --only checks|comment --fps 60
//
// Output: raw/<shot>/f-*.jpg and raw/<shot>/meta.json
//
// Nothing is authenticated: the target PR must be publicly readable so the
// recording never shows credentials.

import { chromium } from 'playwright';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const args = process.argv.slice(2);
const arg = (name, fallback) => {
  const i = args.indexOf(name);
  return i >= 0 ? args[i + 1] : fallback;
};

const PR = arg('--pr', 'https://github.com/ceaser-org/ceaser-demo/pull/1').replace(/\/+$/, '');
const OUT = path.resolve(here, arg('--out', 'raw'));
const ONLY = arg('--only', '');
const FPS = Number(arg('--fps', 60));
const QUALITY = Number(arg('--quality', 95));
const STATE_FILE = path.resolve(here, arg('--storage-state', '.auth/state.json'));
const W = 1920;
const H = 1080;

// Camera choreography. Coordinates are in document pixels for the real GitHub
// layout that scripts/inspect.mjs reports for the target PR (viewport 1920x1080).
const SHOTS = {
  checks: {
    kind: 'checks',
    url: `${PR}/checks`,
    origin: { x: -897, y: 276 },   // keeps the failed "test" block centred
    zoom: 1.5,
    timeline: { lead: 350, zoomIn: 1150, hold: 850 },
    probes: [
      ['failed-block', 'on: pull_request'],
      ['pr-title', 'cover late route registration'],
    ],
  },
  comment: {
    kind: 'comment',
    url: PR,
    scroll: 210,                   // document y to settle on before zooming
    origin: { x: 595, y: 1050 },   // keeps "tree.go:418 node.getValue" centred
    zoom: 1.65,
    timeline: { lead: 300, scroll: 1100, settle: 250, zoomIn: 1300, hold: 1600 },
    probes: [
      ['ceaser-heading', 'Ceaser diagnosis'],
      ['what-failed', 'What failed'],
      ['likely-fn-heading', 'Likely function to inspect'],
      ['tree-lead', 'tree\\.go:418'],
    ],
  },
};

const CLEAN_CSS = `
  html { scrollbar-width: none !important; }
  ::-webkit-scrollbar { display: none !important; }
`;

function totalMs(shot) {
  const t = shot.timeline;
  return t.lead + (t.scroll || 0) + (t.settle || 0) + t.zoomIn + t.hold;
}

async function waitForSettled(page) {
  await page.waitForLoadState('domcontentloaded');
  await page.waitForFunction(() => document.fonts && document.fonts.status === 'done', null, { timeout: 15000 }).catch(() => {});
  await page.waitForLoadState('networkidle', { timeout: 20000 }).catch(() => {});
  await page.waitForTimeout(1000);
}

// Installs the deterministic renderer used for every frame of a shot.
async function installRenderer(page, shot) {
  await page.evaluate((spec) => {
    const ease = (x) => (x < 0.5 ? 4 * x * x * x : 1 - Math.pow(-2 * x + 2, 3) / 2);
    const root = document.documentElement;
    const el = document.scrollingElement;
    root.style.transformOrigin = `${spec.origin.x}px ${spec.origin.y}px`;

    window.__renderAt = (t) => {
      const tl = spec.timeline;
      let scale = 1;
      let y = 0;
      if (spec.kind === 'checks') {
        if (t > tl.lead) scale = 1 + (spec.zoom - 1) * ease(Math.min(1, (t - tl.lead) / tl.zoomIn));
      } else {
        if (t > tl.lead) y = spec.scroll * ease(Math.min(1, (t - tl.lead) / tl.scroll));
        const zoomStart = tl.lead + tl.scroll + tl.settle;
        if (t > zoomStart) scale = 1 + (spec.zoom - 1) * ease(Math.min(1, (t - zoomStart) / tl.zoomIn));
      }
      el.scrollTop = y;
      root.style.transform = `scale(${scale})`;
      // Wait for the next paint so the screenshot reflects this exact state.
      return new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)));
    };
  }, shot);
}

async function verifyFraming(page, shot) {
  return page.evaluate((list) => {
    const find = (re) => {
      const rx = new RegExp(re, 'm');
      const els = [...document.querySelectorAll('span, pre, code, a, strong, h2, h3, div')]
        .filter((e) => rx.test(e.textContent || '') && (e.textContent || '').trim().length < 90);
      return els[els.length - 1];
    };
    const out = { scale: getComputedStyle(document.documentElement).transform, probes: {} };
    for (const [name, re] of list) {
      const e = find(re);
      if (!e) { out.probes[name] = null; continue; }
      const r = e.getBoundingClientRect();
      out.probes[name] = {
        x: Math.round(r.left + r.width / 2), y: Math.round(r.top + r.height / 2),
        visible: r.bottom > 0 && r.top < 1080 && r.right > 0 && r.left < 1920,
      };
    }
    out.viewportCentre = { x: 960, y: 540 };
    return out;
  }, shot.probes);
}

async function captureShot(browser, name, shot) {
  const dir = path.join(OUT, name);
  fs.rmSync(dir, { recursive: true, force: true });
  fs.mkdirSync(dir, { recursive: true });

  const context = await browser.newContext({
    viewport: { width: W, height: H },
    deviceScaleFactor: 1,
    colorScheme: 'dark',
    locale: 'en-US',
    timezoneId: 'America/Los_Angeles',
    reducedMotion: 'no-preference',
    permissions: [],
    ...(fs.existsSync(STATE_FILE) ? { storageState: STATE_FILE } : {}),
  });
  await context.addInitScript((css) => {
    const add = () => {
      const s = document.createElement('style');
      s.textContent = css;
      document.documentElement.appendChild(s);
      document.documentElement.setAttribute('data-color-mode', 'dark');
      document.documentElement.setAttribute('data-dark-theme', 'dark');
    };
    if (document.documentElement) add();
  }, CLEAN_CSS);

  const page = await context.newPage();
  await page.goto(shot.url, { waitUntil: 'domcontentloaded', timeout: 60000 });
  await waitForSettled(page);
  await installRenderer(page, shot);

  const total = totalMs(shot);
  const frames = Math.round((total / 1000) * FPS);
  const t0 = Date.now();
  for (let i = 0; i < frames; i++) {
    const t = (i / frames) * total;
    await page.evaluate((ms) => window.__renderAt(ms), t);
    await page.screenshot({ path: path.join(dir, `f-${String(i + 1).padStart(5, '0')}.jpg`), type: 'jpeg', quality: QUALITY });
  }
  const framing = await verifyFraming(page, shot).catch(() => null);
  fs.writeFileSync(path.join(dir, 'meta.json'), JSON.stringify({
    url: shot.url, fps: FPS, frames, duration: total / 1000,
    captureSeconds: Math.round((Date.now() - t0) / 100) / 10, framing,
  }, null, 2));
  console.log(`[${name}] ${frames} frames @ ${FPS}fps (${(total / 1000).toFixed(2)}s, captured in ${Math.round((Date.now() - t0) / 1000)}s)`);
  console.log(`[${name}] ${JSON.stringify(framing)}`);
  await context.close();
}

const browser = await chromium.launch({
  channel: 'chrome',
  headless: false,
  args: [
    '--no-first-run', '--no-default-browser-check', '--disable-notifications',
    '--disable-features=Translate,MediaRouter,OptimizationHints',
    '--force-color-profile=srgb', '--hide-scrollbars',
  ],
});

for (const [name, shot] of Object.entries(SHOTS)) {
  if (ONLY && ONLY !== name) continue;
  await captureShot(browser, name, shot);
}
await browser.close();
console.log('capture done');
