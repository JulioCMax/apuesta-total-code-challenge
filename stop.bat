@echo off
setlocal enabledelayedexpansion
rem Double-click teardown for Windows, matching start.bat. Runs
rem `docker compose down -v` exactly as documented in README section 1 -
rem this script only adds preflight checks and a window that stays open
rem long enough to read the result.
rem
rem Note on encoding: on-screen text below is deliberately unaccented ASCII
rem Spanish - see start.bat for why (UTF-8 accented text corrupts cmd.exe's
rem own command parsing here, even under `chcp 65001`).

set "SCRIPT_DIR=%~dp0"
cd /d "%SCRIPT_DIR%"

echo ==========================================================
echo   Apuesta Total - deteniendo el entorno
echo ==========================================================
echo.

if not exist "%SCRIPT_DIR%docker-compose.yml" (
    echo [ERROR] No se encontro docker-compose.yml junto a este script ^(%SCRIPT_DIR%^).
    echo Asegurese de que stop.bat permanezca en la raiz del repositorio clonado.
    goto :fail
)

where docker >nul 2>&1
if errorlevel 1 (
    echo [ERROR] No se encontro el comando 'docker'.
    echo Se requiere Docker Desktop. Descarguelo desde:
    echo   https://www.docker.com/products/docker-desktop/
    goto :fail
)

docker info >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Docker esta instalado, pero el daemon no responde.
    echo Si Docker Desktop no esta en ejecucion, no hay nada que detener.
    goto :fail
)

echo Deteniendo y limpiando los contenedores, la red y los volumenes...
echo.
docker compose down -v
if errorlevel 1 (
    echo.
    echo [ERROR] No se pudo detener el entorno correctamente.
    echo Revise el detalle con: docker compose ps
    goto :fail
)

echo.
echo Entorno detenido y limpio.
echo Para volver a levantarlo: start.bat ^(o start.command en macOS/Linux^), o docker compose up --build
echo.
echo Presione una tecla para cerrar esta ventana...
pause >nul
exit /b 0

:fail
echo.
echo Presione una tecla para cerrar esta ventana...
pause >nul
exit /b 1
