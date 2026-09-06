// Run against a local frontend; every API request is intercepted (no real data changes).
// npm install --prefix /tmp/ctflow-ui-check playwright
// /tmp/ctflow-ui-check/node_modules/.bin/playwright install chromium
// PLAYWRIGHT_MODULE=/tmp/ctflow-ui-check/node_modules/playwright/index.mjs node scripts/check-ui.mjs
import assert from 'node:assert/strict';
import { mkdir, readFile } from 'node:fs/promises';
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage({ viewport: { width: 1440, height: 1000 }, reducedMotion: 'reduce' });
const base = process.env.UI_BASE_URL || 'http://localhost:3100';
const output = process.env.UI_SCREENSHOTS || '/tmp/ctflow-ui-check/screenshots';
await mkdir(output, { recursive: true });
const errors = [];
page.on('pageerror', (error) => errors.push(error.message));
let signedIn = true;
let failProjects = false;
let exportFailure = 0;
let holdExport = false;
let releaseExport;
const exports = [];
const exportBodies = { yolo: Buffer.from('PK\x03\x04test-yolo'), coco: Buffer.from('{"images":[],"annotations":[],"categories":[]}'), voc: Buffer.from('PK\x03\x04test-voc') };
const owner = { oid: 'oid-1', username: 'Alex Chen' };
const teammate = { oid: 'oid-2', username: 'Nina Park' };
let projects = [
  ['Assembly line inspection', 'assembly-line', 128, 864, owner],
  ['Surface defect detection', 'surface-defects', 86, 240, teammate],
  ['Component quality control', 'components', 42, 0, owner],
  ['Packaging verification', 'packaging', 0, 0, null],
].map(([name, folder, labeled, auto, person], index) => ({ id: index + 1, name, input_dir: `/data/${folder}`, task_type: 'detection', owner: person, labeled, auto, contributors: person ? [{ ...person, boxes: 200 }] : [], created_at: '2026-09-01T12:00:00Z', updated_at: `2026-09-0${6 - index}T10:00:00Z` }));
const imagePath = '/data/assembly-line/frame-001.jpg';
const testImage = '/data/assembly-line/test-001.jpg';
const metric = { precision: 0.9, recall: 0.8, f1: 0.85, tp: 8, fp: 1, fn: 2 };
const evaluation = { overall: metric, per_class: { Component: metric }, per_image: [{ image: testImage, gt: [], pred: [], tp: 8, fp: 1, fn: 2 }], images: 1, iou: 0.5, conf: 0.25 };
const bank = { classes: [{ name: 'Component', count: 12 }], labeled: [], auto: [], model: 'yoloe' };
const imageSvg = '<svg xmlns="http://www.w3.org/2000/svg" width="960" height="540"><rect width="960" height="540" fill="#313c42"/><path d="M0 350H960M0 390H960M0 430H960" stroke="#63737c" stroke-width="10"/><rect x="200" y="170" width="180" height="180" rx="12" fill="#83928f"/><rect x="560" y="150" width="180" height="200" rx="12" fill="#a2ada4"/></svg>';
const writes = [];
await page.route('**/api/**', async (route) => {
  const req = route.request();
  const path = new URL(req.url()).pathname;
  const method = req.method();
  const data = req.postDataJSON();
  if (method !== 'GET') writes.push({ path, method, data });
  let result;
  if (path === '/api/export') {
    const params = Object.fromEntries(new URL(req.url()).searchParams);
    exports.push(params);
    if (holdExport) await new Promise((resolve) => { releaseExport = resolve; });
    if (exportFailure) return route.fulfill({ status: exportFailure, ...(exportFailure === 400 ? { json: { detail: "nothing to export for 'pool' -- label something first" } } : { contentType: 'text/plain', body: 'Upstream unavailable' }) });
    const filename = params.format === 'coco' ? 'annotations_coco.json' : `labels_${params.format}.zip`;
    return route.fulfill({ contentType: params.format === 'coco' ? 'application/json' : 'application/zip', headers: { 'Content-Disposition': `attachment; filename="${filename}"` }, body: exportBodies[params.format] });
  }
  if (path === '/api/auth/me') result = { enabled: true, user: signedIn ? owner.username : null, oid: signedIn ? owner.oid : null, mode: 'local' };
  else if (path === '/api/projects') {
    if (failProjects) return route.fulfill({ status: 503, json: { detail: 'Service unavailable. Try again.' } });
    if (method === 'POST') { const p = { ...projects[0], ...data, id: 5 }; projects.push(p); result = { project: p }; }
    else result = { projects };
  } else if (/^\/api\/projects\/\d+$/.test(path)) {
    const id = Number(path.split('/').pop());
    if (method === 'PATCH') projects = projects.map((p) => p.id === id ? { ...p, ...data } : p);
    if (method === 'DELETE') projects = projects.filter((p) => p.id !== id);
    result = { project: projects.find((p) => p.id === id), deleted: id };
  } else if (path === '/api/config') result = { colors: ['#0d9488'], roots: ['/data'], models: [{ id: 'yoloe', family: 'YOLOE', size: 's', note: 'Small', available: true }], default_model: 'yoloe' };
  else if (path === '/api/session') result = { images: [imagePath, testImage], bank, testset: { images: [testImage], labeled: ['test-001'], classes: ['Component'] }, bank_orphaned: false };
  else if (path === '/api/evaluate') result = { job_id: 'evaluation', total: 1 };
  else if (path === '/api/jobs/evaluation') result = { finished: true, done: 1, total: 1, started_at: 1, now: 2, result: evaluation };
  else if (path === '/api/state') result = { labeled: [], auto: [], testset_labeled: ['test-001'], claims: {} };
  else if (path === '/api/claim') result = { claims: {} };
  else if (path === '/api/boxes' || path === '/api/predict') result = { boxes: [], labeled_by: [] };
  else if (path === '/api/history') result = { history: [8, 12].map((n) => ({ ts: n * 1000, conf: 0.25, prompts: { Component: n }, totalPrompts: n, overall: metric, perClass: { Component: metric } })) };
  else if (path === '/api/pool') result = { total: 1, counts: { unlabeled: 1, labeled: 0, auto: 0, test: 0 }, items: [{ path: imagePath, status: 'unlabeled', held_by: null }] };
  else if (path === '/api/image' || path === '/api/thumb') return route.fulfill({ contentType: 'image/svg+xml', body: imageSvg });
  else return route.fulfill({ status: 404, json: { detail: `Unmocked route: ${path}` } });
  await route.fulfill({ json: result });
});
const visible = (role, name) => page.getByRole(role, { name, exact: true }).waitFor({ state: 'visible' });
const fits = async (label) => {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > innerWidth);
  if (overflow) {
    console.log(label, await page.evaluate(() => [...document.querySelectorAll("main *")].filter((el) => el.getBoundingClientRect().right > innerWidth + 1).map((el) => [el.tagName, el.className, el.getBoundingClientRect().right, el.textContent?.slice(0, 80)]).slice(0, 15)));
    await page.screenshot({ path: `${output}/overflow.png`, fullPage: true });
  }
  assert.equal(overflow, false, `${label}: horizontal overflow`);
};
try {
  await page.goto(base);
  await visible('heading', 'Assembly line inspection');
  assert.equal(await page.locator('.project-card').count(), 4);
  await page.screenshot({ path: `${output}/projects-desktop.png`, fullPage: true });
  await page.getByRole('button', { name: 'Owned by me', exact: true }).click();
  assert.equal(await page.locator('.project-card').count(), 2, 'Ownership uses oid, not display name');
  await page.getByRole('button', { name: 'Team projects', exact: true }).click();
  assert.equal(await page.locator('.project-card').count(), 2);
  await page.getByRole('button', { name: 'All projects', exact: true }).click();
  await page.getByRole('searchbox').fill('surface');
  assert.equal(await page.locator('.project-card').count(), 1);
  await page.getByRole('searchbox').fill('no-such-project');
  await visible('heading', 'No matching projects');
  await page.getByRole('button', { name: 'Clear filters' }).click();
  await page.getByLabel('Sort projects').selectOption('name');
  assert.match(await page.locator('.project-card h3').first().innerText(), /Assembly/);
  await page.getByRole('button', { name: 'New project', exact: true }).click();
  await visible('dialog', 'New project');
  await page.keyboard.press('Escape');
  await page.getByRole('dialog').waitFor({ state: 'hidden' });
  assert.equal(await page.getByRole('button', { name: 'New project', exact: true }).evaluate((el) => el === document.activeElement), true, 'Modal restores focus');
  await page.getByRole('button', { name: 'Rename', exact: true }).first().click();
  await page.getByRole('textbox', { name: 'Project name' }).fill('Renamed inspection');
  await page.getByRole('textbox', { name: 'Project name' }).press('Enter');
  await visible('heading', 'Renamed inspection');
  assert(writes.some((w) => w.method === 'PATCH' && w.data.name === 'Renamed inspection'));
  await page.getByRole('button', { name: 'Delete Packaging verification', exact: true }).click();
  await visible('alertdialog', 'Delete “Packaging verification”?');
  await page.getByRole('button', { name: 'Cancel', exact: true }).click();
  assert(!writes.some((w) => w.method === 'DELETE'), 'Cancel must not delete');
  await page.getByRole('button', { name: 'Use dark theme' }).click();
  await page.reload();
  await visible('heading', 'Projects');
  assert.equal(await page.locator('html').getAttribute('data-theme'), 'dark');
  await page.screenshot({ path: `${output}/projects-dark.png`, fullPage: true });
  await page.getByRole('button', { name: 'Use light theme' }).click();
  for (const theme of ['light', 'dark']) {
    const ratios = await page.evaluate((theme) => {
      document.documentElement.dataset.theme = theme;
      const probe = document.createElement('span'); probe.style.setProperty('transition', 'none', 'important'); document.body.append(probe);
      const luminance = (token) => {
        probe.style.color = `var(${token})`;
        const rgb = getComputedStyle(probe).color.match(/[\d.]+/g).slice(0, 3).map(Number).map((v) => v / 255).map((v) => v <= .04045 ? v / 12.92 : ((v + .055) / 1.055) ** 2.4);
        return rgb[0] * .2126 + rgb[1] * .7152 + rgb[2] * .0722;
      };
      const pairs = [['--text', '--surface'], ['--muted', '--surface-3'], ['--faint', '--surface-3'], ['--brand', '--brand-soft'], ['--on-primary', '--primary']];
      const result = pairs.map(([a, b]) => { const x = luminance(a), y = luminance(b); return [a, (Math.max(x, y) + .05) / (Math.min(x, y) + .05)]; });
      probe.remove(); return result;
    }, theme);
    for (const [token, ratio] of ratios) assert(ratio >= 4.5, `${theme} ${token}: contrast ${ratio}`);
  }
  await page.evaluate(() => { document.documentElement.dataset.theme = 'light'; });
  for (const width of [375, 667, 768, 1024, 1440]) {
    await page.setViewportSize({ width, height: width === 667 ? 375 : 1000 });
    await fits(`Projects ${width}`);
    if (width === 375) await page.screenshot({ path: `${output}/projects-mobile.png`, fullPage: true });
  }
  await page.getByRole('link', { name: 'Open project', exact: true }).first().click();
  await visible('navigation', 'Workflow');
  await page.locator('.canvas-wrap img').waitFor({ state: 'visible' });
  await page.screenshot({ path: `${output}/workspace-desktop.png`, fullPage: true });
  await page.getByRole('button', { name: 'Export dataset', exact: true }).click();
  const exportDialog = page.getByRole('dialog', { name: 'Export dataset', exact: true });
  await exportDialog.waitFor();
  assert.equal(await exportDialog.getByLabel('Data source').inputValue(), 'pool');
  await page.locator('dialog[open]').waitFor();
  await page.screenshot({ path: `${output}/export-desktop.png`, fullPage: true });
  await page.setViewportSize({ width: 375, height: 900 });
  await fits('Export dialog mobile');
  await page.screenshot({ path: `${output}/export-mobile.png`, fullPage: true });
  await page.setViewportSize({ width: 1440, height: 1000 });
  for (const kind of ['pool', 'testset']) {
    await exportDialog.getByLabel('Data source').selectOption(kind);
    for (const [format, name] of [['yolo', 'YOLO'], ['coco', 'COCO'], ['voc', 'Pascal VOC']]) {
      await exportDialog.getByRole('radio', { name: new RegExp(name) }).check();
      const downloading = page.waitForEvent('download');
      await exportDialog.getByRole('button', { name: 'Download annotations', exact: true }).click();
      const download = await downloading;
      assert.equal(download.suggestedFilename(), format === 'coco' ? 'annotations_coco.json' : `labels_${format}.zip`);
      assert.deepEqual(await readFile(await download.path()), exportBodies[format], 'Downloaded bytes are unchanged');
      assert.deepEqual(exports.at(-1), { input_dir: '/data/assembly-line', format, kind });
      await exportDialog.getByRole('status').filter({ hasText: 'Download started' }).waitFor();
    }
  }
  for (const status of [400, 503, 401]) {
    exportFailure = status;
    await exportDialog.getByRole('button', { name: /Download annotations|Try again/ }).click();
    await exportDialog.getByRole('alert').waitFor();
    assert.match(await exportDialog.getByRole('alert').innerText(), status === 400 ? /label something first/ : status === 401 ? /session has expired/ : /Export failed/);
  }
  exportFailure = 0;
  holdExport = true;
  await exportDialog.getByRole('button', { name: 'Try again' }).click();
  await exportDialog.getByRole('button', { name: 'Preparing…' }).waitFor();
  assert(await exportDialog.getByRole('button', { name: 'Preparing…' }).isDisabled());
  assert(releaseExport, 'Export request is pending');
  let canceledDownloads = 0;
  const unexpectedDownload = () => { canceledDownloads++; };
  page.on('download', unexpectedDownload);
  await exportDialog.getByRole('button', { name: 'Cancel export' }).click();
  await exportDialog.waitFor({ state: 'hidden' });
  holdExport = false;
  releaseExport();
  await page.getByRole('button', { name: 'Export dataset', exact: true }).click();
  await exportDialog.waitFor();
  await page.keyboard.press('Escape');
  await exportDialog.waitFor({ state: 'hidden' });
  assert.equal(canceledDownloads, 0, 'Canceled export must not download');
  page.off('download', unexpectedDownload);
  const canvas = await page.locator('.canvas-wrap').first().boundingBox();
  await page.mouse.move(canvas.x + 30, canvas.y + 30);
  await page.mouse.down();
  await page.mouse.move(canvas.x + 120, canvas.y + 100);
  await page.mouse.up();
  await page.getByText('1 box', { exact: true }).waitFor();
  await page.getByRole('button', { name: 'Export dataset', exact: true }).click();
  await exportDialog.getByText(/You have unsaved edits/).waitFor();
  await page.keyboard.press('Escape');
  await exportDialog.waitFor({ state: 'hidden' });
  await page.getByText('1 box', { exact: true }).waitFor();
  await page.getByRole('button', { name: 'Undo (Ctrl+Z)', exact: true }).click();
  await page.getByRole('button', { name: 'Evaluate', exact: true }).click();
  await page.getByRole('navigation', { name: 'Workflow' }).getByRole('button', { name: /^Report/ }).waitFor();
  await page.waitForFunction(() => ![...document.querySelectorAll('.nav-item')].find((el) => el.textContent.startsWith('Report'))?.disabled);
  for (const name of ['Gallery', 'Test set', 'Report', 'Progress', 'Label']) {
    await page.getByRole('navigation', { name: 'Workflow' }).getByRole('button', { name: new RegExp(`^${name}`) }).click();
    if (name === 'Test set') {
      await page.getByRole('button', { name: 'Export dataset', exact: true }).click();
      await exportDialog.waitFor();
      assert.equal(await exportDialog.getByLabel('Data source').inputValue(), 'testset');
      await page.keyboard.press('Escape');
      await exportDialog.waitFor({ state: 'hidden' });
    }
    for (const width of [375, 667, 1440]) {
      await page.setViewportSize({ width, height: width === 667 ? 375 : 1000 });
      await fits(`${name} ${width}`);
    }
  }
  await page.setViewportSize({ width: 375, height: 900 });
  await page.screenshot({ path: `${output}/workspace-mobile.png`, fullPage: true });
  await page.getByRole('button', { name: /Keyboard shortcuts, single-key/ }).click();
  await page.getByRole('dialog').waitFor();
  await fits('Shortcuts dialog mobile');
  await page.keyboard.press('Escape');
  await page.goto(base);
  failProjects = true;
  await page.reload();
  await page.getByRole('alert').filter({ hasText: 'Service unavailable' }).waitFor();
  failProjects = false;
  await page.getByRole('button', { name: 'Try again' }).click();
  await page.locator('.project-card').first().waitFor();
  signedIn = false;
  await page.goto(`${base}/entry/login`);
  await visible('textbox', 'Username');
  await fits('Login mobile');
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.screenshot({ path: `${output}/login-desktop.png`, fullPage: true });
  assert.deepEqual(errors, [], 'No browser runtime errors');
  console.log(`UI checks passed. Screenshots: ${output}`);
} finally { await browser.close(); }
