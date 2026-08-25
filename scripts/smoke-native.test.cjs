const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {
  signingSecretsPresent,
  classifyAuthenticodeStatus,
  listReleaseArtifacts,
  buildReport,
} = require('./smoke-native.cjs');

function testDetectsSigningSecretsWithoutPrintingThem() {
  assert.equal(signingSecretsPresent({}), false);
  assert.equal(signingSecretsPresent({ CSC_LINK: 'secret' }), true);
  assert.equal(signingSecretsPresent({ APPLE_ID: 'user@example.com' }), true);
}

function testClassifiesAuthenticode() {
  assert.equal(classifyAuthenticodeStatus('NotSigned'), 'unsigned');
  assert.equal(classifyAuthenticodeStatus('Valid'), 'valid');
  assert.equal(classifyAuthenticodeStatus('HashMismatch'), 'invalid');
}

function testInventoriesArtifactsAndBlocksUnsignedRelease() {
  const distRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'zajuna-native-'));
  fs.writeFileSync(path.join(distRoot, 'Zajuna App Setup 0.1.0.exe'), 'installer');
  fs.writeFileSync(path.join(distRoot, 'release-manifest.json'), '{}');
  const artifacts = listReleaseArtifacts(distRoot);
  assert.equal(artifacts.length, 1);
  assert.equal(artifacts[0].file, 'Zajuna App Setup 0.1.0.exe');
  assert.match(artifacts[0].sha256, /^[a-f0-9]{64}$/);

  const report = buildReport({ distRoot, env: {}, platform: 'linux', unpackedExists: false });
  assert.equal(report.signingSecretsPresent, false);
  assert.equal(report.releaseBlocked, true);
  assert.equal(report.artifacts[0].signing, 'unsigned');
  assert.ok(report.blockers.some((item) => /sin firma/i.test(item) || /CSC_LINK/i.test(item)));
}

function testEmptyDistIsBlockedWithoutClaimingGreen() {
  const distRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'zajuna-empty-'));
  const report = buildReport({ distRoot, env: {}, platform: 'win32', unpackedExists: false });
  assert.equal(report.artifacts.length, 0);
  assert.equal(report.releaseBlocked, true);
  assert.ok(report.blockers.some((item) => /No hay artefactos/i.test(item)));
}

testDetectsSigningSecretsWithoutPrintingThem();
testClassifiesAuthenticode();
testInventoriesArtifactsAndBlocksUnsignedRelease();
testEmptyDistIsBlockedWithoutClaimingGreen();
console.log('smoke-native tests: 4 passed');
