const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const METADATA_FILES = new Set([
  'release-manifest.json',
  'sbom.cyclonedx.json',
  'downloads.html',
  'native-smoke-report.json',
]);

function signingSecretsPresent(env = process.env) {
  return Boolean(env.CSC_LINK || env.CSC_NAME || env.APPLE_ID);
}

function classifyAuthenticodeStatus(status) {
  const normalized = String(status || '').trim();
  if (!normalized || normalized === 'NotSigned') return 'unsigned';
  if (normalized === 'Valid') return 'valid';
  return 'invalid';
}

function listReleaseArtifacts(distRoot) {
  if (!fs.existsSync(distRoot)) return [];
  return fs.readdirSync(distRoot, { withFileTypes: true })
    .filter((entry) => entry.isFile() && !METADATA_FILES.has(entry.name))
    .map((entry) => {
      const filePath = path.join(distRoot, entry.name);
      return {
        file: entry.name,
        size: fs.statSync(filePath).size,
        sha256: crypto.createHash('sha256').update(fs.readFileSync(filePath)).digest('hex'),
        path: filePath,
      };
    })
    .sort((a, b) => a.file.localeCompare(b.file));
}

function authenticodeStatus(filePath) {
  if (process.platform !== 'win32') return null;
  const result = spawnSync(
    'powershell',
    ['-NoProfile', '-Command', `try { (Get-AuthenticodeSignature -LiteralPath ${JSON.stringify(filePath)}).Status.ToString() } catch { 'UnknownError' }`],
    { encoding: 'utf8', windowsHide: true },
  );
  if (result.status !== 0) return 'UnknownError';
  return String(result.stdout || '').trim() || 'UnknownError';
}

function inspectArtifact(artifact, env = process.env, authenticodeLookup = authenticodeStatus) {
  const authenticode = authenticodeLookup ? authenticodeLookup(artifact.path) : null;
  const signing = authenticode
    ? classifyAuthenticodeStatus(authenticode)
    : (signingSecretsPresent(env) ? 'configured' : 'unsigned');
  return {
    file: artifact.file,
    size: artifact.size,
    sha256: artifact.sha256,
    authenticode: authenticode || 'n/a',
    signing,
  };
}

function buildReport({ distRoot, env = process.env, platform = process.platform, unpackedExists = false, authenticodeLookup }) {
  const lookup = authenticodeLookup ?? (platform === 'win32' ? authenticodeStatus : () => null);
  const artifacts = listReleaseArtifacts(distRoot).map((artifact) => inspectArtifact(artifact, env, lookup));
  const secrets = signingSecretsPresent(env);
  const allValid = artifacts.length > 0 && artifacts.every((item) => item.signing === 'valid');
  const blocked = artifacts.length === 0 || !allValid;
  return {
    generatedAt: new Date().toISOString(),
    platform,
    signingSecretsPresent: secrets,
    unpackedExecutablePresent: unpackedExists,
    artifacts,
    releaseBlocked: blocked,
    blockers: [
      artifacts.length === 0 ? 'No hay artefactos en dist/ (instalador no construido en esta corrida).' : null,
      !secrets ? 'No hay CSC_LINK / CSC_NAME / APPLE_ID en el entorno; la firma no se ejecutó.' : null,
      artifacts.some((item) => item.signing === 'unsigned') ? 'Al menos un artefacto está sin firma válida.' : null,
      artifacts.some((item) => item.signing === 'invalid') ? 'Al menos un artefacto tiene firma inválida o no confiable.' : null,
      platform === 'win32' && !unpackedExists ? 'No hay win-unpacked para smoke empaquetado.' : null,
    ].filter(Boolean),
  };
}

function defaultUnpackedExecutable(projectRoot) {
  if (process.platform === 'win32') return path.join(projectRoot, 'dist', 'win-unpacked', 'Zajuna App.exe');
  if (process.platform === 'darwin') return path.join(projectRoot, 'dist', 'mac', 'Zajuna App.app', 'Contents', 'MacOS', 'Zajuna App');
  return path.join(projectRoot, 'dist', 'linux-unpacked', 'Zajuna App');
}

function writeReport(projectRoot = path.resolve(__dirname, '..'), env = process.env) {
  const distRoot = path.join(projectRoot, 'dist');
  fs.mkdirSync(distRoot, { recursive: true });
  const unpacked = defaultUnpackedExecutable(projectRoot);
  const report = buildReport({
    distRoot,
    env,
    platform: process.platform,
    unpackedExists: fs.existsSync(unpacked),
  });
  const outputPath = path.join(distRoot, 'native-smoke-report.json');
  fs.writeFileSync(outputPath, `${JSON.stringify(report, null, 2)}\n`);
  return { report, outputPath };
}

function printSummary(report) {
  console.log(`smoke-native: ${report.artifacts.length} artefacto(s), releaseBlocked=${report.releaseBlocked}`);
  for (const artifact of report.artifacts) {
    console.log(`- ${artifact.file} sha256=${artifact.sha256} signing=${artifact.signing}`);
  }
  for (const blocker of report.blockers) {
    console.log(`blocker: ${blocker}`);
  }
}

function main() {
  const requireSigned = process.env.ZAJUNA_REQUIRE_SIGNED === '1' || process.argv.includes('--require-signed');
  const { report, outputPath } = writeReport();
  printSummary(report);
  console.log(`reporte: ${outputPath}`);
  if (requireSigned && report.releaseBlocked) {
    process.exit(1);
  }
}

module.exports = {
  signingSecretsPresent,
  classifyAuthenticodeStatus,
  listReleaseArtifacts,
  inspectArtifact,
  buildReport,
  writeReport,
};

if (require.main === module) {
  main();
}
