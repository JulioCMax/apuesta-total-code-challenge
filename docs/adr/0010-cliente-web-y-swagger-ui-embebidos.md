# ADR-0010: Cliente web y Swagger UI embebidos, con dependencias vendorizadas

- **Estado**: aceptada
- **Ámbito**: `internal/adapters/web/`, `api/swagger-ui/`, `api/embed.go`,
  `internal/adapters/http/handler/app.go`, `internal/adapters/http/handler/docs.go`

## Contexto

La API se puede ejercitar con `curl`, pero dos afirmaciones del proyecto no se
demuestran con `curl`:

1. Que la metadata expuesta por el catálogo **alcanza para construir una interfaz**. Los
   campos `settings.hasStatistics`, `settings.isBetBuilderEnabled` y `marketType.id`
   existen porque una UI real los consume; sin una UI que los consuma, son campos que
   nadie ha comprobado que sirvan para algo.
2. Que el débito bajo concurrencia funciona. Existe una prueba de integración con
   goroutines contra `dynamodb-local` (ADR-0003), pero una prueba que pasa en una
   terminal convence menos que dos colocaciones simultáneas resolviéndose en pantalla.

Servir una interfaz introduce, sin embargo, una pregunta que este proyecto tenía
deliberadamente cerrada hasta ahora: **de dónde salen los recursos que el navegador
carga**. La respuesta habitual —un CDN o una etapa de compilación de JavaScript— entra
en conflicto directo con dos propiedades ya adquiridas: que el servicio se despliega
como un único ZIP de Lambda sin dependencias en tiempo de ejecución (ADR-0004) y que
`docker compose up` es un solo comando sin herramientas adicionales.

La página de documentación `/docs` ya se había enfrentado a la misma pregunta con Swagger
UI. Este ADR recoge la decisión que ambas comparten.

## Alternativas evaluadas

| Alternativa | Ventajas | Desventajas |
|---|---|---|
| No enviar interfaz alguna: solo `curl` y la especificación OpenAPI | Cero superficie añadida | Las dos afirmaciones del contexto quedan sin demostrar; la metadata de presentación del catálogo no tiene ningún consumidor que la valide |
| Aplicación separada, desplegada aparte (S3 + CloudFront, Vercel) | Separación limpia entre API y cliente; camino de producción real | Un segundo artefacto que desplegar, versionar y mantener sincronizado; introduce CORS y un origen distinto; CloudFront añade coste y superficie fuera del alcance (ADR-0004) |
| Bundler (Vite) con etapa Node en el `Dockerfile` | Componentes de un solo archivo, *tree-shaking*, ecosistema npm completo | El contenido del binario de Go pasaría a depender de que un `npm install` funcione; añade una etapa Node a la imagen, un árbol `node_modules` y una cadena de herramientas de JavaScript a un repositorio que hoy no tiene ninguna |
| Vue desde un CDN (`unpkg`, `jsdelivr`) | Nada que vendorizar; una etiqueta `<script>` | `/app` dejaría de funcionar sin conexión y pasaría a depender de la disponibilidad de un tercero; introduce una dependencia en tiempo de ejecución que el resto de la arquitectura no tiene. La `index.html` que el propio Swagger UI distribuye ilustra el problema: apunta por defecto a `https://petstore.swagger.io/v2/swagger.json` |
| **Dependencias vendorizadas y embebidas con `go:embed`, servidas por el mismo binario** | El binario se basta a sí mismo; funciona sin conexión; ninguna etapa de compilación nueva; una sola cosa que desplegar | Los archivos vendorizados se revisan y se actualizan a mano; sin *tree-shaking*; el binario crece |

## Decisión

Se sirven **desde el propio binario** tanto el cliente web (`GET /app`) como la
documentación interactiva (`GET /docs`), con sus dependencias de navegador
**vendorizadas** en el repositorio y embebidas con `go:embed`.

Es la misma estrategia de entrega que ADR-0002 aplica al catálogo de eventos —un dato
inmutable viaja dentro del binario en vez de resolverse en tiempo de ejecución—,
aplicada aquí a los recursos de una interfaz.

**Qué se vendoriza**, con su procedencia registrada en un `VENDORED.md` por directorio:

- **Vue 3.5.41** (`vue.esm-browser.prod.js`, ≈ 171 KB), MIT. Se elige a propósito la
  compilación que **incluye el compilador de plantillas**: es lo que permite que los
  componentes declaren su marcado como cadenas `template` corrientes, sin ningún paso de
  compilación previo. Su licencia se embebe junto al runtime, de modo que el aviso viaja
  con cada copia del binario que lo redistribuye.
- **Swagger UI 5.32.14** (≈ 1.7 MB entre el *bundle*, la hoja de estilos y el favicon),
  Apache 2.0. Se descarta `swagger-ui-standalone-preset.js`, que solo aporta la barra
  superior que permite apuntar la interfaz a una especificación externa arbitraria:
  `/docs` renderiza siempre exactamente una (`/openapi.yaml`), así que ese preset —y la
  superficie de ataque que implica— no se vendoriza.

