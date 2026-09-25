const { spawn, spawnSync } = require('node:child_process');
const fs = require('node:fs/promises');
const fsSync = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const projectRoot = path.resolve(__dirname, '..');
const devMode = process.env.ZAJUNA_EXTERNAL_DEV === '1';
const executable = path.resolve(
  process.env.ZAJUNA_PACKAGED_EXECUTABLE ||
    (devMode
      ? path.join(projectRoot, 'node_modules', 'electron', 'dist', process.platform === 'win32' ? 'electron.exe' : 'Electron')
      : process.platform === 'win32'
        ? path.join(projectRoot, 'dist', 'win-unpacked', 'Zajuna App.exe')
        : path.join(projectRoot, 'dist', 'linux-unpacked', 'zajuna-app')),
);

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function endpointFor(pid, timeoutMs = 20000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const candidate = (await fs.readdir(os.tmpdir())).find((name) => name.startsWith(`zajuna-app-${pid}-`) && name.endsWith('.json'));
      if (!candidate) throw new Error('endpoint pending');
      const file = path.join(os.tmpdir(), candidate);
      const endpoint = JSON.parse(await fs.readFile(file, 'utf8'));
      const response = await fetch(`${endpoint.url}/api/health`);
      if (response.ok) return { endpoint, file };
    } catch {
      // Electron y Go pueden tardar unos milisegundos en iniciar.
    }
    await sleep(250);
  }
  throw new Error(`El modo navegador externo no expuso /api/health en ${timeoutMs} ms.`);
}

async function stopProcess(child) {
  if (!child || child.exitCode !== null) return;
  if (process.platform === 'win32') {
    spawnSync('taskkill', ['/PID', String(child.pid), '/T', '/F'], { stdio: 'ignore', windowsHide: true });
  } else {
    child.kill('SIGTERM');
  }
  await new Promise((resolve) => {
    const timer = setTimeout(resolve, 5000);
    child.once('exit', () => {
      clearTimeout(timer);
      resolve();
    });
  });
}

async function main() {
  if (!fsSync.existsSync(executable)) {
    throw new Error(`No existe el ejecutable empaquetado: ${executable}`);
  }
  console.log(`Iniciando smoke de navegador externo: ${executable}`);
  const userDataDir = path.join(projectRoot, 'tmp', 'smoke-external-user-data');
  await fs.rm(userDataDir, { recursive: true, force: true });
  const launchArgs = devMode ? [projectRoot, '--open-browser', `--user-data-dir=${userDataDir}`] : ['--open-browser', `--user-data-dir=${userDataDir}`];
  const child = spawn(executable, launchArgs, {
    cwd: path.dirname(executable),
    windowsHide: true,
    stdio: 'ignore',
    env: { ...process.env, ZAJUNA_OPEN_BROWSER: '1', ZAJUNA_SKIP_EXTERNAL_OPEN: '1' },
  });
  let duplicate;
  try {
    const { endpoint, file } = await endpointFor(child.pid);
    console.log(`Smoke OK: modo externo activo, endpoint loopback ${endpoint.url}.`);
    duplicate = spawn(executable, launchArgs, {
      cwd: path.dirname(executable),
      windowsHide: true,
      stdio: 'ignore',
      env: { ...process.env, ZAJUNA_OPEN_BROWSER: '1', ZAJUNA_SKIP_EXTERNAL_OPEN: '1' },
    });
    await sleep(1000);
    const duplicateFiles = (await fs.readdir(os.tmpdir())).filter((name) => name.startsWith(`zajuna-app-${duplicate.pid}-`) && name.endsWith('.json'));
    if (duplicateFiles.length > 0) {
      throw new Error('el segundo lanzamiento creó un endpoint propio');
    }
    const health = await fetch(`${endpoint.url}/api/health`);
    if (!health.ok) throw new Error('el core existente dejó de responder después del segundo lanzamiento');
    console.log('Smoke OK: el segundo lanzamiento reutilizó la instancia existente.');
    const coreLog = await fs.readFile(path.join(userDataDir, 'logs', 'zajuna-core.log'), 'utf8').catch(() => '');
    const prepared = coreLog.split('Sesión local preparada').length - 1;
    if (prepared < 2) {
      throw new Error(`el lanzador no preparó la sesión local en ambos lanzamientos (${prepared})`);
    }
    const anonymous = await fetch(`${endpoint.url}/api/setup/status`);
    if (anonymous.status !== 401 || anonymous.headers.get('set-cookie')) {
      throw new Error(`la API aceptó una lectura sin sesión local (HTTP ${anonymous.status})`);
    }
    console.log('Smoke OK: cada lanzamiento obtuvo un enlace de sesión y la API rechaza lecturas anónimas.');
    await stopProcess(duplicate);
    await stopProcess(child);
    await fs.rm(file, { force: true });
    await fs.rm(userDataDir, { recursive: true, force: true });
  } catch (error) {
    await stopProcess(duplicate);
    await stopProcess(child);
    await fs.rm(userDataDir, { recursive: true, force: true });
    throw error;
  }
}

main().catch((error) => {
  console.error(`Smoke de navegador externo falló: ${error.message}`);
  process.exit(1);
});
