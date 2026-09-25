const { app, shell } = require('electron');
const { spawn } = require('node:child_process');
const { stopProcessTree } = require('./stop-process-tree.cjs');
const crypto = require('node:crypto');
const fs = require('node:fs/promises');
const path = require('node:path');
const os = require('node:os');

const isDevelopment = !app.isPackaged;
// Electron is only the silent launcher/supervisor. It does not create a
// BrowserWindow: React is rendered by the user's default browser at loopback.
// GPU/hardware acceleration is never needed here; disabling it avoids a GPU
// process crash on Linux hosts without accelerated graphics (CI runners,
// containers, some window managers) that would otherwise block startup.
app.disableHardwareAcceleration();
if (process.platform === 'linux') {
  // The AppImage target has no privileged install step, so chrome-sandbox
  // never ends up root-owned/mode 4755 the way Chromium's SUID sandbox
  // requires — Electron aborts on startup otherwise. This previously only
  // disabled the sandbox in the CI-only smoke test (scripts/smoke-packaged.cjs),
  // which meant CI stayed green while every real Linux user hit the same
  // fatal error the smoke test was bypassing. The app never renders remote or
  // untrusted content in a Chromium window (React runs in the user's own
  // default browser), so disabling the sandbox here does not expose it to a
  // hostile page the way it would in a general-purpose browser.
  app.commandLine.appendSwitch('no-sandbox');
  // Electron always initializes a display backend (X11/Wayland) even though
  // this app never creates a BrowserWindow, so it fatals with "Missing X
  // server or $DISPLAY" on any headless/server Linux host (no desktop
  // session, SSH-only access, minimal installs). Forcing the headless Ozone
  // platform skips that requirement entirely.
  app.commandLine.appendSwitch('ozone-platform', 'headless');
}
const hasSingleInstanceLock = app.requestSingleInstanceLock();
const skipExternalOpen = process.env.ZAJUNA_SKIP_EXTERNAL_OPEN === '1';
let coreProcess;
let endpointFile;
let coreEndpoint;
let stopCorePromise;
let coreStartPromise;
let coreRecoveryPromise;
let coreLogWrite = Promise.resolve();
let quitting = false;

const CORE_LOG_LIMIT = 1024 * 1024;
// Shared only with the core it spawns (via its environment, which the core
// clears at startup). It is the only way to mint the single-use URLs that
// start a browser session, so no other local process can obtain one.
const launcherSecret = crypto.randomBytes(32).toString('base64url');
const SESSION_START_PREFIX = '/api/session/start?';

