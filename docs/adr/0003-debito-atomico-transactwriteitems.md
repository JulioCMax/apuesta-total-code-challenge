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
| Contador de versión optimista con reintentos | Sin transacción explícita | Bucle de reintento propio, más código y más pruebas para un conflicto de una sola fila; el resultado final es el mismo que ofrece la condición del propio motor |
| **`TransactWriteItems` con condición sobre el saldo** | Todo o nada en un único viaje; la condición se evalúa **dentro** de la transacción, por lo que nunca se lee un saldo obsoleto | El código del SDK es más verboso que su equivalente en SQL |

## Decisión

Se adopta **`TransactWriteItems`**. La colocación de una apuesta se resuelve en como
máximo dos transacciones, y el orden de los elementos es significativo porque DynamoDB
devuelve los motivos de cancelación en paralelo al índice de cada elemento.

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

Tres decisiones asociadas completan el comportamiento:

- **Elemento de idempotencia independiente (D9)**: la clave se guarda como
  `IDEMP#<key>` con su propio TTL de 24 horas, y el identificador de la apuesta es un
  ULID nuevo. Derivar el identificador de la apuesta de una cabecera enviada por el
  cliente habría acoplado la identidad del recurso a un valor externo y habría roto el
  orden cronológico natural del `SK`.
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
- **Prueba**: la garantía se verifica en dos niveles.
  `TestPlaceBet_ConcurrentDebits_NoOverdraftGivenAnAtomicPort` prueba el caso de uso
  **dado un puerto atómico**; `TestPlaceAtomically_NConcurrentGoroutines_LeavesExactBalance`
  es la prueba real: lanza 12 goroutines contra `amazon/dynamodb-local` con un saldo que
  financia exactamente N−1 apuestas y comprueba N−1 aceptadas, 1 rechazada, saldo final
  exacto y N apuestas almacenadas.
