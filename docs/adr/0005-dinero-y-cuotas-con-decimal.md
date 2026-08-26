# ADR-0005: Dinero y cuotas con aritmética decimal exacta y un único punto de redondeo

- **Estado**: aceptada
- **Ámbito**: `internal/domain/money/`
- **Decisiones de diseño relacionadas**: D6, D7, D13

## Contexto

Todos los importes del sistema (monto apostado, saldo, retorno potencial) y todas las
cuotas atraviesan una cadena de multiplicaciones cuyo resultado se muestra al usuario y
se compara con el saldo almacenado. El reto evalúa explícitamente el redondeo de los
retornos potenciales.

En el dataset de origen, `TrueOdds` llega ya con precisión de dos decimales, y el
resultado esperado de una combinada es el producto de las cuotas redondeado, no el
producto exacto sin redondear.

## Alternativas evaluadas

| Alternativa | Ventajas | Desventajas |
|---|---|---|
| `float64` en todo el sistema | Lo más simple de escribir | La representación binaria no puede expresar exactamente valores como `0.1` o `33.10`; el error se acumula en cada multiplicación. Un `stake` de `33.10` puede convertirse en `33.099999999999994` antes de cualquier cálculo. Inaceptable para dinero |
| Enteros de céntimos con `math.Round` manual | Almacenamiento exacto | El modo de redondeo se implementa a mano en cada punto de llamada; cualquier omisión introduce una discrepancia silenciosa |
| **`shopspring/decimal` con un objeto de valor `Money` y un único `Round2`** | Aritmética decimal exacta; un solo punto de redondeo que se puede probar de forma aislada | Una dependencia adicional (pequeña y muy consolidada) |

## Decisión

Se adopta **`shopspring/decimal`** encapsulado en dos objetos de valor del dominio,
`money.Money` y `money.Odds`, con una única función de redondeo en todo el código:

```go
// Round2 es el ÚNICO punto de redondeo del código (D6).
func Round2(d decimal.Decimal) decimal.Decimal {
    return d.Round(2)
}
```

Reglas asociadas:

- **Ningún otro paquete llama a `decimal.Round` directamente.** Toda operación que
  produzca un `Money` o un `Odds` pasa por `Round2`. Al existir un solo punto de
  redondeo, basta una prueba para demostrar su corrección
  (`TestRound2_HalfUpBoundaries`, con los casos `1.005`, `2.675`, `0.005`,
  `152.494999` y `152.495`).
- **`Round` de `shopspring/decimal` redondea alejándose de cero**, lo que equivale a
  HALF-UP para los valores no negativos que `Money` y `Odds` siempre contienen. La
  equivalencia se afirma en una prueba en lugar de darse por supuesta.
- **Orden de cálculo fijo (D7)**: `combinedOdds = Round2(∏ cuotas)` y, a continuación,
  `potentialReturns = Round2(stake × combinedOdds)`. Calcular el retorno a partir del
  producto sin redondear produciría un importe que no cuadra con la cuota mostrada al
  usuario. Ejemplo verificado: `4.34 × 3.58 = 15.5372` → `combinedOdds = 15.54` →
  `potentialReturns = 1554.00` sobre un monto de `100.00`.
- **El monto apostado se lee como `json.Number`, no como `float64`.** El DTO
  (`internal/adapters/http/dto/request.go`) conserva los dígitos literales enviados por
  el cliente y los entrega a `decimal.NewFromString`: un `stake` de `33.10` nunca se
  convierte en `33.099999999999994` durante el binding.
- **Serialización JSON como número sin comillas y con dos decimales (D13)**:
  `"stake": 100.00`. Se eligió por legibilidad para el evaluador y por simetría con el
  formato numérico de `TrueOdds` en el dataset de origen. La alternativa de producción
  (importes como cadena, para blindarse frente a clientes que los parsean como
  `double`) se documenta en el README como el siguiente paso natural.
- **En DynamoDB el dinero se almacena como `Number`** (decimal exacto de hasta 38
  dígitos), de modo que la condición `balance >= :stake` de la transacción es una
  comparación numérica exacta, sin coma flotante en ningún punto del recorrido.

## Consecuencias

- **Positivas**: se elimina una clase entera de defectos (deriva por coma flotante) y el
  redondeo queda demostrado por una sola prueba sobre una sola función.
- **Positivas**: la envolvente `Money` impide construir un importe negativo
  (`NewMoney` devuelve `ErrNegativeAmount`) y `Odds` impide una cuota inferior a `1.01`
  (`ErrOddsTooLow`), de modo que ambas invariantes se cumplen por construcción.
- **Negativas**: la aritmética decimal es más lenta que la de coma flotante nativa. A
  esta escala (unas pocas multiplicaciones por solicitud) la diferencia no es medible.
- **Negativas**: `decimal.Decimal` mantiene campos no exportados, por lo que tanto
  `MarshalJSON` como la conversión hacia DynamoDB deben implementarse a mano; sin ellas,
  la serialización por reflexión emitiría `{}` o una estructura interna. Ambas están
  cubiertas por pruebas propias.
- **Neutras**: `Odds` con un mínimo de `1.01` obligó a tomar una decisión explícita sobre
  las selecciones del dataset cuya cuota está por debajo de ese mínimo; se detalla en el
  README, en la sección de límites y procedencia de los datos.
