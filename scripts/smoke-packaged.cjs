const { spawn, spawnSync } = require('node:child_process');
const fs = require('node:fs/promises');
const fsSync = require('node:fs');
const path = require('node:path');

const projectRoot = path.resolve(__dirname, '..');

function defaultExecutable() {
  if (process.platform === 'win32') return path.join(projectRoot, 'dist', 'win-unpacked', 'Zajuna App.exe');
  return path.join(projectRoot, 'dist', 'linux-unpacked', 'zajuna-app');
}

function packagedCoreDir(executable) {
  return path.resolve(path.dirname(executable), 'resources', 'core');
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function endpointFor(child, output, timeoutMs = 20000) {
  const tmpDir = require('node:os').tmpdir();
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (child.exitCode !== null || child.signalCode !== null) {
      throw new Error(
        `El proceso empaquetado terminó antes de exponer /api/health (código ${child.exitCode}, señal ${child.signalCode}).\n--- stdout/stderr ---\n${output.text() || '(vacío)'}`
      );
    }
    try {
      const candidate = (await fs.readdir(tmpDir)).find((name) => name.startsWith(`zajuna-app-${child.pid}-`) && name.endsWith('.json'));
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
  throw new Error(
    `El paquete no expuso /api/health en ${timeoutMs} ms (pid ${child.pid}).\n--- stdout/stderr ---\n${output.text() || '(vacío)'}`
  );
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
  const args = [`--user-data-dir=${userDataDir}`];
  if (process.platform === 'linux') {
    // El chrome-sandbox empaquetado no queda con dueño root/modo 4755 en un
    // runner de CI corriente; solo este smoke lo desactiva, nunca el binario
    // que instala un usuario real.
    args.push('--no-sandbox');
  }
  const child = spawn(executable, args, {
    cwd: path.dirname(executable),
    windowsHide: true,
    stdio: ['ignore', 'pipe', 'pipe'],
    env: { ...process.env, ZAJUNA_SKIP_EXTERNAL_OPEN: '1' },
  });
  const outputChunks = [];
  child.stdout?.on('data', (chunk) => outputChunks.push(chunk));
  child.stderr?.on('data', (chunk) => outputChunks.push(chunk));
  const output = { text: () => Buffer.concat(outputChunks).toString('utf8').trim() };
  try {
    const { endpoint, file } = await endpointFor(child, output);
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
    // Sin sesión local toda ruta /api/* (salvo /api/health) responde 401 JSON
    // antes del router: nunca el HTML de la SPA ni una cookie.
    const anonymous = await fetch(`${endpoint.url}/api/missing`);
    const anonymousBody = await anonymous.text();
    if (anonymous.status !== 401 || anonymous.headers.get('set-cookie') || !anonymousBody.includes('local_session_required')) {
      throw new Error(`ruta API sin sesión: estado ${anonymous.status}, esperaba 401 local_session_required`);
    }
    console.log('Smoke OK: embed, fallback SPA, assets, 404 static y API protegida sin sesión.');

    await stopProcess(child);
    await fs.rm(file, { force: true });
    if (fsSync.existsSync(file)) throw new Error('El endpoint temporal no se pudo eliminar.');
    await fs.rm(userDataDir, { recursive: true, force: true });
  } catch (error) {
    await stopProcess(child);
    await fs.rm(userDataDir, { recursive: true, force: true });
    throw error;
  }
}

main().catch((error) => {
  console.error(`Smoke empaquetado falló: ${error.message}`);
  process.exit(1);
});
