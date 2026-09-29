# AGENTS.md — Contrato de colaboración multiagente

Este documento es el **contrato común** del repositorio Zajuna Sync para el
entorno de desarrollo multiagente. Lo usan Orca al coordinar y todos los
agentes al trabajar dentro de sus Worktrees.

Contiene **reglas permanentes de colaboración y comportamiento**. No es
documentación del producto ni planificación de tareas.

| Pieza | Responsabilidad |
|---|---|
| **AGENTS.md** | Reglas de colaboración entre agentes (este archivo). |
| **GSD** | Planificación específica de cada tarea. |
| **Linear** | Tareas, requisitos y alcance. |
| **Orca** | Coordinación de agentes y Git Worktrees. |
| **Slack** | Comunicación. |
| **Config de cada agente** | Instrucciones propias de ese agente. |

Las instrucciones específicas de un solo agente **no van aquí**: viven en su
propia configuración.

## Contexto mínimo del proyecto

Aplicación de escritorio local. Electron es un launcher silencioso que arranca
el core Go en loopback; el core sirve la API local y el frontend React
embebido con `go:embed`.

- `core/` — Go: API local, SQLite y migraciones, workers, scheduler,
  evidencias, reportes, capturas con Playwright.
- `frontend/` — React 19 + Vite + TypeScript, lint con oxlint.
- `desktop/` — ciclo de vida Electron y supervisor del core.
- `scripts/` — build, sincronización, empaquetado y smokes.
- `docs/` — arquitectura, API local, sistema visual, auditorías y matriz de
  pruebas. Es la referencia previa a cualquier cambio de fondo.

Detalles vigentes: [`README.md`](README.md),
[`docs/architecture.md`](docs/architecture.md),
[`docs/api-local.md`](docs/api-local.md),
[`docs/design-system.md`](docs/design-system.md).

## Regla de independencia

Los agentes evitan invadir el trabajo de otros sin necesidad. Si un agente
detecta un problema que corresponde principalmente a otro:

1. lo documenta con claridad;
2. evita hacer cambios fuera de su responsabilidad salvo que sea necesario;
3. deja que el agente responsable realice la corrección.

## Regla de Worktrees y Orca

Orca es el entorno que coordina los agentes y sus Git Worktrees. Cada agente
trabaja **únicamente dentro del Worktree que Orca le asignó** para la tarea.

Los agentes:

- no modifican Worktrees ajenos;
- no asumen que pueden cambiar ramas de otros agentes;
- no eliminan cambios hechos por otros agentes;
- revisan el estado de Git antes de tocar archivos cuando haya posibilidad de
  trabajo paralelo;
- mantienen sus cambios dentro del alcance de la tarea.

## Regla de cambio mínimo

Solo los cambios necesarios para cumplir correctamente la tarea. No se debe:

- refactorizar código no relacionado;
- cambiar la arquitectura por preferencia personal;
- reformatear archivos completos sin necesidad;
- instalar dependencias innecesarias;
- modificar partes del sistema ajenas a la tarea.

Nota concreta del repo: `core/cmd/zajuna-core/web/` es salida generada por
`npm run frontend:sync`. No se edita a mano; se regenera desde `frontend/`.

## Regla de verificación

Una tarea no está terminada por el solo hecho de haber escrito el código.
Cuando corresponda, el flujo es:

```text
IMPLEMENTACIÓN → REVISIÓN INDEPENDIENTE → TESTS → VERIFICACIÓN → ENTREGA
```

Los resultados de revisiones y tests se consideran **antes** de marcar una
tarea como completada.

Verificaciones disponibles en el repo (ejecutar las que apliquen al cambio):

```powershell
npm run build --prefix frontend
npm run lint --prefix frontend
go -C core test ./...
go -C core vet ./...
npm run test:downloads
npm run test:smoke:native
npm run test:browser:core
```

CI (`.github/workflows/ci.yml`) ejecuta lint y build del frontend, `go test` y
`go vet` del core, y los tests de `scripts/`. Los instaladores nativos se
construyen solo por `workflow_dispatch`.

## Regla de comunicación

Al terminar su trabajo, cada agente informa de forma clara:

- qué realizó;
- qué archivos modificó;
- qué verificaciones ejecutó;
- qué tests ejecutó;
- qué problemas encontró;
- qué problemas quedaron pendientes;
- qué decisiones importantes tomó.
