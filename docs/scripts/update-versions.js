#!/usr/bin/env node
/**
 * Updates docs/versions.json (+ the served public/ copy) with the current
 * release version, and optionally archives the previous minor.
 *
 * Usage:
 *   node scripts/update-versions.js <version> <is_prerelease> [needs_archive] [archive_minor] [archive_path]
 *
 * Examples:
 *   node scripts/update-versions.js 1.6.0 false                    # release, no archive
 *   node scripts/update-versions.js 1.6.0 false true 1.5 v1-5      # archive 1.5 when releasing 1.6
 */

const fs = require('fs');
const path = require('path');

const VERSIONS_FILE = path.join(__dirname, '..', 'versions.json');
const PUBLIC_VERSIONS_FILE = path.join(__dirname, '..', 'public', 'versions.json');
const PUBLIC_SCHEMA_REF = 'https://promptarena.altairalabs.ai/versions.schema.json';

function parseVersion(version) {
  const v = version.replace(/^v/, '');
  const [base, prerelease] = v.split('-');
  const [major, minor, patch] = base.split('.').map(Number);
  return {
    full: prerelease ? `${major}.${minor}.${patch}-${prerelease}` : `${major}.${minor}.${patch}`,
    major,
    minor,
    patch,
    prerelease: prerelease || null,
    minorVersion: `${major}.${minor}`,
  };
}

function main() {
  const args = process.argv.slice(2);
  if (args.length < 2) {
    console.error('Usage: node update-versions.js <version> <is_prerelease> [needs_archive] [archive_minor] [archive_path]');
    process.exit(1);
  }

  const version = args[0];
  const isPrerelease = args[1] === 'true';
  const needsArchive = args[2] === 'true';
  const archiveMinor = args[3] || '';
  const archivePath = args[4] || '';

  const parsed = parseVersion(version);
  const now = new Date().toISOString();

  console.log(`Updating versions.json for ${version} (prerelease: ${isPrerelease})`);
  if (needsArchive) {
    console.log(`Archiving previous version ${archiveMinor} at /${archivePath}/`);
  }

  let data;
  try {
    data = JSON.parse(fs.readFileSync(VERSIONS_FILE, 'utf8'));
  } catch (err) {
    console.error(`Error reading ${VERSIONS_FILE}:`, err.message);
    process.exit(1);
  }

  if (!('current' in data)) data = { current: null, archived: [] };
  if (!Array.isArray(data.archived)) data.archived = [];

  // Archive the outgoing version when the minor changes.
  if (needsArchive && data.current && archiveMinor && archivePath) {
    const archivedVersion = {
      version: archiveMinor,
      fullVersion: data.current.fullVersion,
      label: `v${archiveMinor}`,
      path: `/${archivePath}/`,
      released: data.current.released,
      status: 'archived',
      eol: null,
    };
    const existingIndex = data.archived.findIndex((a) => a.version === archiveMinor);
    if (existingIndex >= 0) {
      data.archived[existingIndex] = archivedVersion;
      console.log(`Updated existing archive entry for ${archiveMinor}`);
    } else {
      data.archived.unshift(archivedVersion); // newest first
      console.log(`Added ${archiveMinor} to archived versions`);
    }
  }

  const statusLabel = isPrerelease ? 'Pre-release' : 'Latest';
  data.current = {
    version: parsed.minorVersion,
    fullVersion: parsed.full,
    label: `${statusLabel} (v${parsed.minorVersion})`,
    path: '/',
    released: now,
    status: isPrerelease ? 'prerelease' : 'stable',
    eol: null,
  };

  // Root copy keeps the relative $schema; served copy uses the absolute URL.
  const rootData = { $schema: './versions.schema.json', ...stripSchema(data) };
  const publicData = { $schema: PUBLIC_SCHEMA_REF, ...stripSchema(data) };

  fs.writeFileSync(VERSIONS_FILE, JSON.stringify(rootData, null, 2) + '\n');
  console.log(`Updated ${VERSIONS_FILE}`);

  fs.mkdirSync(path.dirname(PUBLIC_VERSIONS_FILE), { recursive: true });
  fs.writeFileSync(PUBLIC_VERSIONS_FILE, JSON.stringify(publicData, null, 2) + '\n');
  console.log(`Updated ${PUBLIC_VERSIONS_FILE}`);

  console.log(JSON.stringify(rootData, null, 2));
}

function stripSchema(data) {
  const { $schema, ...rest } = data;
  return rest;
}

main();
