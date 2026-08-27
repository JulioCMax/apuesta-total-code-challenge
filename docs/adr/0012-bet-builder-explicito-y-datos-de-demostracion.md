# ADR-0012: Bet Builder como excepción explícita, y los datos autorados que lo hacen demostrable

- **Estado**: aceptada
- **Ámbito**: `internal/domain/betslip/betslip.go`,
  `internal/adapters/memory/seed/superodds.go`,
  `internal/adapters/memory/seed/betbuilder.go`,
  `internal/adapters/memory/loader.go`, `internal/adapters/http/dto/`

## Contexto

El servicio ya rechazaba con `SAME_EVENT_COMBO` cualquier combinada que incluyera dos
selecciones del mismo evento. **Bet Builder** es la única excepción legítima a esa regla:
permite combinar varias selecciones de un mismo partido —por ejemplo, ganador del partido
más «ambos equipos anotan»— cuando el evento lo admite.

Al implementarlo aparecieron dos problemas que exigen una decisión explícita, y están
encadenados:

**1. ¿Cómo se activa la excepción?** El dato lleva la señal `Settings.IsBetBuilderEnabled`
por evento, y la tentación evidente es usarla sola: si el evento lo admite, se permite la
combinada del mismo evento. Pero **los 24 eventos de `data.json` traen esa bandera en
`true`**. Deducir la elegibilidad únicamente de la bandera volvería la regla original
inalcanzable: ninguna solicitud podría producir jamás un `SAME_EVENT_COMBO`, y la prueba
que lo verifica pasaría a ser código muerto que da falsa seguridad. Una regla de negocio
que sólo existe en su prueba no existe.

**2. Si todos los eventos lo admiten, ¿cómo se demuestra la puerta?** El problema es
simétrico. Con los 24 eventos habilitados, el rechazo `BET_BUILDER_NOT_AVAILABLE` es una
rama inalcanzable contra el catálogo real: se puede probar con dobles de prueba, pero no
se puede enseñar. Lo mismo ocurre con **Super Cuota**: el conjunto de datos no trae
ninguna señal de promoción ni de cuota mejorada, de modo que sin un dato adicional la
funcionalidad no tiene nada que mostrar.

Y sobre ambos pesa una restricción que no se negocia: **`data.json` es el conjunto de
datos de referencia del reto y debe permanecer intacto**, para que cualquiera pueda
comparar lo entregado con lo recibido byte a byte.

## Alternativas evaluadas

| Alternativa | Ventajas | Desventajas |
|---|---|---|
| Deducir Bet Builder de `IsBetBuilderEnabled`, sin activación explícita | Ninguna bandera nueva en la API; menos superficie | Vuelve inalcanzable el `SAME_EVENT_COMBO` con este dataset y convierte su prueba en código muerto. Además, quien agrega dos selecciones de un partido sin saberlo recibiría una combinada distinta de la que pidió |
| Activación explícita, ignorando la bandera del evento | Simple de explicar | Concede al cliente una decisión que le corresponde al catálogo: un evento sin mercados compatibles admitiría igualmente la combinada |
| **Exigir ambas: activación explícita `isBetBuilder` y bandera del evento** | Conserva intacta la regla original; el cliente declara la intención y el catálogo declara la capacidad | Requiere una bandera nueva en el contrato y un código de error propio |
| Modificar `data.json` para deshabilitar algunos eventos y mejorar algunas cuotas | Un solo origen de datos, sin superposiciones | Altera el material de referencia del reto: se pierde la comparación byte a byte y cualquier hallazgo posterior queda bajo sospecha de haber sido inducido |
| No agregar dato alguno y dejar las ramas sin demostrar | Ningún dato inventado | `BET_BUILDER_NOT_AVAILABLE` nunca se produce contra el catálogo real y Super Cuota no tiene nada que mostrar: dos funcionalidades imposibles de evaluar |
| **Superposiciones autoradas en `seed/`, con `data.json` intacto** | El dato de origen no se toca; el dato agregado queda confinado, comentado y cubierto por pruebas | Introduce dato inventado, que debe declararse como tal de forma visible |

## Decisión

### Bet Builder exige dos condiciones, no una

Una combinada del mismo evento se acepta **sólo** cuando se cumplen ambas:

1. La solicitud envía `isBetBuilder: true` de forma explícita. Omitirlo equivale a
   `false`, que es el valor cero de Go y no necesita etiqueta de vinculación alguna.
2. **Cada** evento repetido en el boleto trae `IsBetBuilderEnabled` en `true`.

Los dos rechazos son distintos a propósito:

| Situación | Respuesta |
|---|---|
| Evento repetido, sin `isBetBuilder` | `422 SAME_EVENT_COMBO` — la regla original, intacta, cualquiera que sea la bandera del evento |
| Evento repetido, con `isBetBuilder`, sobre un evento que no lo admite | `422 BET_BUILDER_NOT_AVAILABLE` |

Quien activó el interruptor pidió algo concreto y merece una respuesta concreta. Degradar
ese caso al genérico `SAME_EVENT_COMBO` le diría que la combinada es imposible, cuando en
realidad lo que ocurre es que **ese** evento no la admite.

