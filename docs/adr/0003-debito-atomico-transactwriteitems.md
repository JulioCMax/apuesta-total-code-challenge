# ADR-0003: Débito atómico del saldo con `TransactWriteItems`

- **Estado**: aceptada
- **Ámbito**: `internal/adapters/dynamo/betrepo.go`, `internal/application/betslip/place.go`
- **Decisiones de diseño relacionadas**: D8, D9, D15, D16

## Contexto

El requisito explícito del reto es que el débito del saldo sea **seguro ante
concurrencia**: dos solicitudes simultáneas del mismo usuario no pueden generar un
saldo negativo ni una apuesta registrada sin su débito correspondiente.

Con el estado mutable en DynamoDB (ADR-0002), una colocación de apuesta implica hasta
tres escrituras que deben ser indivisibles:

1. descontar el importe del saldo del usuario, solo si el saldo alcanza;
2. registrar la apuesta;
3. registrar la clave de idempotencia, cuando el cliente envía la cabecera
   `Idempotency-Key`.

## Alternativas evaluadas

| Alternativa | Ventajas | Desventajas |
|---|---|---|
| `SELECT ... FOR UPDATE` en PostgreSQL dentro de una transacción | Muy explícito y fácil de explicar; bloqueo de fila clásico | Exige una base relacional, descartada en ADR-0002 por la restricción de coste y por el stack de destino; añade el problema de agotamiento de conexiones desde Lambda |
| `UpdateItem` condicional y, a continuación, `PutItem` de la apuesta | Dos llamadas simples | **Inseguro**: si el proceso muere entre ambas llamadas, el saldo queda debitado sin apuesta registrada. Es exactamente la ventana que el requisito pide cerrar |
| Contador de versión optimista con reintentos | Sin transacción explícita | El reintento pasaría a ser el mecanismo de corrección: cada conflicto obligaría a releer, recomparar y reescribir en el cliente, y la garantía dependería de que ese bucle esté bien escrito. La condición del propio motor ofrece el mismo resultado sin delegar la corrección al código de la aplicación (el reintento acotado que sí existe, descrito más abajo, es una respuesta a la contención, no el mecanismo que garantiza la atomicidad) |
| **`TransactWriteItems` con condición sobre el saldo** | Todo o nada en un único viaje; la condición se evalúa **dentro** de la transacción, por lo que nunca se lee un saldo obsoleto | El código del SDK es más verboso que su equivalente en SQL |

## Decisión

Se adopta **`TransactWriteItems`**. La colocación de una apuesta se resuelve en como
máximo dos transacciones lógicas, y el orden de los elementos es significativo porque
DynamoDB devuelve los motivos de cancelación en paralelo al índice de cada elemento.

**Intento 1 — camino aceptado** (3 elementos, o 2 sin `Idempotency-Key`):

| Índice | Operación | Condición | Significado si falla |
|---|---|---|---|
| 0 | `Update` sobre `USER#<id> / PROFILE`: `SET balance = balance - :stake` | `attribute_exists(PK) AND balance >= :stake` | saldo insuficiente → intento 2 |
| 1 | `Put` de `USER#<id> / BET#<ulid>` con `status = accepted` | `attribute_not_exists(SK)` | colisión de ULID (transitoria, no esperada) |
| 2 | `Put` de `USER#<id> / IDEMP#<key>` | `attribute_not_exists(SK)` | réplica idempotente → se resuelve el resultado registrado |

**Intento 2 — registro del rechazo** (solo si falló el índice 0). Una segunda
transacción **sin ninguna actualización de saldo**, de modo que un intento rechazado es
estructuralmente incapaz de mover dinero:

| Índice | Operación | Condición |
|---|---|---|
| 0 | `Put` de `USER#<id> / BET#<ulid>` con `status = rejected` y `rejectionReason = insufficient_funds` | `attribute_not_exists(SK)` |
| 1 | `Put` de `USER#<id> / IDEMP#<key>` con `outcome = rejected` | `attribute_not_exists(SK)` |

### Reintento acotado ante contención

Cada una de esas dos transacciones se emite a través de `transactWithConflictRetry`, que
la reintenta hasta **4 intentos** con espera exponencial y jitter (≈ 25, 50 y 100 ms,
con un tope de 2 s). La razón es concreta: **el reintentador estándar de `aws-sdk-go-v2`
no reintenta `TransactionCanceledException`**. Sin ese bucle, dos colocaciones
simultáneas del mismo usuario —que por definición contienden sobre el mismo elemento
`PROFILE`— salían como un 500 sin clasificar y **sin ninguna apuesta persistida**, ni
aceptada ni rechazada: justo el desenlace que el requisito de concurrencia prohíbe.

Este reintento **no relaja la atomicidad ni la sustituye**, y esa distinción es la que
separa esta decisión del contador de versión optimista descartado arriba:

- Cada intento es exactamente la misma transacción todo o nada. Un reintento solo ocurre
  cuando DynamoDB ya confirmó que **no se escribió nada**, así que no existe estado
  parcial que reconciliar. No hay relectura, ni recomparación, ni reescritura en el
  cliente.
- **Una cancelación con cualquier `ConditionalCheckFailed` nunca se reintenta.** Eso es
  un veredicto sobre la solicitud —saldo insuficiente, clave de idempotencia ya usada— y
  al cliente hay que responderle, no hacerle esperar. Solo se reintentan los motivos que
  no dicen nada sobre la solicitud (`TransactionConflict`, `ThrottlingError`,
  `ProvisionedThroughputExceeded`, `TransactionInProgress`) y la lista de motivos vacía,
  una forma anómala que el propio contrato de DynamoDB no debería producir.
