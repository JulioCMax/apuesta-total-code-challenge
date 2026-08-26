@echo off
setlocal enabledelayedexpansion
rem Double-click launcher for Windows that complements the documented
rem `docker compose up --build` path (README section 1) rather than
rem replacing it. It adds the preflight checks and readiness feedback a
rem reviewer running this unattended would otherwise be missing: Docker
rem Desktop not running, port 8080 already taken, a build that silently
rem takes a minute, and not knowing when the API actually becomes reachable.
rem
rem All business configuration (env vars, ports, build targets) stays in
rem docker-compose.yml; this script only orchestrates around it.
rem
rem Note on encoding: on-screen text below is deliberately unaccented ASCII
rem Spanish. Empirically verified on this host: UTF-8 accented text in a
rem .bat, even under `chcp 65001`, corrupts cmd.exe's own command parsing
rem (observed failure: "echo Verificando..." became an unrecognized command
rem named "ficando"). Plain ASCII sidesteps that failure mode entirely.

set "API_URL=http://localhost:8080"
set "HEALTH_TIMEOUT=120"

rem --- Resolve the script's own directory -------------------------------
rem A double-click from Explorer does not guarantee the working directory
rem is the script's directory, so every path below is resolved relative to
rem where this file actually lives ("%~dp0").
set "SCRIPT_DIR=%~dp0"
cd /d "%SCRIPT_DIR%"

echo ==========================================================
echo   Apuesta Total - World Cup Betting API
echo ==========================================================
echo.

rem --- Preflight: running from the repository root ----------------------
if not exist "%SCRIPT_DIR%docker-compose.yml" (
    echo [ERROR] No se encontro docker-compose.yml junto a este script ^(%SCRIPT_DIR%^).
    echo Asegurese de que start.bat permanezca en la raiz del repositorio clonado.
    goto :fail
)

rem --- Preflight: Docker CLI on PATH --------------------------------------
where docker >nul 2>&1
if errorlevel 1 (
    echo [ERROR] No se encontro el comando 'docker'.
    echo Se requiere Docker Desktop. Descarguelo desde:
    echo   https://www.docker.com/products/docker-desktop/
    goto :fail
)

rem --- Preflight: Docker daemon actually running --------------------------
rem Checking that the CLI exists is not enough - 'docker info' is the only
rem way to know the daemon is actually reachable.
echo Verificando que Docker Desktop este en ejecucion...
docker info >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Docker esta instalado, pero el daemon no responde.
    echo Inicie Docker Desktop y espere a que indique que esta listo ^("Docker Desktop is running"^).
    echo Luego vuelva a ejecutar este script.
    goto :fail
)
echo Docker esta listo.
echo.

rem --- Preflight: port 8080 free (or already ours) ------------------------
echo Verificando el puerto 8080...
set "PORT_BUSY=0"
netstat -ano | findstr /C:"LISTENING" | findstr /C:":8080 " >nul 2>&1
if not errorlevel 1 set "PORT_BUSY=1"

if "!PORT_BUSY!"=="1" (
    set "OUR_API=0"
    rem findstr /X (exact whole-line match) unreliably fails to match the
    rem LF-terminated lines that `docker compose ps` emits (a known
    rem findstr quirk with Unix line endings, confirmed on this host) - a
    rem literal substring match is used instead, which is unambiguous here
    rem since no other service name in docker-compose.yml contains "api".
    docker compose ps --status running --services 2>nul | findstr /C:"api" >nul 2>&1
    if not errorlevel 1 set "OUR_API=1"

    if "!OUR_API!"=="1" (
        echo El puerto 8080 ya esta en uso por la propia pila de este proyecto ^(ejecucion anterior^). Continuando...
    ) else (
        echo [ERROR] El puerto 8080 ya esta en uso por otro proceso.
        echo La API necesita ese puerto para su servidor HTTP. Opciones:
        echo   1. Detenga el proceso que actualmente usa el puerto 8080, o
        echo   2. Publique la API en otro puerto agregando un docker-compose.override.yml, por ejemplo:
        echo        services:
        echo          api:
        echo            ports:
        echo              - "8081:8080"
        echo      y vuelva a ejecutar este script.
        goto :fail
    )
)
echo.

rem --- Build and start ------------------------------------------------------
echo Construyendo y levantando los servicios ^(dynamodb, seed, api^)...
echo La primera compilacion puede tardar aproximadamente 1 minuto; si no ve
echo salida nueva durante un rato, es normal, no se ha colgado.
echo.

docker compose up -d --build
if errorlevel 1 (
    echo.
    echo [ERROR] La construccion o el arranque de los contenedores fallo.
    echo Revise el detalle con: docker compose logs
    goto :fail
)
echo.

rem --- Wait for readiness -----------------------------------------------------
echo Esperando a que la API responda en %API_URL%/health ...

set "HAVE_CURL=0"
where curl >nul 2>&1
if not errorlevel 1 set "HAVE_CURL=1"

set /a ELAPSED=0

:waitloop
if !ELAPSED! GEQ !HEALTH_TIMEOUT! (
    echo.
    echo [ERROR] La API no respondio dentro de !HEALTH_TIMEOUT! segundos.
    echo Consulte los registros con: docker compose logs api
    goto :fail
)

if "!HAVE_CURL!"=="1" (
    curl -fs -o nul "%API_URL%/health" 2>nul
) else (
    powershell -NoProfile -Command "try { Invoke-WebRequest -Uri '%API_URL%/health' -UseBasicParsing -TimeoutSec 3 | Out-Null; exit 0 } catch { exit 1 }" >nul 2>&1
)
if not errorlevel 1 goto :ready

echo   ... esperando ^(!ELAPSED!s / !HEALTH_TIMEOUT!s^)
timeout /t 2 /nobreak >nul
set /a ELAPSED+=2
goto :waitloop

:ready
echo.
echo La API esta lista.
echo.

rem --- Open the browser ---------------------------------------------------------
start "" "%API_URL%/docs"

rem --- Summary -------------------------------------------------------------------
echo ==========================================================
echo   Entorno listo
echo ==========================================================
echo.
echo Documentacion interactiva (Swagger UI): %API_URL%/docs
echo Especificacion OpenAPI 3:               %API_URL%/openapi.yaml
echo Sonda de vida:                          %API_URL%/health
echo.
echo Credenciales de demostracion (saldo inicial S/ 1000.00 cada una):
echo   demo1@apuestatotal.com / Demo1234^^!
echo   demo2@apuestatotal.com / Demo1234^^!
echo.
echo Prueba de extremo a extremo ^(login -^> calculo -^> colocacion -^> saldo^):
echo   scripts\smoke.sh %API_URL%
echo.
echo Para detener y limpiar el entorno:
echo   stop.bat   ^(o stop.command en macOS/Linux^)
echo   o directamente:  docker compose down -v
echo.
echo Presione una tecla para cerrar esta ventana...
pause >nul
exit /b 0

:fail
echo.
echo Presione una tecla para cerrar esta ventana...
pause >nul
exit /b 1
