// Locates the prebuilt Go binary that ships in a per-platform sibling package.
//
// This replaces the old postinstall downloader. npm 12 blocks dependency
// lifecycle scripts unless the consumer allowlists them, so a `postinstall`
// that fetches the binary silently never runs and every invocation dies with
// "binary not found". Platform packages listed as optionalDependencies are
// plain resolution instead: npm/pnpm/yarn/bun filter them by the `os` and `cpu`
// fields and unpack only the one that matches, with no scripts involved.

const SCOPE = '@altairalabs';

// Targets we publish a binary package for, spelled the way Node spells them
// (`process.platform`-`process.arch`) so a target string can be compared
// against the running interpreter without a lookup table. GoReleaser builds
// linux/darwin/windows against amd64/arm64, which is exactly these six.
export const TARGETS = Object.freeze([
  'darwin-arm64',
  'darwin-x64',
  'linux-arm64',
  'linux-x64',
  'win32-arm64',
  'win32-x64',
]);

export function targetFor(platform = process.platform, arch = process.arch) {
  return `${platform}-${arch}`;
}

export function isSupported(target) {
  return TARGETS.includes(target);
}

export function packageFor(tool, target) {
  return `${SCOPE}/${tool}-${target}`;
}

export function binaryFor(tool, platform = process.platform) {
  return platform === 'win32' ? `${tool}.exe` : tool;
}

/**
 * Resolve the on-disk path of the binary for the running platform.
 *
 * @param {string} tool            Binary name, e.g. 'promptarena'.
 * @param {object} options
 * @param {(id: string) => string} options.resolve  Resolver, normally
 *   `createRequire(import.meta.url).resolve`. Injected so tests can drive the
 *   failure paths without installing real platform packages.
 * @param {string} [options.platform]  Defaults to process.platform.
 * @param {string} [options.arch]      Defaults to process.arch.
 * @returns {string} Absolute path to the executable.
 */
export function resolveBinary(tool, { resolve, platform, arch } = {}) {
  const target = targetFor(platform, arch);

  if (!isSupported(target)) {
    throw new Error(
      `${tool} does not ship a prebuilt binary for ${target}.\n` +
        `Supported targets: ${TARGETS.join(', ')}.\n` +
        `Build from source instead: https://github.com/AltairaLabs/promptarena`
    );
  }

  const pkg = packageFor(tool, target);

  try {
    return resolve(`${pkg}/${binaryFor(tool, platform ?? process.platform)}`);
  } catch (cause) {
    // The optional dependency is declared but absent. The usual causes are an
    // install that opted out of optional deps, or a lockfile carried over from
    // a different platform.
    throw new Error(
      `${tool} could not find its platform binary package ${pkg}.\n` +
        `This normally means the install ran with --no-optional or ` +
        `--omit=optional, or a lockfile from another platform was reused.\n` +
        `Reinstall with optional dependencies enabled: npm install ${SCOPE}/${tool}`,
      { cause }
    );
  }
}
