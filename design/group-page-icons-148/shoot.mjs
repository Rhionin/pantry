import { spawn } from 'node:child_process';
import { mkdir, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = dirname(fileURLToPath(import.meta.url));
const outDir = resolve(root, 'screenshots');
const pageUrl = `file://${resolve(root, 'index.html')}`;
const port = 9333;

function words(text) {
  return text
    .replace(/\s+/g, ' ')
    .trim()
    .split(' ')
    .filter((token) => /[0-9A-Za-z]/.test(token));
}

const chrome = spawn('google-chrome', [
  '--headless=new',
  '--disable-gpu',
  '--hide-scrollbars',
  '--no-sandbox',
  '--disable-dev-shm-usage',
  '--no-first-run',
  '--no-default-browser-check',
  `--remote-debugging-port=${port}`,
  '--user-data-dir=/tmp/chrome-group-mockups-148',
  'about:blank',
], { stdio: 'ignore' });

function sleep(ms) {
  return new Promise((resolveSleep) => setTimeout(resolveSleep, ms));
}

async function debuggerUrl() {
  for (let attempt = 0; attempt < 40; attempt += 1) {
    try {
      const response = await fetch(`http://127.0.0.1:${port}/json/version`);
      if (response.ok) return (await response.json()).webSocketDebuggerUrl;
    } catch {
      // Chrome is still starting.
    }
    await sleep(150);
  }
  throw new Error('Chrome did not open a debugging port');
}

function cdp(ws) {
  let id = 0;
  const pending = new Map();
  ws.addEventListener('message', (event) => {
    const message = JSON.parse(event.data);
    if (message.id && pending.has(message.id)) {
      const { resolve: done, reject } = pending.get(message.id);
      pending.delete(message.id);
      if (message.error) reject(new Error(JSON.stringify(message.error)));
      else done(message.result);
    }
  });
  return (method, params = {}) => new Promise((done, reject) => {
    const next = ++id;
    pending.set(next, { resolve: done, reject });
    ws.send(JSON.stringify({ id: next, method, params }));
  });
}

let browser;
let page;
try {
const browserSocket = new WebSocket(await debuggerUrl());
browser = browserSocket;
await new Promise((resolveOpen) => browser.addEventListener('open', resolveOpen));
const send = cdp(browser);
const { targetId } = await send('Target.createTarget', { url: pageUrl });
const { webSocketDebuggerUrl } = (await (await fetch(`http://127.0.0.1:${port}/json/list`)).json())
  .find((target) => target.id === targetId);
page = new WebSocket(webSocketDebuggerUrl);
await new Promise((resolveOpen) => page.addEventListener('open', resolveOpen));
const call = cdp(page);
await call('Page.enable');
await call('Runtime.enable');
await sleep(400);

const counted = await call('Runtime.evaluate', {
  expression: `(() => {
    const words = (text) => text.replace(/\\s+/g, ' ').trim().split(' ').filter((token) => /[0-9A-Za-z]/.test(token));
    const details = [...document.querySelectorAll('[data-count="detail"]')].map((node) => ({
      label: node.dataset.label,
      count: words(node.innerText).length,
      text: node.innerText.replace(/\\s+/g, ' ').trim(),
    }));
    const lists = [...document.querySelectorAll('[data-count="rows"]')].map((node) => {
      const rows = [...node.querySelectorAll('[data-count="row"]')].map((row) => ({
        count: words(row.innerText).length,
        text: row.innerText.replace(/\\s+/g, ' ').trim(),
      }));
      return { label: node.dataset.label, count: rows.reduce((sum, row) => sum + row.count, 0), rows };
    });
    return { details, lists };
  })()`,
  returnByValue: true,
});

await mkdir(outDir, { recursive: true });
const shots = await call('Runtime.evaluate', {
  expression: `(() => [...document.querySelectorAll('[data-shot]')].map((node) => {
    const box = node.getBoundingClientRect();
    return { name: node.dataset.shot, x: box.x, y: box.y, width: box.width, height: box.height };
  }))()`,
  returnByValue: true,
});

for (const shot of shots.result.value) {
  const { data } = await call('Page.captureScreenshot', {
    format: 'png',
    captureBeyondViewport: true,
    clip: {
      x: shot.x,
      y: shot.y,
      width: shot.width,
      height: shot.height,
      scale: 1,
    },
  });
  await writeFile(resolve(outDir, `${shot.name}.png`), Buffer.from(data, 'base64'));
  console.log(`${shot.name} ${Math.round(shot.width)}x${Math.round(shot.height)}`);
}

await writeFile(resolve(root, 'wordcount.json'), JSON.stringify(counted.result.value, null, 2));
console.log(JSON.stringify(counted.result.value, null, 2));

} finally {
  browser?.close();
  page?.close();
  chrome.kill();
}
