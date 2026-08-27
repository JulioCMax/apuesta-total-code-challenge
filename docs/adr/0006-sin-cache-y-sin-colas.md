# ADR-0006: Sin caché y sin colas

- **Estado**: aceptada
- **Ámbito**: arquitectura completa
- **Decisiones de diseño relacionadas**: D14

## Contexto

El enunciado del reto pregunta de forma explícita por el "uso (o no) de caché y colas".
La respuesta esperada no es una lista de tecnologías añadidas, sino una decisión
razonada: qué problema resolvería cada una en **este** sistema y si ese problema existe.

Conviene, por tanto, examinar los dos candidatos por separado y contra el
comportamiento real del servicio.

## Alternativas evaluadas

### Caché

| Alternativa | Qué resolvería aquí | Coste real |
|---|---|---|
| ElastiCache (Redis) para el catálogo de eventos | Nada: el catálogo ya está en memoria del proceso | El propio clúster: un `cache.t4g.micro` encendido de forma permanente ronda los **12 USD/mes**, más del **doble** del tope de presupuesto completo. Su capa gratuita dura 12 meses, no es permanente (mismo motivo por el que el ADR-0004 descarta ECR) |
| ElastiCache para el saldo | Empeoraría la corrección: introduciría una copia del saldo fuera de la transacción atómica | Mismo coste del clúster, con un riesgo de consistencia añadido |
| Caché HTTP (`Cache-Control`) en las rutas de catálogo | Reduciría solicitudes repetidas desde un mismo cliente | Coste cero, pero no aporta a lo evaluado; se anota como mejora trivial, no se implementa |

Una caché resuelve el problema de una lectura **cara**. En este servicio la lectura del
catálogo es un acceso a un mapa en memoria del propio proceso: microsegundos, sin salto
de red, sin serialización. Poner Redis delante de eso añadiría latencia de red a una
operación que hoy no la tiene, además de una infraestructura con coste. La lectura del
saldo, la única que sí atraviesa la red, **no debe** cachearse: es exactamente el valor
cuya exactitud garantiza la condición de la transacción (ADR-0003).

**Sobre la VPC que ElastiCache arrastra**, conviene ser preciso en lugar de repetir el
argumento genérico. ElastiCache solo es alcanzable desde dentro de una VPC, así que sí
obligaría a adjuntar la función a una. Ahora bien:

- **La puerta de entrada no se ve afectada.** Una Lambda en VPC se sigue invocando a
  través del plano de control del servicio Lambda, no por la red de la VPC. Tanto la
  Function URL como la API Gateway de reserva (ADR-0004) funcionan igual con una función
  adjunta a una VPC: el NAT gateway es un asunto de **salida**, no de entrada.
- **El NAT gateway sería evitable aquí.** La única dependencia saliente del binario es
  DynamoDB, y DynamoDB dispone de un *Gateway VPC Endpoint* **sin coste** —ni por hora
  ni por datos procesados—; CloudWatch Logs sigue funcionando porque los entrega el
  propio servicio Lambda y no la interfaz de red de la función. Un NAT gateway
  (≈ 32 USD/mes) solo haría falta para un tercer destino en internet que este servicio
  no tiene.

Es decir: el coste que descarta ElastiCache es **el del propio clúster**, no el de una
infraestructura de red que podría evitarse. El argumento económico es más pequeño de lo
que sugeriría la cifra del NAT, y precisamente por eso es más sólido: no depende de un
componente que un lector informado señalaría como innecesario.

### Colas

| Alternativa | Qué resolvería aquí | Por qué se descarta |
|---|---|---|
| SQS delante de la colocación de la apuesta | Absorbería picos de escritura | La colocación **debe** ser síncrona: el usuario necesita saber en la misma respuesta si su apuesta fue aceptada y cuál es su saldo. Con una cola de por medio haría falta, además, un endpoint de consulta del estado, y la escritura seguiría necesitando exactamente la misma escritura condicional atómica: la cola no elimina el problema de concurrencia, solo lo desplaza |
| SQS/EventBridge **después** de una colocación aceptada | Difusión de eventos hacia consumidores externos (notificaciones, analítica, liquidación) | Es un caso de uso legítimo, pero no hay ningún consumidor dentro del alcance. Se documenta como mejora futura en ADR-0009 |

## Decisión

**No se incorpora ninguna caché ni ningún sistema de colas.**

La justificación se apoya en tres argumentos independientes, y cualquiera de ellos
bastaría por sí solo:

1. **Técnico**: los datos de solo lectura ya residen en la memoria del proceso; el dato
   mutable es precisamente el que no debe cachearse.
2. **De diseño**: la colocación de una apuesta es una operación síncrona por naturaleza,
   ya que el usuario necesita una confirmación inmediata del débito. Una cola no
   eliminaría la necesidad de la escritura condicional atómica.
3. **Económico**: el clúster de ElastiCache más pequeño encendido de forma permanente
   ronda los 12 USD/mes, más del doble del presupuesto total del proyecto y con una capa
   gratuita que caduca a los 12 meses, para resolver un problema que no existe.

## Consecuencias

- **Positivas**: menos componentes que desplegar, monitorizar y explicar. El coste
  mensual se mantiene en 0 USD (ADR-0004) y no se introduce ninguna fuente de
  inconsistencia entre una copia cacheada y el dato autoritativo.
- **Negativas**: no hay amortiguación ante un pico de escrituras. Con la capacidad
  aprovisionada actual el límite práctico ronda 1–2 colocaciones por segundo sostenidas.
  A escala real, la respuesta correcta sería subir la capacidad de la tabla o pasar a
  modo bajo demanda, no interponer una cola: el cuello de botella es la escritura
  condicional, y esa escritura debe ocurrir igualmente.
- **Neutras**: si en el futuro apareciera una lectura genuinamente cara (por ejemplo, un
  catálogo servido por un proveedor externo con latencia), el punto de inserción de una
  caché ya está preparado: sería otra implementación del puerto `EventCatalog`, sin
  tocar ningún caso de uso (ADR-0002).
- **Neutras**: la difusión de eventos tras una colocación aceptada queda registrada como
  mejora futura, con su punto de conexión exacto documentado (ADR-0009).
