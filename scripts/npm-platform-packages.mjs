#!/usr/bin/env node
//
// Build the per-platform npm packages that carry the Go binaries.
//
// The main packages (@altairalabs/promptarena, @altairalabs/packc) declare
// these as optionalDependencies. npm/pnpm/yarn/bun pick the one matching the
// host via its `os`/`cpu` fields and unpack only that one — no lifecycle
// script runs, which is the whole point: npm 12 blocks dependency install
// scripts by default, so the old `postinstall` downloader never fired.
//
// Input is the set of archives goreleaser already published, so the bytes we
// ship on npm are the same bytes on the GitHub release and in the Homebrew
// cask.
//
// Usage:
//   node scripts/npm-platform-packages.mjs \
//     --version 1.7.0 --archives ./release-archives --out ./dist/npm
//
//   # then, publishing platform packages BEFORE the main package:
//   for d in dist/npm/*/; do npm publish "$d" --access public; done

import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';

const TOOLS = ['promptarena', 'packc'];

// npm target -> the {Os}_{Arch} fragment goreleaser puts in archive names
// (see the archives.name_template in .goreleaser.yml).
const TARGETS = {
  'darwin-arm64': { os: 'darwin', cpu: 'arm64', goos: 'Darwin', goarch: 'arm64' },
  'darwin-x64': { os: 'darwin', cpu: 'x64', goos: 'Darwin', goarch: 'x86_64' },
  'linux-arm64': { os: 'linux', cpu: 'arm64', goos: 'Linux', goarch: 'arm64' },
  'linux-x64': { os: 'linux', cpu: 'x64', goos: 'Linux', goarch: 'x86_64' },
  'win32-arm64': { os: 'win32', cpu: 'arm64', goos: 'Windows', goarch: 'arm64' },
  'win32-x64': { os: 'win32', cpu: 'x64', goos: 'Windows', goarch: 'x86_64' },
};

function parseArgs(argv) {
  const args = {};
  for (let i = 0; i < argv.length; i += 2) {
    const key = argv[i].replace(/^--/, '');
    args[key] = argv[i + 1];
  }
  for (const required of ['version', 'archives', 'out']) {
    if (!args[required]) {
      console.error(`Missing --${required}`);
      console.error(
        'Usage: npm-platform-packages.mjs --version X.Y.Z --archives <dir> --out <dir>'
      );
      process.exit(1);
    }
  }
  args.version = args.version.replace(/^v/, '');
  return args;
}

// Don't assume an extension per OS. The old postinstall downloader hardcoded
// `.zip` for Windows, but .goreleaser.yml sets no format_overrides, so
// goreleaser emits tar.gz for every target — that guess 404'd on every Windows
// install. Probe what is actually on disk instead.
function findArchive(dir, tool, version, target) {
  const { goos, goarch } = TARGETS[target];
  const stem = `${tool}_${version}_${goos}_${goarch}`;
  for (const ext of ['tar.gz', 'zip']) {
    const candidate = path.join(dir, `${stem}.${ext}`);
    if (fs.existsSync(candidate)) return candidate;
  }
  throw new Error(`Missing release archive for ${target}: ${path.join(dir, stem)}.{tar.gz,zip}`);
}

function extractBinary(archivePath, binaryName, destDir) {
  if (archivePath.endsWith('.zip')) {
    execFileSync('unzip', ['-o', '-j', archivePath, binaryName, '-d', destDir], {
      stdio: 'inherit',
    });
  } else {
    execFileSync('tar', ['-xzf', archivePath, '-C', destDir, binaryName], {
      stdio: 'inherit',
    });
  }
  const dest = path.join(destDir, binaryName);
  if (!fs.existsSync(dest)) {
    throw new Error(`${binaryName} not found inside ${archivePath}`);
  }
  // Executable for everyone, writable only by the owner.
  fs.chmodSync(dest, 0o755);
  return dest;
}

function manifest(tool, target, version) {
  const { os, cpu } = TARGETS[target];
  const binary = os === 'win32' ? `${tool}.exe` : tool;
  return {
    name: `@altairalabs/${tool}-${target}`,
    version,
    description: `${tool} binary for ${target}`,
    license: 'Apache-2.0',
    repository: {
      type: 'git',
      url: 'https://github.com/AltairaLabs/promptarena.git',
    },
    homepage: 'https://github.com/AltairaLabs/promptarena#readme',
    // How the package manager decides this is the right one for the host.
    os: [os],
    cpu: [cpu],
    // Yarn PnP would otherwise keep the package zipped, leaving no real path
    // to exec. Binaries have to be unpacked on disk.
    preferUnplugged: true,
    files: [binary, 'LICENSE'],
    publishConfig: { access: 'public' },
  };
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  const repoRoot = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..');
  const license = path.join(repoRoot, 'LICENSE');

  fs.mkdirSync(args.out, { recursive: true });

  let built = 0;
  for (const tool of TOOLS) {
    for (const target of Object.keys(TARGETS)) {
      const pkgDir = path.join(args.out, `${tool}-${target}`);
      fs.rmSync(pkgDir, { recursive: true, force: true });
      fs.mkdirSync(pkgDir, { recursive: true });

      const archive = findArchive(args.archives, tool, args.version, target);
      const binaryName = TARGETS[target].os === 'win32' ? `${tool}.exe` : tool;
      extractBinary(archive, binaryName, pkgDir);

      fs.writeFileSync(
        path.join(pkgDir, 'package.json'),
        JSON.stringify(manifest(tool, target, args.version), null, 2) + '\n'
      );
      fs.copyFileSync(license, path.join(pkgDir, 'LICENSE'));

      console.log(`built @altairalabs/${tool}-${target}@${args.version}`);
      built += 1;
    }
  }
  console.log(`\n${built} platform packages in ${args.out}`);
}

main();
