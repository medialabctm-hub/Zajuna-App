const { spawn, spawnSync } = require('node:child_process');
const fs = require('node:fs/promises');
const fsSync = require('node:fs');
const path = require('node:path');

const projectRoot = path.resolve(__dirname, '..');

// electron-builder nombra el ejecutable distinto en cada plataforma: en Windows
// usa productName ("Zajuna App.exe") y en Linux `appInfo.sanitizedName`
// minusculo, es decir el campo `name` de package.json ("zajuna-app"). Buscar
// "Zajuna App" en linux-unpacked hacia fallar el smoke con el paquete correcto
// ya construido.
const NON_APP_BINARIES = new Set(['chrome-sandbox', 'chrome_crashpad_handler']);

function firstExistingPath(candidates) {
  return candidates.find((candidate) => fsSync.existsSync(candidate));
}

function discoverLinuxExecutable(unpackedDir) {
  if (!fsSync.existsSync(unpackedDir)) return undefined;
  return fsSync
    .readdirSync(unpackedDir, { withFileTypes: true })
    .filter((entry) => entry.isFile() && !NON_APP_BINARIES.has(entry.name) && path.extname(entry.name) === '')
    .map((entry) => path.join(unpackedDir, entry.name))
    .find((candidate) => {
      try {
        fsSync.accessSync(candidate, fsSync.constants.X_OK);
        return true;
      } catch {
        return false;
      }
    });
}

function defaultExecutable() {
  if (process.platform === 'win32') {
    return path.join(projectRoot, 'dist', 'win-unpacked', 'Zajuna App.exe');
  }
  const unpackedDir = path.join(projectRoot, 'dist', 'linux-unpacked');
  const named = firstExistingPath([
    path.join(unpackedDir, 'zajuna-app'),
    path.join(unpackedDir, 'Zajuna App'),
  ]);
  return named || discoverLinuxExecutable(unpackedDir) || path.join(unpackedDir, 'zajuna-app');
}

function packagedCoreDir(executable) {
  return path.resolve(path.dirname(executable), 'resources', 'core');
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function endpointFor(pid, timeoutMs = 20000) {
  const tmpDir = require('node:os').tmpdir();
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const candidate = (await fs.readdir(tmpDir)).find((name) => name.startsWith(`zajuna-app-${pid}-`) && name.endsWith('.json'));
      if (!candidate) throw new Error('endpoint pending');
      const expected = path.join(tmpDir, candidate);
      const endpoint = JSON.parse(await fs.readFile(expected, 'utf8'));
      const response = await fetch(`${endpoint.url}/api/health`);
      if (response.ok) return { endpoint, file: expected };
    } catch {
      // Electron y el core pueden tardar unos milisegundos en estar listos.
    }
    await sleep(250);
  }
  throw new Error(`El paquete no expuso /api/health en ${timeoutMs} ms (pid ${pid}).`);
}

