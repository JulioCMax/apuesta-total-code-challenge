# ADR-0009: Publicación de eventos diferida (EventBridge / SNS)

- **Estado**: aceptada — mejora futura, deliberadamente no implementada
- **Ámbito**: `internal/application/betslip/place.go` (punto de conexión documentado)

## Contexto

En una plataforma de apuestas real, una colocación aceptada interesa a varios
consumidores además del que realiza la solicitud: notificaciones al usuario, analítica
casi en tiempo real, detección de fraude, el proceso de liquidación posterior al partido
y la exposición de métricas de negocio.

El patrón habitual en AWS para ese reparto es publicar un evento de dominio
(`BetPlaced`) en **EventBridge** o en **SNS**, y que cada consumidor se suscriba de forma
independiente sin que el servicio que origina el evento los conozca.

La pregunta que resuelve este ADR es si conviene implementarlo ahora.

## Alternativas evaluadas

| Alternativa | Ventajas | Desventajas |
|---|---|---|
| Implementar la publicación en EventBridge, con consumidores | Demuestra el patrón completo de extremo a extremo | No existe ningún consumidor dentro del alcance; habría que inventar uno artificial, además de coste y superficie de despliegue adicionales |
| Definir una interfaz `EventPublisher` con una implementación vacía | Deja el "hueco" visible en el código | Abstracción sin consumidor real: una interfaz cuya única implementación es un `no-op` es código muerto que aparenta funcionalidad. Contradice el principio de no anticipar necesidades |
| Publicar en local mediante un emulador | Mantendría la simetría entre local y nube | La emulación local de EventBridge es débil y no equivalente a la real; se estaría demostrando el emulador, no la integración |
| **Documentar la mejora, con su punto de conexión exacto, sin implementarla** | Coste cero; deja constancia de que el patrón se conoce y de por qué se pospone | Requiere que la documentación sea precisa; una mención vaga no aportaría nada |

## Decisión

**No se implementa.** Se documenta como mejora futura, con el punto de conexión
identificado con precisión.

**Punto de conexión**: inmediatamente después de que `betslip.Place` obtenga una apuesta
almacenada con `status = accepted` y antes de devolver el resultado al handler. En ese
instante la transacción ya está confirmada, de modo que el evento describiría un hecho
consumado y no una intención.

**Forma que tendría el cambio**, con una estimación honesta del alcance:

1. Declarar el puerto en `internal/application/betslip/ports.go`, siguiendo la misma
   convención de interfaces propiedad del consumidor que el resto del proyecto:

   ```go
   type EventPublisher interface {
       PublishBetPlaced(ctx context.Context, b betslip.Bet) error
   }
   ```

2. Añadir un adaptador `internal/adapters/eventbridge/` que implemente ese puerto.
3. Inyectarlo en `cmd/api/main.go` (una línea) y llamarlo desde `Place`.
4. Añadir a la política de IAM el permiso `events:PutEvents` acotado al bus de destino.

**Semántica de fallo que exigiría la implementación**: la publicación **no puede** hacer
fallar la colocación. La apuesta ya está confirmada y el saldo ya está debitado; devolver
un error porque la difusión falló informaría al cliente de un fallo inexistente y
provocaría un reintento que, sin `Idempotency-Key`, colocaría una segunda apuesta. El
tratamiento correcto es registrar el fallo y continuar, con reintento asíncrono si se
requiere una garantía de entrega más fuerte. Es el mismo criterio ya aplicado en el
código actual a la lectura del saldo posterior a la colocación, que se resuelve como
`balanceAfter: null` en lugar de convertir un error cosmético en un fallo de la
operación.

## Consecuencias

- **Positivas**: no se añade una abstracción sin uso. La arquitectura hexagonal ya
  garantiza que incorporarla más adelante sea un puerto nuevo y un adaptador nuevo, sin
  modificar el dominio ni el resto de los casos de uso.
- **Positivas**: coste de infraestructura y de despliegue sin cambios (ADR-0004).
- **Negativas**: el proyecto no demuestra de forma ejecutable una integración con
  mensajería. Se compensa dejando por escrito el punto de conexión, la forma del cambio
  y la semántica de fallo, que es donde reside la decisión de diseño relevante.
- **Neutras**: el diagrama de arquitectura (`docs/diagrams/arquitectura.svg`) representa
  esta ruta con una flecha discontinua y la etiqueta "mejora futura, no implementada",
  para que la ausencia sea una decisión visible y no un olvido.