**El cliente web vive en `internal/adapters/web/`**, y esa ubicación es una afirmación
arquitectónica, no una comodidad: es un adaptador de entrega igual que `adapters/http`.
Consume el mismo contrato HTTP público que consumiría cualquier cliente externo, no
contiene ninguna regla de negocio, y el dominio y los casos de uso no saben que existe.
Sustituirlo o eliminarlo no toca nada por debajo.

Tres detalles del servido quedaron anclados por pruebas porque ninguno es evidente:

- **`c.Data` en lugar de `http.FileServer`.** `FileServer` resuelve el archivo desde
  `c.Request.URL.Path`, lo que obligaría a reescribir la ruta de la solicitud a medio
  vuelo; `middleware.Logging` lee ese mismo campo cuando el handler ya terminó, así que
  cada recurso quedaría registrado bajo una ruta que el cliente nunca pidió.
- **Tabla propia de tipos de contenido, no `mime.TypeByExtension`.** Esa función consulta
  el registro del sistema anfitrión —en Windows, literalmente el registro—, de modo que
  el mismo binario podría servir `.js` como `text/plain` en una máquina y como
  `text/javascript` en otra. Un módulo ES servido como `text/plain` lo rechaza de plano
  el cargador de cualquier navegador. La tabla convierte esa respuesta en parte de la
  compilación (`TestAppHandler_ServesModulesAsJavaScript`).
- **El recurso al *shell* para rutas desconocidas se detiene en el prefijo `/app`.** Es
  el comportamiento estándar de una aplicación de página única, para que un enlace
  profundo o una recarga aterricen en el cliente. Fuera de `/app`, una ruta desconocida
  sigue llegando al `NoRoute` del router y a la envolvente JSON 404 de siempre
  (ADR-0001). Un intento de travesía de directorios (`/app/../../etc/passwd`) se rechaza
  con `fs.ValidPath` como un 404 duro, nunca como una ruta de cliente
  (`TestAppHandler_RejectsTraversal`).

`/app` y `/docs` son superficies de demostración, no endpoints de negocio: no llevan
límite de tasa ni verificación de JWT. Eso no relaja nada, porque el navegador no es más
que otro cliente — cada llamada que hace atraviesa `/api/v1` y queda limitada y
protegida exactamente igual que la de cualquier otro consumidor.

## Consecuencias

- **Positivas**: `/app` y `/docs` funcionan **sin conexión a internet** y sin ningún
  tercero disponible. La ausencia de recursos externos no es una promesa de la prosa:
  `TestDocsHTML_HasNoExternalScriptOrLinkTags` falla si alguien introduce una etiqueta
  `<script>` o `<link>` que apunte fuera.
- **Positivas**: no se añade ninguna cadena de herramientas de JavaScript. No hay
  `package.json` en el repositorio, ni etapa Node en el `Dockerfile`, ni `node_modules`.
  Construir el proyecto sigue siendo `go build`, y desplegarlo sigue siendo el mismo ZIP
  de Lambda de ADR-0004, sin un segundo artefacto que sincronizar.
- **Negativas**: el binario crece ≈ 1.9 MB entre Swagger UI y Vue, que se suman a los
  1.7 MB del catálogo (ADR-0002). Es holgadamente asumible dentro de los límites de
  paquete de Lambda, y se paga una sola vez en el despliegue en lugar de en cada carga de
  página por cada visitante.
- **Negativas**: no hay *tree-shaking* ni componentes de un solo archivo, y actualizar
  una dependencia es un `npm pack` manual y una sustitución de archivos —el
  procedimiento exacto está escrito en cada `VENDORED.md`— en lugar de un cambio de
  versión. Para una interfaz de este tamaño ninguna de esas pérdidas compensa reintroducir
  una etapa de compilación.
- **Negativas**: se incorpora al repositorio código de terceros que nadie de este
  proyecto escribió. Se mitiga con procedencia explícita: versión exacta, tarball de
  origen, fecha de obtención, licencia copiada literalmente, y la lista de lo que se
  descartó a propósito y por qué. `TestAppHandler_VendoredLicenseIsShipped` comprueba que
  el aviso de licencia de Vue efectivamente se sirve.
- **Neutras**: el cliente web replica el diseño de referencia del reto y, por tanto,
  tiene texto en español, coherente con la frontera de ADR-0007: es prosa dirigida a una
  persona, mientras que los identificadores, rutas y campos JSON que consume siguen en
  inglés.
- **Neutras**: si algún día conviniera un despliegue separado del cliente, el cambio es
  eliminar este adaptador y publicar `assets/` en cualquier alojamiento estático. El
  cliente ya consume únicamente el contrato HTTP público, así que no hay nada que
  desacoplar primero — solo aparecería CORS, que hoy no existe porque el origen es el
  mismo.
