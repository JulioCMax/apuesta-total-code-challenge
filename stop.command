#!/usr/bin/env bash
# Double-click teardown for macOS (and, unchanged, most Linux desktops),
# matching start.command. Runs `docker compose down -v` exactly as documented
# in README section 1 — this script only adds preflight checks and a window
# that stays open long enough to read the result.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" >/dev/null 2>&1 && pwd -P)"
cd "$SCRIPT_DIR"

log()  { printf '%s\n' "$1"; }
warn() { printf '%s\n' "$1" >&2; }
die() {
  warn ""
  warn "[ERROR] $1"
  exit 1
}

if [ -t 0 ]; then
  pause_on_exit() {
    printf '\n'
    read -n 1 -s -r -p "Presione una tecla para cerrar esta ventana..." || true
    printf '\n'
  }
  trap pause_on_exit EXIT
fi

log "=========================================================="
log "  Apuesta Total - deteniendo el entorno"
log "=========================================================="
log ""

[ -f "${SCRIPT_DIR}/docker-compose.yml" ] || die "No se encontró docker-compose.yml junto a este script (${SCRIPT_DIR}).
Asegúrese de que stop.command permanezca en la raíz del repositorio clonado."

if ! command -v docker >/dev/null 2>&1; then
  die "No se encontró el comando 'docker'.
Se requiere Docker Desktop. Descárguelo desde:
  https://www.docker.com/products/docker-desktop/"
fi

if ! docker info >/dev/null 2>&1; then
  die "Docker está instalado, pero el daemon no responde.
Si Docker Desktop no está en ejecución, no hay nada que detener."
fi

log "Deteniendo y limpiando los contenedores, la red y los volúmenes..."
log ""
if ! docker compose down -v; then
  die "No se pudo detener el entorno correctamente.
Revise el detalle con: docker compose ps"
fi

log ""
log "Entorno detenido y limpio."
log "Para volver a levantarlo: ./start.command (o start.bat en Windows), o docker compose up --build"
log ""