// Guarda una cola acotada de la salida del proceso empaquetado: sin esto un
// fallo de arranque solo se ve como "no expuso /api/health".
function collectChildOutput(child, maxChars = 4000) {
  const chunks = [];
  let total = 0;
  const append = (stream, label) => {
    if (!stream) return;
    stream.setEncoding('utf8');
    stream.on('data', (chunk) => {
      chunks.push(`[${label}] ${chunk}`);
      total += chunk.length;
      while (total > maxChars && chunks.length > 1) total -= chunks.shift().length;
    });
    stream.on('error', () => {});
  };
  append(child.stdout, 'stdout');
  append(child.stderr, 'stderr');
  return { tail: () => chunks.join('').trim().slice(-maxChars) };
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

async function assertPage(url, expectedStatus, description, check) {
  const response = await fetch(url);
  if (response.status !== expectedStatus) {
    throw new Error(`${description}: estado ${response.status}, esperaba ${expectedStatus}`);
  }
  const body = await response.text();
  if (check && !check(body)) throw new Error(`${description}: contenido inesperado`);
  return body;
}

async function main() {
  const executable = path.resolve(process.env.ZAJUNA_PACKAGED_EXECUTABLE || defaultExecutable());
  if (!fsSync.existsSync(executable)) {
    throw new Error(`No existe el ejecutable empaquetado: ${executable}`);
  }
  const coreDir = packagedCoreDir(executable);
  const chromiumDir = path.join(coreDir, 'playwright', 'browsers');
  if (!fsSync.existsSync(path.join(coreDir, process.platform === 'win32' ? 'zajuna-core.exe' : 'zajuna-core'))) {
    throw new Error(`El paquete no incluye el core Go en ${coreDir}.`);
  }
  if (!fsSync.existsSync(chromiumDir)) {
    throw new Error(`El paquete no incluye Chromium/Playwright en ${chromiumDir}.`);
  }
  console.log(`Iniciando smoke del paquete: ${executable}`);
  const userDataDir = path.join(projectRoot, 'tmp', 'smoke-packaged-user-data');
  await fs.rm(userDataDir, { recursive: true, force: true });
  // El smoke corre el directorio `linux-unpacked` recien extraido, donde
  // `chrome-sandbox` no puede ser setuid root, asi que Chromium aborta con
  // "The SUID sandbox helper binary ... is not configured correctly". Este
  // smoke valida el core Go y el frontend embebido, no el sandbox de Chromium:
  // el launcher no abre BrowserWindow ni carga contenido remoto en Electron.
  // Esto NO cambia la app entregada; solo esta invocacion de prueba.
  const launchArgs = [`--user-data-dir=${userDataDir}`];
  if (process.platform === 'linux') {
    console.log('Linux: el paquete sin instalar se lanza con --no-sandbox (chrome-sandbox no es setuid en dist/).');
    launchArgs.push('--no-sandbox');
  }
  const child = spawn(executable, launchArgs, {
    cwd: path.dirname(executable),
    windowsHide: true,
    // Capturado, no descartado: cuando el paquete no publica endpoint la unica
    // pista de por que esta en la salida del launcher.
    stdio: ['ignore', 'pipe', 'pipe'],
    env: { ...process.env, ZAJUNA_SKIP_EXTERNAL_OPEN: '1' },
  });
  const childOutput = collectChildOutput(child);
  try {
    const { endpoint, file } = await endpointFor(child.pid);
    const parsedEndpoint = new URL(endpoint.url);
    if (parsedEndpoint.protocol !== 'http:' || parsedEndpoint.hostname !== '127.0.0.1') {
      throw new Error(`El paquete publicó un endpoint fuera de loopback: ${endpoint.url}`);
    }
    console.log('Smoke OK: el core empaquetado respondió a /api/health en loopback.');

    const index = await assertPage(`${endpoint.url}/`, 200, 'shell raíz', (body) => body.includes('id="root"'));
    await assertPage(`${endpoint.url}/resumen`, 200, 'deep link SPA', (body) => body.includes('id="root"'));
    const scriptPath = index.match(/<script[^>]+src="([^"]+\.js)"/)?.[1];
    if (!scriptPath) throw new Error('El index embebido no referencia el bundle JavaScript.');
    await assertPage(new URL(scriptPath, endpoint.url).toString(), 200, 'bundle JavaScript', (body) => body.length > 1000);
    await assertPage(`${endpoint.url}/assets/missing.js`, 404, 'asset inexistente');
    await assertPage(`${endpoint.url}/api/missing`, 404, 'ruta API inexistente');
    console.log('Smoke OK: embed, fallback SPA, assets y 404 API/static verificados.');

    await stopProcess(child);
    await fs.rm(file, { force: true });
    if (fsSync.existsSync(file)) throw new Error('El endpoint temporal no se pudo eliminar.');
    await fs.rm(userDataDir, { recursive: true, force: true });
  } catch (error) {
    await stopProcess(child);
    await fs.rm(userDataDir, { recursive: true, force: true });
    const tail = childOutput.tail();
    throw new Error(tail ? `${error.message}
Salida del paquete:
${tail}` : error.message);
  }
}

main().catch((error) => {
  console.error(`Smoke empaquetado falló: ${error.message}`);
  process.exit(1);
});
