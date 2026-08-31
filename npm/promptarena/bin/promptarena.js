#!/usr/bin/env node

import { spawn } from 'node:child_process';
import { createRequire } from 'node:module';
import { resolveBinary } from '../lib/resolve.js';

const require = createRequire(import.meta.url);

let binaryPath;
try {
  // Bind through an arrow so require.resolve keeps its receiver.
  binaryPath = resolveBinary('promptarena', {
    resolve: (id) => require.resolve(id),
  });
} catch (err) {
  console.error(`Error: ${err.message}`);
  process.exit(1);
}

const child = spawn(binaryPath, process.argv.slice(2), {
  stdio: 'inherit',
  windowsHide: false,
});

child.on('error', (err) => {
  console.error('Failed to start promptarena:', err.message);
  process.exit(1);
});

child.on('exit', (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
  } else {
    process.exit(code || 0);
  }
});
