// Capture high-resolution full-page screenshots for the Remotion composition.
//
// Remotion does the scroll/zoom itself, so these are flat, un-cropped pages at
// 2x device scale (3840 px wide) to stay sharp when the camera zooms in.
//
// Output: remotion/public/<name>.png

import { chromium } from 'playwright';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const outDir = path.resolve(here, '..', 'remotion', 'public');
fs.mkdirSync(outDir, { recursive: true });

const PR = process.env.PR_URL || 'https://github.com/ceaser-org/ceaser-demo/pull/1';
const pages = [
  ['ceaser-checks', `${PR}/checks`],
  ['ceaser-comment', PR],
];

const browser = await chromium.launch({
  channel: 'chrome', headless: false,
  args: ['--no-first-run', '--no-default-browser-check', '--hide-scrollbars', '--force-color-profile=srgb'],
});
const context = await browser.newContext({
  viewport: { width: 1920, height: 1080 },
  deviceScaleFactor: 2,
  colorScheme: 'dark',
  locale: 'en-US',
});
for (const [name, url] of pages) {
  const page = await context.newPage();
  await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 60000 });
  await page.waitForFunction(() => document.fonts && document.fonts.status === 'done', null, { timeout: 15000 }).catch(() => {});
  await page.waitForLoadState('networkidle', { timeout: 20000 }).catch(() => {});
  await page.waitForTimeout(1200);
  await page.evaluate(() => {
    document.documentElement.setAttribute('data-color-mode', 'dark');
    const s = document.createElement('style');
    s.textContent = 'html{scrollbar-width:none!important}::-webkit-scrollbar{display:none!important}';
    document.documentElement.appendChild(s);
  });
  const file = path.join(outDir, `${name}.png`);
  await page.screenshot({ path: file, fullPage: true, type: 'png' });
  const size = await page.evaluate(() => ({ w: document.body.scrollWidth, h: document.body.scrollHeight }));
  console.log(`${name}.png  css ${size.w}x${size.h}`);
  await page.close();
}
await browser.close();
console.log('screenshots done ->', outDir);