**Cada** evento duplicado se verifica, no sólo el primero. Un boleto puede combinar
legítimamente pares del mismo evento de más de un partido, y basta que uno de ellos no lo
admita para rechazar el conjunto. Consultar sólo el primer duplicado hallado hacía que el
resultado dependiera del orden en que llegaran las selecciones y dejaba pasar sin revisar
el par de un evento deshabilitado; fue un defecto detectado en revisión adversarial y
corregido. Los duplicados se recorren en **orden de selección**, de modo que el evento
nombrado en `details.eventId` es siempre el primero que **impide** la operación, de forma
determinista.

La activación debe reflejar una acción deliberada de la persona que apuesta —el
interruptor «Bet Builder» del cupón—, nunca deducirse de que el boleto contenga dos
selecciones del mismo partido. Un cliente que la infiera convierte un descuido en una
apuesta distinta de la que se quiso hacer.

### `data.json` no se modifica; el dato de demostración vive en superposiciones

Se agregan dos mapas al paquete de siembra, siguiendo exactamente el patrón que
`seed/groups.go` ya había establecido (ADR-0008): dato autorado, confinado a un archivo,
con su procedencia escrita en su propio comentario de cabecera y sostenida por pruebas.

| Archivo | Qué contiene | Para qué existe |
|---|---|---|
| `seed/superodds.go` | Cinco selecciones reales con su cuota mejorada | Da a Super Cuota algo que mostrar. `loader.go` sustituye `Odds` por la cuota mejorada y guarda la anterior en `OriginalOdds`, de modo que la interfaz pueda decir «antes 1.56, ahora 1.70» con ambos números a la vista |
| `seed/betbuilder.go` | Dos eventos —*Catar vs Suiza* y *Haití vs Escocia*— tratados como no aptos para Bet Builder | Hace alcanzable `BET_BUILDER_NOT_AVAILABLE` contra el catálogo real. Sin al menos un evento deshabilitado, la puerta de la funcionalidad no se puede demostrar de extremo a extremo |

Ambas superposiciones se aplican **después** de decodificar el archivo, en `loader.go`.
Los bytes de `data.json` no se tocan: `betBuilderEnabled := re.Settings.IsBetBuilderEnabled
&& !seed.BetBuilderDisabled[re.ID]`. El origen sigue siendo comparable byte a byte con el
material entregado, y la diferencia entre lo recibido y lo servido queda escrita en dos
archivos de Go legibles, no escondida dentro de un JSON de miles de líneas.

Las cinco selecciones mejoradas pertenecen a cinco partidos distintos, y **ninguno de
ellos coincide con los dos eventos deshabilitados**: las dos demostraciones nunca se pisan
sobre la misma prueba.

Cada mapa está sostenido por pruebas guardianas construidas sobre el modelo de
`TestGroupSeed_CoversEveryEvent`: `TestSuperCuotaSeed_CoversRealSelections` y
`TestBetBuilderDisabledSeed_CoversRealEvents` fallan si un identificador deja de
corresponder a un dato real; `TestSuperCuotaSeed_EveryBoostExceedsOriginal` falla si una
supuesta mejora empeora la cuota; `TestBetBuilderDisabledSeed_ActuallyDisablesTheEvent`
comprueba que la superposición llega efectivamente al catálogo cargado; y
`TestBetBuilderDisabledSeed_NeverCollidesWithSuperCuota` mantiene separadas ambas
demostraciones.

## Consecuencias

- **Positivas**: la regla `SAME_EVENT_COMBO` sigue siendo alcanzable y su prueba sigue
  siendo una prueba. La excepción no la reemplaza: convive con ella, y cuál de las dos se
  aplica lo decide una elección explícita del cliente.
- **Positivas**: las dos ramas del Bet Builder —aceptada y rechazada— se pueden ejecutar
  contra el catálogo real, desde `curl` o desde Swagger UI, con identificadores del propio
  conjunto de datos. Una funcionalidad que sólo se puede probar con dobles de prueba no se
  puede enseñar.
- **Positivas**: `data.json` permanece idéntico al archivo recibido, de modo que la
  comparación con el material de referencia sigue siendo válida.
- **Negativas, y conviene decirlo sin rodeos**: **los dos eventos deshabilitados y las
  cinco cuotas mejoradas son datos inventados para la demostración**. El conjunto de datos
  original no trae ninguna señal de promoción, y en él ningún evento tiene el Bet Builder
  deshabilitado. En producción, ambas señales llegarían del proveedor de datos: la
  capacidad de Bet Builder por evento y las cuotas promocionales son decisiones de trading,
  no del servicio. Al vivir en el adaptador de memoria, sustituirlas por un origen real es
  cambiar de adaptador, no de dominio.
- **Negativas**: el contrato de la API gana una bandera y un código de error más, que todo
  cliente debe conocer. Es el precio de que la excepción sea explícita en lugar de
  implícita, y queda documentado en `api/openapi.yaml` y en la tabla de errores del README.
- **Neutras**: `OriginalOdds` se omite por completo cuando no hay mejora, en lugar de
  enviarse como `null`, siguiendo el mismo criterio que ya se aplicaba a `group`
  (ADR-0008). La ausencia del campo significa «sin promoción», sin necesidad de comparar
  dos números.
- **Neutras**: la valoración se hace siempre con la cuota mejorada, que es la única que el
  dominio conoce. `OriginalOdds` es información para mostrar, nunca un valor de cálculo, de
  modo que no existe forma de que la promoción y el importe pagado discrepen.
