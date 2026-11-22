// Geometry helper for retuning the camera when Ceaser's comment changes.
//
// Opens a PR (or /checks) page at 1920x1080 in dark mode and prints the
// document coordinates of the headings and code blocks the recorder targets,
// so you can update the `origin` / `scroll` values in capture.mjs.
//
// Usage: node scripts/inspect.mjs [url]

import { chromium } from 'playwright';

const url = process.argv[2] || 'https://github.com/ceaser-org/ceaser-demo/pull/1';
const ctx = await chromium.launchPersistentContext('/tmp/ceaser-inspect', {
  channel: 'chrome', headless: false,
  viewport: { width: 1920, height: 1080 }, colorScheme: 'dark',
  args: ['--no-first-run', '--no-default-browser-check'],
});
const page = ctx.pages()[0] || await ctx.newPage();
await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 60000 });
await page.waitForTimeout(4000);

const data = await page.evaluate(() => {
  const rows = [];
  const rx = /Ceaser diagnosis|What failed|Likely function|tree\.go:|getValue|on: pull_request|in 1m|1 error/i;
  for (const e of document.querySelectorAll('span, pre, code, a, strong, h2, h3, div')) {
    const t = (e.textContent || '').replace(/\s+/g, ' ').trim();
    if (!t || t.length > 70 || !rx.test(t)) continue;
    const r = e.getBoundingClientRect();
    if (!r.width || !r.height) continue;
    rows.push({ text: t.slice(0, 55), docX: Math.round(r.left), docY: Math.round(r.top + scrollY), w: Math.round(r.width), h: Math.round(r.height) });
  }
  return { bodyHeight: document.body.scrollHeight, rows };
});
console.log('bodyHeight', data.bodyHeight);
for (const r of data.rows) console.log(String(r.docY).padStart(6), String(r.docX).padStart(5), `${r.w}x${r.h}`.padEnd(11), r.text);
await ctx.close();