- Agotado el presupuesto, el error resultante es `ErrConcurrencyConflict` **tipado**, no
  un 500 opaco, de modo que el handler puede traducirlo a una respuesta con sentido.
- El bucle nunca sobrevive a su solicitud: la espera se cancela con el contexto.

Queda anclado por tres pruebas complementarias:
`TestBetRepository_Place_RetriesTransactionConflictAndSucceeds` (se reintenta lo
transitorio), `TestBetRepository_Place_DoesNotRetryConditionalCheckFailure` (no se
reintenta un veredicto) y
`TestBetRepository_Place_ExhaustedTransactionConflictIsTypedNotUnclassified` (agotarse
produce un error tipado). Las tres usan un `BetStore` que devuelve cancelaciones
fabricadas, porque `dynamodb-local` serializa las transacciones y por tanto **jamás**
puede emitir un `TransactionConflict`.

Cuatro decisiones asociadas completan el comportamiento:

- **Elemento de idempotencia independiente (D9)**: la clave se guarda como
  `IDEMP#<key>` con su propio TTL (`IDEMPOTENCY_TTL`, 24 horas por defecto; un valor
  cero omite el atributo `expiresAt` y el registro no caduca), y el identificador de la
  apuesta es un ULID nuevo. Derivar el identificador de la apuesta de una cabecera
  enviada por el cliente habría acoplado la identidad del recurso a un valor externo y
  habría roto el orden cronológico natural del `SK`.
- **Los rechazos se persisten (D15)**: el reto exige "persistir la apuesta con estado
  claro (accepted, rejected, etc.)". El intento rechazado queda registrado como
  evidencia auditable, y la ausencia de actualización de saldo en esa segunda
  transacción garantiza que no puede alterar el saldo bajo ninguna circunstancia.
- **Una réplica devuelve el resultado registrado, nunca reevalúa (D16)**: una clave de
  idempotencia responde a la pregunta "¿mi solicitud llegó a procesarse?", así que la
  misma clave debe dar siempre la misma respuesta. Reevaluar permitiría que una única
  clave produjese resultados distintos en el tiempo y podría generar un débito que el
  cliente ya consideraba fallido. Un reintento legítimo tras una recarga de saldo es una
  **solicitud nueva** y debe usar una **clave nueva** (semántica equivalente a la de
  Stripe).
- **La clave se ancla a la solicitud que la estrenó**: junto al registro `IDEMP#<key>` se
  guarda una huella (`requestHash`) del contenido de la apuesta. Al resolver una réplica,
  esa huella debe coincidir; si no, la respuesta es `ErrIdempotencyKeyReuse` en lugar del
  resultado almacenado. Sin esa comprobación, reutilizar una clave con un cupón distinto
  devolvería silenciosamente la apuesta *anterior* como si fuera la nueva —y el cliente
  daría por colocada una apuesta que nunca existió—. Es el complemento necesario de D16:
  la misma clave siempre da la misma respuesta **porque** solo se acepta para la misma
  solicitud.

## Consecuencias

- **Positivas**: la corrección ante concurrencia vive en una sola función, revisable de
  principio a fin. La condición `balance >= :stake` se evalúa dentro de la transacción,
  por lo que la segunda solicitud simultánea nunca puede leer un saldo obsoleto.
- **Positivas**: el dinero se almacena como `Number` de DynamoDB (decimal exacto de
  hasta 38 dígitos), de modo que la comparación del saldo es exacta y no interviene
  ninguna aritmética de coma flotante (véase ADR-0005).
- **Negativas**: una escritura transaccional consume el doble de capacidad. Una
  colocación aceptada cuesta 3 elementos × 2 WCU = 6 WCU, y una rechazada ronda las
  10 WCU sumando el intento cancelado. Con la capacidad aprovisionada dentro de la capa
  siempre gratuita (véase ADR-0004) esto equivale a 1–2 colocaciones por segundo
  sostenidas: un límite de escala de demostración, documentado en el README.
- **Negativas**: la traducción de `TransactionCanceledException` a errores de dominio es
  código no trivial. Está aislada en `internal/adapters/dynamo/errors.go`, de modo que
  ningún caso de uso llega a ver un tipo del SDK de AWS.
- **Negativas**: bajo contención real, el reintento acotado añade hasta unos 175 ms de
  espera antes de responder, y una colocación contendida puede llegar a emitir más de
  una llamada a `TransactWriteItems` por transacción lógica (con el consumo de capacidad
  correspondiente). Es el precio de convertir un 500 sin apuesta persistida en una
  respuesta correcta, y el presupuesto se mantiene deliberadamente pequeño porque la
  contención sobre un único elemento se resuelve en milisegundos o no se resuelve.
- **Prueba**: la garantía se verifica en dos niveles.
  `TestPlaceBet_ConcurrentDebits_NoOverdraftGivenAnAtomicPort` prueba el caso de uso
  **dado un puerto atómico**; `TestPlaceAtomically_NConcurrentGoroutines_LeavesExactBalance`
  es la prueba real: lanza 12 goroutines contra `amazon/dynamodb-local` con un saldo que
  financia exactamente N−1 apuestas y comprueba N−1 aceptadas, 1 rechazada, saldo final
  exacto y N apuestas almacenadas.
