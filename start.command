#!/usr/bin/env bash
# Double-click launcher for macOS (and, unchanged, most Linux desktops) that
# complements the documented `docker compose up --build` path (README section
# 1) rather than replacing it. It adds the preflight checks and readiness
# feedback a reviewer running this unattended would otherwise be missing:
# Docker Desktop not running, port 8080 already taken, a build that silently
# takes a minute, and not knowing when the API actually becomes reachable.
#
# All business configuration (env vars, ports, build targets) stays in
# docker-compose.yml; this script only orchestrates around it.
set -euo pipefail

API_URL="http://localhost:8080"
HEALTH_TIMEOUT_SECONDS=120

# --- Resolve the script's own directory -------------------------------------
# A double-click (Finder or Explorer-via-WSL-style launch) does not
# guarantee the working directory is the script's directory, so every path
# below is resolved relative to where this file actually lives.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" >/dev/null 2>&1 && pwd -P)"
cd "$SCRIPT_DIR"

log()  { printf '%s\n' "$1"; }
warn() { printf '%s\n' "$1" >&2; }
die() {
  warn ""
  warn "[ERROR] $1"
  exit 1
}

# Keeps the terminal window open long enough to read the result when this
# script was launched from a double-click, without blocking automated runs
# (no tty attached) or CI.
if [ -t 0 ]; then
  pause_on_exit() {
    printf '\n'
    read -n 1 -s -r -p "Presione una tecla para cerrar esta ventana..." || true
    printf '\n'
  }
  trap pause_on_exit EXIT
fi

log "=========================================================="
log "  Apuesta Total - World Cup Betting API"
log "=========================================================="
log ""

# --- Preflight: running from the repository root ----------------------------
[ -f "${SCRIPT_DIR}/docker-compose.yml" ] || die "No se encontró docker-compose.yml junto a este script (${SCRIPT_DIR}).
Asegúrese de que start.command permanezca en la raíz del repositorio clonado."

# --- Preflight: Docker CLI on PATH ------------------------------------------
if ! command -v docker >/dev/null 2>&1; then
  die "No se encontró el comando 'docker'.
Se requiere Docker Desktop. Descárguelo desde:
  https://www.docker.com/products/docker-desktop/"
fi

# --- Preflight: Docker daemon actually running ------------------------------
# Checking that the CLI exists is not enough — 'docker info' is the only way
# to know the daemon is actually reachable.
log "Verificando que Docker Desktop esté en ejecución..."
if ! docker info >/dev/null 2>&1; then
  die "Docker está instalado, pero el daemon no responde.
Inicie Docker Desktop y espere a que indique que está listo ('Docker Desktop is running').
Luego vuelva a ejecutar este script."
fi
log "Docker está listo."
log ""

# --- Preflight: port 8080 free (or already ours) ----------------------------
# Uses bash's built-in /dev/tcp pseudo-device so this check needs no external
# tool (no lsof/ss/nc dependency) on either macOS or Linux.
port_in_use() {
  (exec 3<>"/dev/tcp/127.0.0.1/8080") 2>/dev/null
}

log "Verificando el puerto 8080..."
if port_in_use; then
  if docker compose ps --status running --services 2>/dev/null | grep -qx "api"; then
    log "El puerto 8080 ya está en uso por la propia pila de este proyecto (ejecución anterior). Continuando..."
  else
    die "El puerto 8080 ya está en uso por otro proceso.
La API necesita ese puerto para su servidor HTTP. Opciones:
  1. Detenga el proceso que actualmente usa el puerto 8080, o
  2. Publique la API en otro puerto agregando un docker-compose.override.yml, por ejemplo:
       services:
         api:
           ports:
             - \"8081:8080\"
     y vuelva a ejecutar este script."
  fi
fi
log ""

# --- Build and start ---------------------------------------------------------
log "Construyendo y levantando los servicios (dynamodb, seed, api)..."
log "La primera compilación puede tardar aproximadamente 1 minuto; si no ve"
log "salida nueva durante un rato, es normal, no se ha colgado."
log ""

if ! docker compose up -d --build; then
  die "La construcción o el arranque de los contenedores falló.
Revise el detalle con: docker compose logs"
fi
log ""

# --- Wait for readiness -------------------------------------------------------
check_health() {
  if command -v curl >/dev/null 2>&1; then
    curl -fs -o /dev/null "${API_URL}/health" 2>/dev/null
    return $?
  elif command -v wget >/dev/null 2>&1; then
    wget -q -O /dev/null "${API_URL}/health"
    return $?
  else
    # Last-resort fallback with no external dependency beyond bash itself:
    # a raw HTTP/1.0 GET over /dev/tcp, just checking for a "200" status line.
    if ! exec 9<>"/dev/tcp/127.0.0.1/8080" 2>/dev/null; then
      return 1
    fi
    printf 'GET /health HTTP/1.0\r\nHost: localhost\r\n\r\n' >&9
    local status_line
    status_line=$(head -n 1 <&9 2>/dev/null || true)
    exec 9>&- 9<&-
    case "$status_line" in
      *200*) return 0 ;;
      *) return 1 ;;
    esac
  fi
}

log "Esperando a que la API responda en ${API_URL}/health ..."
elapsed=0
until check_health; do
  if [ "$elapsed" -ge "$HEALTH_TIMEOUT_SECONDS" ]; then
    die "La API no respondió dentro de ${HEALTH_TIMEOUT_SECONDS} segundos.
Consulte los registros con: docker compose logs api"
  fi
  printf '.'
  sleep 2
  elapsed=$((elapsed + 2))
done
log ""
log "La API está lista."
log ""

# --- Open the browser ---------------------------------------------------------
# The web client, not the API docs: this launcher exists for a reviewer
# who would rather not open a terminal, and the running application is the
# useful landing page. /docs is listed in the summary below.
DOCS_URL="${API_URL}/app"
if command -v open >/dev/null 2>&1; then
  open "$DOCS_URL" >/dev/null 2>&1 || warn "No se pudo abrir el navegador automáticamente. Ábralo manualmente en: ${DOCS_URL}"
elif command -v xdg-open >/dev/null 2>&1; then
  xdg-open "$DOCS_URL" >/dev/null 2>&1 || warn "No se pudo abrir el navegador automáticamente. Ábralo manualmente en: ${DOCS_URL}"
else
  warn "No se encontró un comando para abrir el navegador automáticamente. Ábralo manualmente en: ${DOCS_URL}"
fi

# --- Summary -------------------------------------------------------------------
log "=========================================================="
log "  Entorno listo"
log "=========================================================="
log ""
log "Aplicación web (calendario y cupón):    ${API_URL}/app"
log "Documentación interactiva (Swagger UI): ${API_URL}/docs"
log "Especificación OpenAPI 3:               ${API_URL}/openapi.yaml"
log "Sonda de vida:                          ${API_URL}/health"
log ""
log "Credenciales de demostración (saldo inicial S/ 1000.00 cada una):"
log "  demo1@apuestatotal.com / Demo1234!"
log "  demo2@apuestatotal.com / Demo1234!"
log ""
log "Prueba de extremo a extremo (login -> cálculo -> colocación -> saldo):"
log "  scripts/smoke.sh ${API_URL}"
log ""
log "Para detener y limpiar el entorno:"
log "  ./stop.command   (o stop.bat en Windows)"
log "  o directamente:  docker compose down -v"
log ""
