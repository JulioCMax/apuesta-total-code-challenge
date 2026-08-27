# ADR-0001: Gin como framework HTTP, frente a chi y `net/http`

- **Estado**: aceptada
- **Ámbito**: `internal/adapters/http/`
- **Decisiones de diseño relacionadas**: D1, D2

## Contexto

La API expone once rutas divididas en dos grupos con requisitos distintos: ocho rutas
públicas (`/health`, `/docs`, `/openapi.yaml`, `/app`, login, listado y detalle del
catálogo de eventos, y cálculo de la apuesta) y tres protegidas por JWT (`place`,
`balance`, `bets`). El servicio necesita, además, una cadena de middleware ordenada
(recovery, request ID, logging estructurado, límite de tamaño del cuerpo, rate limit
por IP y verificación de token) y la posibilidad de aplicar parte de esa cadena solo a
un subárbol de rutas: el rate limit cubre `/api/v1` pero no `/health`, `/docs` ni
`/app`, y la verificación de token cubre únicamente el subgrupo protegido. `/docs` y
`/app` son superficies de demostración servidas desde el propio binario; su contenido y
su procedencia se deciden en ADR-0010.

El exceso de acoplamiento a un framework es un riesgo real en un proyecto que se
presenta como arquitectura hexagonal: si el framework aparece en el dominio o en los
casos de uso, la separación deja de ser verificable.

## Alternativas evaluadas

| Alternativa | Ventajas | Desventajas |
|---|---|---|
| `net/http` con el `ServeMux` mejorado de Go 1.22+ | Cero dependencias; máxima transparencia sobre la mecánica estándar | No ofrece encadenamiento de middleware ni grupos de rutas; toda la composición es manual |
| chi | Compatible con `http.Handler`; grupos de rutas y middleware por grupo; muy delgado | Una dependencia adicional; no es el estándar del equipo receptor |
| **Gin** | Grupos de rutas, binding y validación de DTO, ecosistema de middleware maduro; alineado con el stack del puesto y de la empresa | Introduce su propio `*gin.Context`, que oculta parte de la mecánica de `net/http` |

## Decisión

Se adopta **Gin**, confinado por completo a `internal/adapters/http/`.

La alineación con el stack de destino tiene prioridad sobre la pureza respecto de la
librería estándar: el reto es una prueba técnica para un puesto cuyo framework de
referencia es Gin. El riesgo de acoplamiento se mitiga por construcción, no por
convención:

- `internal/domain/` no importa nada más que la librería estándar y
  `shopspring/decimal`.
- `internal/application/` declara sus propias interfaces (puertos) y no conoce Gin.
- Solo `internal/adapters/http/` importa `github.com/gin-gonic/gin`.

Esa regla se puede comprobar en un instante sobre el árbol de importaciones, y es lo
que permite afirmar que sustituir Gin por chi o por `net/http` es un cambio de
adaptador, no una reescritura.

## Consecuencias

- **Positivas**: el código HTTP es breve y legible; el binding con etiquetas
  `binding:"required,email"` elimina validación manual repetitiva; el grupo protegido
  (`protected := v1.Group("/")` con `middleware.JWTAuth`) hace evidente que la
  verificación del token ocurre antes de cualquier handler.
- **Negativas**: Gin abstrae conceptos de `net/http` que conviene explicitar. Por
  eso el README describe la equivalencia entre el `*gin.Context` y el par
  `(http.ResponseWriter, *http.Request)`, y entre la cadena de middleware de Gin y la
  composición de `http.Handler` de la librería estándar.
- **Neutras**: `SetTrustedProxies(nil)` es obligatorio con Gin para que `ClientIP()`
  resuelva a la dirección real y no a una cabecera `X-Forwarded-For` falsificable
  (véase D11, y ADR-0004 sobre el alcance por instancia del límite de tasa). Es un
  detalle específico del framework que quedó anclado por una prueba:
  `TestRateLimit_KeyIsClientIPNeverXForwardedFor`.
- **Neutras**: Gin trae dos comportamientos por defecto que hubo que corregir de forma
  explícita, ambos invisibles hasta que se prueban. `HandleMethodNotAllowed` es `false`,
  de modo que una ruta existente invocada con un método no soportado cae en `NoRoute`
  (404) en lugar de `NoMethod` (405); y, sin registrar ninguno de los dos, Gin sirve su
  propio cuerpo en texto plano —sin `requestId` y sin la envolvente de error del
  contrato—, lo que además anula el registro de la ruta en el logging. `NewRouter`
  activa el primero y registra ambos manejadores con la envolvente estándar.
- **Neutras**: `middleware.BodyLimit` se aplica en la cadena global, de modo que ninguna
  ruta —incluidas las que no leen cuerpo— puede llegar a bufferizar un cuerpo sin
  límite. No rechaza nada por sí mismo: envuelve `c.Request.Body` en un
  `http.MaxBytesReader` de 1 MiB, así que un cuerpo desmedido hace fallar la lectura y
  los handlers lo tratan con su camino de error de binding ya existente. El resultado es
  la misma envolvente `VALIDATION_ERROR` 400 que cualquier otra solicitud malformada, sin
  código de error nuevo ni rama adicional.
- El mismo `*gin.Engine` se sirve en local mediante `http.Server` y en AWS mediante
  `aws-lambda-go-api-proxy`; el framework elegido no obliga a duplicar el router
  (véase ADR-0004).
