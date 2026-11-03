// Loads puppeteer for the Chrome tests and the cosmetic-filter check: from this
// project's own node_modules (npm install), else from a neighbouring project
// that has it (inside the Nexodyne repository: nexodyne-server).
import { createRequire } from 'node:module';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const KG = join(dirname(fileURLToPath(import.meta.url)), '..');

export function loadPuppeteer() {
  for (const base of [join(KG, 'package.json'), join(KG, '..', 'nexodyne-server', 'package.json')]) {
    try { return createRequire(base)('puppeteer'); } catch { /* try the next place */ }
  }
  throw new Error('puppeteer is missing: run npm install in this folder');
}
