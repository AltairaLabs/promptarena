// Minimal Playwright screenshot helper for visual verification.
// Usage: node docs/scripts/shot.mjs <url> <name>
// Writes /tmp/<name>-dark.png and /tmp/<name>-light.png.
import { chromium } from 'playwright';

const [url, name] = process.argv.slice(2);
if (!url || !name) {
  console.error('usage: node shot.mjs <url> <name>');
  process.exit(1);
}

const b = await chromium.launch();
for (const theme of ['dark', 'light']) {
  const p = await b.newPage({ viewport: { width: 1280, height: 900 } });
  await p.goto(url, { waitUntil: 'networkidle' });
  await p.evaluate((t) => {
    document.documentElement.dataset.theme = t;
    try { localStorage.setItem('starlight-theme', t); } catch {}
  }, theme);
  await p.waitForTimeout(150);
  await p.screenshot({ path: `/tmp/${name}-${theme}.png`, fullPage: true });
  await p.close();
}
await b.close();
console.log(`shots written: /tmp/${name}-dark.png /tmp/${name}-light.png`);
