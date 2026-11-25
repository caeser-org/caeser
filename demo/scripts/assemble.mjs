// Assemble the captured frames into the demo deliverables with ffmpeg.
//
// Frames are captured at a constant rate by capture.mjs, so each shot is a
// plain image sequence. The two shots are concatenated (hard cut) and encoded
// to WebM, MP4, an optional GIF, and a WebP poster.
//
// Usage: node scripts/assemble.mjs [--fps 60] [--crf-webm 33] [--crf-mp4 20]

import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '..');
const rawDir = path.join(root, 'raw');
const outDir = path.join(root, 'out');

const args = process.argv.slice(2);
const arg = (n, d) => { const i = args.indexOf(n); return i >= 0 ? args[i + 1] : d; };
const ORDER = ['checks', 'comment'];
const FPS = Number(arg('--fps', '0')) || JSON.parse(fs.readFileSync(path.join(rawDir, ORDER[0], 'meta.json'), 'utf8')).fps;
const CRF_WEBM = arg('--crf-webm', '36');
const CRF_MP4 = arg('--crf-mp4', '20');

fs.mkdirSync(outDir, { recursive: true });

const inputs = [];
const filter = [];
ORDER.forEach((name, i) => {
  const dir = path.join(rawDir, name);
  const meta = path.join(dir, 'meta.json');
  if (!fs.existsSync(meta)) throw new Error(`missing ${meta}; run capture.mjs first`);
  inputs.push('-framerate', String(FPS), '-i', path.join(dir, 'f-%05d.jpg'));
  filter.push(`[${i}:v]`);
});
const lastDir = path.join(rawDir, ORDER[ORDER.length - 1]);
const lastFile = fs.readdirSync(lastDir).filter((f) => /^f-\d+\.jpg$/.test(f)).sort().pop();

const vf = `scale=1920:1080:flags=lanczos,setsar=1,fps=${FPS},format=yuv420p`;
const fc = `${filter.join('')}concat=n=${ORDER.length}:v=1:a=0[v]`;

const run = (a) => execFileSync('ffmpeg', a, { stdio: ['ignore', 'inherit', 'inherit'] });
const common = ['-y', ...inputs, '-filter_complex', `${fc};[v]${vf}[vo]`, '-map', '[vo]', '-an'];

// MP4 (H.264) — broad compatibility.
run([...common, '-c:v', 'libx264', '-preset', 'slow', '-crf', CRF_MP4,
  '-profile:v', 'high', '-level', '4.1', '-movflags', '+faststart',
  path.join(outDir, 'ceaser-demo.mp4')]);

// WebM (VP9) — optimised for the web, target < 3 MB.
run([...common, '-c:v', 'libvpx-vp9', '-b:v', '0', '-crf', CRF_WEBM,
  '-row-mt', '1', '-cpu-used', '2', '-tile-columns', '2', '-frame-parallel', '1',
  path.join(outDir, 'ceaser-demo.webm')]);

// Poster — the final held frame (the most compelling evidence).
// This ffmpeg build has no webp encoder, so use cwebp (libwebp) instead.
try {
  execFileSync('cwebp', ['-quiet', '-q', '82', path.join(lastDir, lastFile),
    '-o', path.join(outDir, 'ceaser-demo-poster.webp')], { stdio: ['ignore', 'inherit', 'inherit'] });
} catch {
  // Fallback for machines without cwebp.
  execFileSync('ffmpeg', ['-y', '-i', path.join(lastDir, lastFile), '-frames:v', '1',
    path.join(outDir, 'ceaser-demo-poster.png')], { stdio: ['ignore', 'inherit', 'inherit'] });
  console.warn('  cwebp unavailable; wrote ceaser-demo-poster.png instead');
}

// GIF — small sharing asset.
run(['-y', ...inputs, '-filter_complex',
  `${fc};[v]fps=12,scale=880:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=160[p];[b][p]paletteuse=dither=bayer[g]`,
  '-map', '[g]', '-loop', '0', path.join(outDir, 'ceaser-demo.gif')]);

console.log(`fps=${FPS}`);
for (const f of ['ceaser-demo.webm', 'ceaser-demo.mp4', 'ceaser-demo.gif', 'ceaser-demo-poster.webp']) {
  const p = path.join(outDir, f);
  console.log(`  ${f.padEnd(30)} ${fs.existsSync(p) ? Math.round(fs.statSync(p).size / 1024) : -1} KB`);
}
console.log('assemble done');