function redactCoreLog(value) {
  return String(value)
    .replace(/((?:sesskey|token|access_token|refresh_token|password|secret|authorization)[=:\s]+)[^\s&,'"]+/gi, '$1[REDACTED]')
    .replace(/([?&](?:sesskey|token|access_token|refresh_token|password|secret|authorization)=)[^&\s]+/gi, '$1[REDACTED]');
}

function coreLogPath() {
  return path.join(app.getPath('userData'), 'logs', 'zajuna-core.log');
}

function appendCoreLog(chunk) {
  const text = redactCoreLog(chunk);
  coreLogWrite = coreLogWrite
    .then(async () => {
      const logPath = coreLogPath();
      await fs.mkdir(path.dirname(logPath), { recursive: true });
      try {
        const stats = await fs.stat(logPath);
        if (stats.size >= CORE_LOG_LIMIT) {
          await fs.rm(`${logPath}.1`, { force: true });
          await fs.rename(logPath, `${logPath}.1`);
        }
      } catch {
        // El archivo puede no existir todavía.
      }
      await fs.appendFile(logPath, text, 'utf8');
    })
    .catch(() => {
      // La telemetría local no debe impedir que la aplicación funcione.
    });
  return coreLogWrite;
}

async function openExternalBrowser(url) {
  if (skipExternalOpen) return;
  try {
    const opened = await shell.openExternal(url);
    if (!opened) {
      await appendCoreLog(`[shell] El sistema no confirmó la apertura del navegador externo para ${url}.\n`);
    }
  } catch (error) {
    // A browser association can be missing on a fresh machine. Keep the core
    // alive so the user can still open the endpoint from the diagnostic log.
    await appendCoreLog(`[shell] No se pudo abrir el navegador externo: ${error.message}\n`);
  }
}

async function sessionStartUrl(endpoint) {
  const response = await fetch(endpoint.url + '/api/session/bootstrap', {
    method: 'POST',
    headers: { Authorization: `Bearer ${launcherSecret}` },
  });
  if (!response.ok) {
    throw new Error(`El núcleo rechazó el inicio de sesión local (HTTP ${response.status}).`);
  }
  const body = await response.json();
  if (!body || typeof body.path !== 'string' || !body.path.startsWith(SESSION_START_PREFIX)) {
    throw new Error('El núcleo devolvió un enlace de inicio de sesión inválido.');
  }
  return endpoint.url + body.path;
}

// Every launch (first, second instance, recovery) mints a new single-use URL.
async function openAppInBrowser(endpoint) {
  let url;
  try {
    url = await sessionStartUrl(endpoint);
  } catch (error) {
    await appendCoreLog(`[launcher] No se pudo preparar la sesión local: ${error.message}\n`);
    return;
  }
  if (skipExternalOpen) {
    await appendCoreLog('[launcher] Sesión local preparada; apertura del navegador omitida.\n');
    return;
  }
  await openExternalBrowser(url);
}

function coreBinaryPath() {
  const binaryName = process.platform === 'win32' ? 'zajuna-core.exe' : 'zajuna-core';
  return isDevelopment
    ? path.join(__dirname, '..', 'core', 'bin', binaryName)
    : path.join(process.resourcesPath, 'core', binaryName);
}

function validateLocalEndpoint(value) {
  if (!value || typeof value.url !== 'string' || !Number.isInteger(value.port)) {
    throw new Error('El núcleo publicó un endpoint local inválido.');
  }
  let parsed;
  try {
    parsed = new URL(value.url);
  } catch {
    throw new Error('El núcleo publicó una URL local inválida.');
  }
  if (
    parsed.protocol !== 'http:' ||
    parsed.hostname !== '127.0.0.1' ||
    parsed.port !== String(value.port) ||
    parsed.username ||
    parsed.password ||
    parsed.search ||
    parsed.hash ||
    parsed.pathname !== '/'
  ) {
    throw new Error('El núcleo debe publicar únicamente una URL HTTP de loopback.');
  }
  return { url: parsed.origin, port: value.port };
}

async function startCore() {
  if (coreStartPromise) return coreStartPromise;
  const startPromise = startCoreOnce();
  coreStartPromise = startPromise;
  try {
    return await startPromise;
  } finally {
    if (coreStartPromise === startPromise) coreStartPromise = undefined;
  }
}

async function startCoreOnce() {
  if (coreProcess && coreProcess.exitCode === null && coreEndpoint) {
    return coreEndpoint;
  }

  if (coreProcess && coreProcess.exitCode !== null) {
    coreProcess = undefined;
  }

  const binary = coreBinaryPath();
  try {
    await fs.access(binary);
  } catch {
    throw new Error(
      'No se encontró el núcleo local en ' + binary + '. Ejecuta "npm run build" antes de abrir Electron.',
    );
  }

  const endpointNonce = crypto.randomBytes(16).toString('hex');
  endpointFile = path.join(os.tmpdir(), `zajuna-app-${process.pid}-${endpointNonce}.json`);
  await fs.rm(endpointFile, { force: true });

  const child = spawn(binary, ['--port=0', '--no-browser', '--endpoint-file=' + endpointFile], {
    cwd: path.dirname(binary),
    // Tells the core a supervisor will restart it: after a staged data reset
    // it exits cleanly and recoverCore() starts it again and reopens the UI.
    env: { ...process.env, ZAJUNA_SUPERVISED: '1', ZAJUNA_LAUNCHER_SECRET: launcherSecret },
    stdio: ['ignore', 'ignore', 'pipe'],
    windowsHide: true,
    // On POSIX, detached makes the core the leader of its own process group
    // (pgid === pid), so stopProcessTree can signal the whole group -pid
    // instead of only the core itself, reaching any worker it spawned.
    detached: process.platform !== 'win32',
  });
  coreProcess = child;

  let coreError = '';
  child.stderr?.on('data', (chunk) => {
    const redacted = redactCoreLog(chunk.toString());
    coreError = (coreError + redacted).slice(-8000);
    void appendCoreLog(redacted);
  });

  return new Promise((resolve, reject) => {
    let settled = false;
    let pollTimer;
    const finish = (callback, value) => {
      if (settled) return;
      settled = true;
      clearTimeout(timeout);
      if (pollTimer) clearTimeout(pollTimer);
      callback(value);
    };
    const timeout = setTimeout(() => {
      const message = 'El núcleo local no inició a tiempo.' + (coreError ? ' ' + coreError.trim() : '');
      stopCore().finally(() => finish(reject, new Error(message)));
    }, 20000);

    child.once('error', (error) => {
      finish(reject, new Error(`No se pudo iniciar el núcleo local: ${error.message}`));
    });

    child.once('exit', (code, signal) => {
      if (coreProcess === child) {
        coreProcess = undefined;
        coreEndpoint = undefined;
      }
      if (!settled) {
        const message = 'El núcleo local terminó inesperadamente.' + (coreError ? ' ' + coreError.trim() : '');
        finish(reject, new Error(message));
      } else if (!quitting) {
        void recoverCore(`exit code=${code ?? 'null'} signal=${signal ?? 'null'}`);
      }
    });

    const poll = async () => {
      if (settled) return;
      try {
        const contents = await fs.readFile(endpointFile, 'utf8');
        const endpoint = validateLocalEndpoint(JSON.parse(contents));
        await waitForCoreReady(endpoint);
        coreEndpoint = endpoint;
        finish(resolve, endpoint);
      } catch {
        if (settled) return;
        if (child.exitCode !== null) {
          const message = 'El núcleo local terminó inesperadamente.' + (coreError ? ' ' + coreError.trim() : '');
          finish(reject, new Error(message));
          return;
        }
        pollTimer = setTimeout(poll, 50);
      }
    };

    poll();
  });
}

async function recoverCore(reason) {
  if (quitting || coreRecoveryPromise) return coreRecoveryPromise;

  coreRecoveryPromise = (async () => {
    await appendCoreLog(`\n[supervisor] El núcleo terminó (${reason}); intentando recuperación.\n`);

    let endpoint;
    let lastError;
    for (let attempt = 1; attempt <= 3; attempt += 1) {
      try {
        endpoint = await startCore();
        break;
      } catch (error) {
        lastError = error;
        await appendCoreLog(`[supervisor] Intento ${attempt}/3 fallido: ${error.message}\n`);
        await new Promise((resolve) => setTimeout(resolve, attempt * 500));
      }
    }

    if (!endpoint) {
      const message = 'El núcleo local no pudo recuperarse después de varios intentos.' +
        (lastError ? ` ${lastError.message}` : '');
      await appendCoreLog(`[supervisor] ${message}\n`);
      quitting = true;
      await stopCore();
      app.quit();
      return;
    }
    await openAppInBrowser(endpoint);
  })().finally(() => {
    coreRecoveryPromise = undefined;
  });

  return coreRecoveryPromise;
}

async function waitForCoreReady(endpoint, timeoutMs = 5000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const controller = new AbortController();
    const abortTimer = setTimeout(() => controller.abort(), 500);
    try {
      const response = await fetch(endpoint.url + '/api/health', { signal: controller.signal });
      if (response.ok) return;
    } catch {
      // El servidor puede haber escrito el endpoint unos milisegundos antes
      // de aceptar la primera petición.
    } finally {
      clearTimeout(abortTimer);
    }
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  throw new Error('El endpoint local no respondió a /api/health.');
}

async function stopCore() {
  if (stopCorePromise) return stopCorePromise;

  stopCorePromise = (async () => {
    const processToStop = coreProcess;
    coreProcess = undefined;
    coreEndpoint = undefined;

    if (processToStop && processToStop.exitCode === null) {
      await stopProcessTree(processToStop);
    }

    if (endpointFile) {
      await fs.rm(endpointFile, { force: true });
      endpointFile = undefined;
    }
  })().finally(() => {
    stopCorePromise = undefined;
  });

  return stopCorePromise;
}


function setupAutoUpdater() {
  // Packaged builds only. Dev runs stay on the local Electron binary and never
  // talk to GitHub Releases. CSC/Authenticode remains optional: unsigned
  // Windows installs may fail the download/apply step (SmartScreen / code
  // integrity); we log and continue so the launcher still works.
  if (!app.isPackaged || process.env.ZAJUNA_DISABLE_AUTO_UPDATE === '1') {
    return;
  }

  let autoUpdater;
  try {
    ({ autoUpdater } = require('electron-updater'));
  } catch (error) {
    void appendCoreLog(`[updater] electron-updater no disponible: ${error.message}\n`);
    return;
  }

  // Download in the background; install only when the app quits. This is a
  // silent launcher (no BrowserWindow / no in-app prompt), so forcing
  // quitAndInstall mid-session would interrupt the core and any capture work.
  autoUpdater.autoDownload = true;
  autoUpdater.autoInstallOnAppQuit = true;
  autoUpdater.allowDowngrade = false;

  autoUpdater.on('checking-for-update', () => {
    void appendCoreLog('[updater] Comprobando actualizaciones (GitHub Releases)…\n');
  });
  autoUpdater.on('update-available', (info) => {
    void appendCoreLog(`[updater] Actualización disponible: ${info.version}\n`);
  });
  autoUpdater.on('update-not-available', (info) => {
    void appendCoreLog(`[updater] Sin actualizaciones (actual ${info.version}).\n`);
  });
  // electron-updater emits 'error' and also rejects checkForUpdates() with the
  // same error, whose message carries every HTTP header: log it once, on one
  // line, in plain words.
  let lastUpdaterError = '';
  const logUpdaterError = (error) => {
    const message = String((error && error.message) || error || '');
    if (message === lastUpdaterError) return;
    lastUpdaterError = message;
    const firstLine = message.split('\n')[0].slice(0, 300);
    const detail = /latest(-linux)?\.yml/.test(message) && /404/.test(message)
      ? 'la publicación más reciente de GitHub no incluye latest.yml; no hay actualización automática disponible.'
      : `${firstLine}. En Windows sin firma Authenticode SmartScreen puede bloquear la descarga/aplicación.`;
    void appendCoreLog(`[updater] Error al actualizar: ${detail}\n`);
  };
  autoUpdater.on('error', logUpdaterError);
  autoUpdater.on('download-progress', (progress) => {
    const pct = Number.isFinite(progress.percent) ? progress.percent.toFixed(1) : '?';
    void appendCoreLog(`[updater] Descarga ${pct}%\n`);
  });
  autoUpdater.on('update-downloaded', (info) => {
    void appendCoreLog(
      `[updater] Actualización ${info.version} descargada. ` +
        'Se instalará al cerrar la aplicación (no se fuerza el cierre ahora).\n',
    );
  });

  void autoUpdater.checkForUpdates().catch(logUpdaterError);
}

if (!hasSingleInstanceLock) {
  app.exit(0);
} else {
  app.on('second-instance', async () => {
    try {
      const endpoint = coreEndpoint || await startCore();
      await openAppInBrowser(endpoint);
    } catch (error) {
      await appendCoreLog(`[launcher] No se pudo reutilizar el core existente: ${error.message}\n`);
    }
  });

  app.whenReady().then(async () => {
    try {
      coreEndpoint = await startCore();
      await openAppInBrowser(coreEndpoint);
      setupAutoUpdater();
    } catch (error) {
      await appendCoreLog(`[launcher] No se pudo iniciar Zajuna App: ${error.message}\n`);
      await stopCore();
      app.quit();
    }
  });
}

app.on('before-quit', (event) => {
  if (quitting) return;
  event.preventDefault();
  quitting = true;
  stopCore().finally(() => app.quit());
});

process.on('uncaughtException', (error) => {
  void appendCoreLog(`[launcher] Error no controlado: ${error.message}\n`);
  quitting = true;
  void stopCore().finally(() => app.exit(1));
});

process.on('unhandledRejection', (reason) => {
  const message = reason instanceof Error ? reason.message : String(reason);
  void appendCoreLog(`[launcher] Promesa no controlada: ${message}\n`);
});
