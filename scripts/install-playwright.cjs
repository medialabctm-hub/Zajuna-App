const { spawnSync } = require('node:child_process');
const path = require('node:path');

const projectRoot = path.join(__dirname, '..');
const result = spawnSync('go', ['run', './cmd/playwright-install'], {
  cwd: path.join(projectRoot, 'core'),
  stdio: 'inherit',
});

if (result.error) {
  console.error(`No se pudo ejecutar el instalador de Chromium: ${result.error.message}`);
  process.exit(1);
}
process.exit(result.status ?? 1);
