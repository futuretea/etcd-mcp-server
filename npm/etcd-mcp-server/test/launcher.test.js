const assert = require('node:assert/strict');
const childProcess = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const test = require('node:test');

const packageRoot = path.resolve(__dirname, '..');
const platformPackages = {
  darwin_x64: {name: 'etcd-mcp-server-darwin-amd64', suffix: ''},
  darwin_arm64: {name: 'etcd-mcp-server-darwin-arm64', suffix: ''},
  linux_x64: {name: 'etcd-mcp-server-linux-amd64', suffix: ''},
  linux_arm64: {name: 'etcd-mcp-server-linux-arm64', suffix: ''},
};

const platformPackage = platformPackages[`${process.platform}_${process.arch}`];

const packLauncher = (tempDirectory) => {
  const packageSource = path.join(tempDirectory, 'package-source');
  fs.mkdirSync(packageSource, {recursive: true});
  fs.cpSync(path.join(packageRoot, 'bin'), path.join(packageSource, 'bin'), {recursive: true});
  fs.copyFileSync(path.join(packageRoot, 'package.json'), path.join(packageSource, 'package.json'));

  const output = childProcess.execFileSync('npm', ['pack', '--json'], {
    cwd: packageSource,
    encoding: 'utf8',
  });
  return {packageSource, tarball: path.join(packageSource, JSON.parse(output)[0].filename)};
};

const installFixturePackages = (tempDirectory, tarball, binary) => {
  const scopedPackages = path.join(tempDirectory, 'node_modules', '@futuretea');
  const unpackDirectory = path.join(tempDirectory, 'unpacked');
  fs.mkdirSync(scopedPackages, {recursive: true});
  fs.mkdirSync(unpackDirectory, {recursive: true});
  childProcess.execFileSync('tar', ['-xzf', tarball, '-C', unpackDirectory]);
  fs.renameSync(path.join(unpackDirectory, 'package'), path.join(scopedPackages, 'etcd-mcp-server'));

  const platformDirectory = path.join(scopedPackages, binary.name);
  fs.mkdirSync(path.join(platformDirectory, 'bin'), {recursive: true});
  fs.writeFileSync(
    path.join(platformDirectory, 'package.json'),
    JSON.stringify({name: `@futuretea/${binary.name}`, version: '0.0.0'}, null, 2),
  );
  const binaryPath = path.join(platformDirectory, 'bin', `etcd-mcp-server${binary.suffix}`);
  fs.writeFileSync(binaryPath, '#!/bin/sh\nprintf "%s\\n" "$@"\n');
  fs.chmodSync(binaryPath, 0o755);
  return path.join(scopedPackages, 'etcd-mcp-server', 'bin', 'index.js');
};

test('installed launcher resolves the native package and forwards MCP arguments', {skip: !platformPackage}, (t) => {
  const tempDirectory = fs.mkdtempSync(path.join(os.tmpdir(), 'etcd-mcp-launcher-'));
  t.after(() => fs.rmSync(tempDirectory, {force: true, recursive: true}));

  const {tarball} = packLauncher(tempDirectory);
  const launcher = installFixturePackages(tempDirectory, tarball, platformPackage);
  const output = childProcess.execFileSync(process.execPath, [launcher, '--config', 'example.yaml'], {
    encoding: 'utf8',
  });

  assert.deepEqual(output.trimEnd().split('\n'), ['mcp', '--config', 'example.yaml']);
});
