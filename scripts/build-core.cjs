const { spawnSync } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');

const projectRoot = path.join(__dirname, '..');
const outputName = process.platform === 'win32' ? 'zajuna-core.exe' : 'zajuna-core';
const outputPath = path.join(projectRoot, 'core', 'bin', outputName);

fs.mkdirSync(path.dirname(outputPath), { recursive: true });

const result = spawnSync('go', ['build', '-o', outputPath, './cmd/zajuna-core'], {
  cwd: path.join(projectRoot, 'core'),
  stdio: 'inherit',
});

if (result.error) {
  console.error(`No se pudo ejecutar Go: ${result.error.message}`);
  process.exit(1);
}

if (result.status !== 0) {
  process.exit(result.status ?? 1);
}

console.log(`Core compilado en ${path.relative(projectRoot, outputPath)}`);
