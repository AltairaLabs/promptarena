import { jest } from '@jest/globals';
import {
  TARGETS,
  targetFor,
  isSupported,
  packageFor,
  binaryFor,
  resolveBinary,
} from './resolve.js';

describe('resolve', () => {
  describe('TARGETS', () => {
    it('covers the six goos/goarch combinations goreleaser builds', () => {
      expect([...TARGETS].sort()).toEqual([
        'darwin-arm64',
        'darwin-x64',
        'linux-arm64',
        'linux-x64',
        'win32-arm64',
        'win32-x64',
      ]);
    });

    it('is frozen so a caller cannot widen the supported set', () => {
      expect(Object.isFrozen(TARGETS)).toBe(true);
    });
  });

  describe('targetFor', () => {
    it('joins platform and arch', () => {
      expect(targetFor('darwin', 'arm64')).toBe('darwin-arm64');
    });

    it('defaults to the running interpreter', () => {
      expect(targetFor()).toBe(`${process.platform}-${process.arch}`);
    });
  });

  describe('isSupported', () => {
    it('accepts a published target', () => {
      expect(isSupported('linux-x64')).toBe(true);
    });

    it('rejects a target we do not build', () => {
      expect(isSupported('freebsd-x64')).toBe(false);
    });
  });

  describe('packageFor', () => {
    it('builds the scoped platform package name', () => {
      expect(packageFor('promptarena', 'darwin-arm64')).toBe(
        '@altairalabs/promptarena-darwin-arm64'
      );
    });
  });

  describe('binaryFor', () => {
    it('appends .exe on Windows', () => {
      expect(binaryFor('promptarena', 'win32')).toBe('promptarena.exe');
    });

    it('leaves the name bare elsewhere', () => {
      expect(binaryFor('promptarena', 'linux')).toBe('promptarena');
      expect(binaryFor('promptarena', 'darwin')).toBe('promptarena');
    });
  });

  describe('resolveBinary', () => {
    it('resolves the platform package subpath', () => {
      const resolve = jest.fn(() => '/pkgs/promptarena-linux-x64/promptarena');
      const got = resolveBinary('promptarena', {
        resolve,
        platform: 'linux',
        arch: 'x64',
      });

      expect(resolve).toHaveBeenCalledWith(
        '@altairalabs/promptarena-linux-x64/promptarena'
      );
      expect(got).toBe('/pkgs/promptarena-linux-x64/promptarena');
    });

    it('asks for the .exe subpath on Windows', () => {
      const resolve = jest.fn(() => 'C:\\pkgs\\promptarena.exe');
      resolveBinary('promptarena', {
        resolve,
        platform: 'win32',
        arch: 'x64',
      });

      expect(resolve).toHaveBeenCalledWith(
        '@altairalabs/promptarena-win32-x64/promptarena.exe'
      );
    });

    it('throws a build-from-source hint on an unsupported target', () => {
      const resolve = jest.fn();

      expect(() =>
        resolveBinary('promptarena', {
          resolve,
          platform: 'freebsd',
          arch: 'x64',
        })
      ).toThrow(/does not ship a prebuilt binary for freebsd-x64/);
      expect(resolve).not.toHaveBeenCalled();
    });

    it('throws a reinstall hint when the optional dependency is missing', () => {
      const resolve = jest.fn(() => {
        throw new Error('Cannot find module');
      });

      expect(() =>
        resolveBinary('promptarena', {
          resolve,
          platform: 'linux',
          arch: 'x64',
        })
      ).toThrow(/--omit=optional/);
    });

    it('keeps the underlying resolution error as the cause', () => {
      const cause = new Error('Cannot find module');
      const resolve = jest.fn(() => {
        throw cause;
      });

      try {
        resolveBinary('promptarena', {
          resolve,
          platform: 'linux',
          arch: 'x64',
        });
        throw new Error('expected resolveBinary to throw');
      } catch (err) {
        expect(err.cause).toBe(cause);
      }
    });
  });

  describe('defaults from the running interpreter', () => {
    it('binaryFor falls back to process.platform', () => {
      const expected = process.platform === 'win32' ? 'promptarena.exe' : 'promptarena';
      expect(binaryFor('promptarena')).toBe(expected);
    });

    it('resolveBinary falls back to the host target', () => {
      const resolve = jest.fn(() => '/resolved/promptarena');
      resolveBinary('promptarena', { resolve });

      const host = `${process.platform}-${process.arch}`;
      expect(resolve).toHaveBeenCalledWith(
        `@altairalabs/promptarena-${host}/${binaryFor('promptarena')}`
      );
    });

    it('resolveBinary rejects a call with no options at all', () => {
      // No resolver supplied: the missing-package path must still produce the
      // actionable error rather than a TypeError.
      expect(() => resolveBinary('promptarena')).toThrow(/promptarena/);
    });
  });
});
