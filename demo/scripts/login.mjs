// Create a reusable Playwright session for private repositories.
//
// Only needed when the target PR is not publicly readable. Opens a headed
// browser, waits for you to sign in to GitHub, then saves the session to
// .auth/state.json (git-ignored). capture.mjs picks it up automatically.
//
// Usage: node scripts/login.mjs [--out .auth/state.json]

import { chromium } from 'playwright';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const args = process.argv.slice(2);
const outIdx = args.indexOf('--out');
const out = outIdx >= 0 ? path.resolve(args[outIdx + 1]) : path.resolve(here, '..', '.auth', 'state.json');

const browser = await chromium.launch({ channel: 'chrome', headless: false, args: ['--no-first-run'] });
const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
const page = await context.newPage();
await page.goto('https://github.com/login', { waitUntil: 'domcontentloaded' });
console.log('Sign in to GitHub in the opened window. Waiting for the session…');

await page.waitForFunction(
  () => !!document.querySelector('meta[name="user-login"]')?.content ||
        !!document.querySelector('summary[aria-label*="View profile"], [data-login]'),
  null, { timeout: 0, polling: 1000 },
);

fs.mkdirSync(path.dirname(out), { recursive: true });
await context.storageState({ path: out });
console.log(`Session saved to ${out}`);
await browser.close();
