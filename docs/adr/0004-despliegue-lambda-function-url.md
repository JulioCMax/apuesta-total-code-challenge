# ADR-0004: Despliegue en AWS Lambda (ZIP arm64) con Function URL

- **Estado**: aceptada
- **Ámbito**: `cmd/api/main.go`, `scripts/deploy-aws.sh`, `Dockerfile`
- **Decisiones de diseño relacionadas**: D12

## Contexto

El despliegue debe realizarse sobre AWS (stack de la empresa) con un presupuesto máximo
de 5 USD al mes y objetivo de 0 USD. El servicio es un único binario de Go sin estado
propio: todo el estado mutable vive en DynamoDB (ADR-0002).

Un segundo requisito, implícito pero relevante para la evaluación, es que la
arquitectura de la demostración se pueda relacionar con la infraestructura real de la
empresa, basada en EKS.

## Alternativas evaluadas

| Alternativa | Coste mensual estimado | Valoración |
|---|---|---|
| **EKS** | Solo el plano de control: 0.10 USD/hora ≈ **73 USD/mes** por clúster | Descartada: aproximadamente 15 veces el tope de presupuesto. Se documenta el mapeo narrativo en el README en lugar de desplegarla |
| ECS Fargate (tarea siempre activa) | ≈ 9 USD/mes | Descartada: supera el tope |
| App Runner | ≈ 5.04 USD/GB-mes solo en reposo | Descartada: iguala o supera el tope sin tráfico alguno |
| Lightsail | 3.50–5 USD/mes | Viable como alternativa, pero consume casi todo el presupuesto y encaja peor en el relato de arquitectura |
| EC2 capa gratuita | 0 USD durante un periodo limitado | Descartada: la capa gratuita está acotada en tiempo o en créditos bajo ambos regímenes de facturación |
| Lambda como imagen de contenedor | 0 USD de cómputo | Descartada: exige ECR, cuya capa gratuita de repositorio privado dura solo 12 meses, no de forma permanente |
| **Lambda ZIP nativo (`provided.al2023`, arm64) + Function URL** | **0 USD** dentro de la capa siempre gratuita | Adoptada |

### Cómo se expone la función a internet

La tabla anterior compara destinos de cómputo. Falta la pregunta que hace todo el mundo
al ver una Lambda: **¿no necesita una API Gateway delante?**

Hasta abril de 2022, sí: hacía falta API Gateway o un balanceador. Desde entonces existen
las **Function URLs**, un extremo HTTPS propio de la función.

| Alternativa | Coste | Valoración |
|---|---|---|
| **API Gateway (HTTP API)** | 1 USD por millón de peticiones; capa gratuita de **12 meses**, no permanente | No adoptada como camino principal: es el único componente de esta arquitectura cuya capa gratuita caduca, lo que rompería la propiedad de 0 USD permanentes. Se conserva como **reserva** (ver más abajo) |
| API Gateway (REST API) | 3.50 USD por millón | Descartada: más cara y no aporta nada que la HTTP API no dé aquí |
| Balanceador de aplicación (ALB) | ≈ 16 USD/mes solo por existir | Descartada: triplica el tope de presupuesto |
| **Function URL** | **0 USD**, permanente | Adoptada |

Además del coste, la Function URL evita **duplicar la tabla de rutas**. El router de Gin
ya la tiene; con API Gateway habría que declarar un `{proxy+}` que sólo reenvía —y
entonces no aporta nada— o repetir cada ruta en la infraestructura y convivir con el
riesgo de que el código y el despliegue se desincronicen.

**Lo que se cede al no usar API Gateway**, y conviene tenerlo escrito:

- **WAF**: no se puede asociar a una Function URL; API Gateway y ALB sí lo admiten.
- **Dominio propio**: no es nativo; requeriría CloudFront por delante.
- **Límite de tasa, planes de uso y claves de API**: los ofrece API Gateway. Aquí el
  límite de tasa lo aplica la propia aplicación (`middleware.RateLimit`), con la
  limitación por instancia que el README documenta.
- **Autorizadores gestionados** (Cognito, autorizador Lambda): sólo hay `NONE` o
  `AWS_IAM`. La autorización la resuelve el JWT de la aplicación.
- **Validación y transformación de peticiones**: no existen; el binario valida.

### Reserva: API Gateway cuando la cuenta restringe las Function URLs

Verificado en una cuenta real: **la misma función respondía 403 a través de su Function
URL y 200 a través de una HTTP API Gateway**, con una política de recurso correcta
(`Principal: "*"`, `lambda:InvokeFunctionUrl`, condición `FunctionUrlAuthType=NONE`),
sin ningún SCP ni RCP en la organización, y con la función devolviendo 200 por invocación
directa. La señal que lo delata es `ConcurrentExecutions: 10` en
`aws lambda get-account-settings`, frente al valor por defecto de 1000: la cuenta está en
estado restringido.

Por eso `scripts/deploy-aws.sh` **intenta primero la Function URL** y sólo crea la API
Gateway si aquella no responde 200. Una cuenta sin restricción nunca crea el recurso de
pago y conserva los 0 USD permanentes; una cuenta restringida obtiene un despliegue que
funciona en lugar de un 403 inexplicable. El despliegue informa cuál de las dos puertas
acabó usando, y `scripts/destroy-aws.sh` elimina la API Gateway antes que la función.

## Decisión

Se adopta **AWS Lambda con paquete ZIP nativo sobre `provided.al2023`, arquitectura
arm64, expuesto mediante una Function URL**.

- **Empaquetado**: `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -o
  bootstrap ./cmd/api`, comprimido con el binario llamado exactamente `bootstrap` en la
  raíz del ZIP. Memoria 512 MB, tiempo de espera 10 s.
- **Sin ECR**: el ZIP se sube directamente a Lambda. La capa gratuita de repositorio
  privado de ECR caduca a los 12 meses; la de Lambda y DynamoDB es permanente.
- **Un único router en ambos entornos**: `cmd/api/main.go` construye el mismo
  `*gin.Engine` en local y en AWS, y la rama entre entornos ocupa unas diez líneas:

  ```go
  if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
      lambda.Start(ginadapter.NewV2(engine).ProxyWithContext)
      return
  }
  ```

- **`AWS_LAMBDA_FUNCTION_NAME` como interruptor (D12)**: es una variable reservada que
  el entorno de ejecución de Lambda siempre define y que un proceso local nunca tiene.
  Una variable propia del tipo `RUNTIME_MODE` sería configurable y, por tanto,
  susceptible de quedar mal configurada.
- **`ginadapter.NewV2`**: la Function URL entrega la carga útil en formato API Gateway
  v2. Un adaptador que solo entienda v1 interpreta mal la ruta de la solicitud de forma
  silenciosa; la prueba de humo (`scripts/smoke.sh`) contra la URL desplegada es la
  salvaguarda frente a ese fallo.
- **`--auth-type NONE` compensado en la aplicación**: la Function URL es pública, y la
  autorización se resuelve en la propia aplicación con JWT sobre todas las rutas que
  modifican estado. La alternativa (`AWS_IAM`) exigiría que el evaluador firmara cada
  solicitud con SigV4, lo que impediría probar la API con `curl` o con la interfaz de
  Swagger.

## Consecuencias

- **Positivas**: coste efectivo de 0 USD dentro de la capa siempre gratuita (1 millón de
  invocaciones y 400 000 GB-s al mes en Lambda; 25 GB, 25 RCU y 25 WCU en DynamoDB;
  5 GB al mes en CloudWatch Logs), sin límite temporal.
- **Positivas**: arm64 (Graviton) ofrece mejor relación precio/rendimiento y un binario
  estático de Go compila sin fricción para esa arquitectura.
- **Negativas**: **arranque en frío**. Tanto Lambda como una tabla poco solicitada
  tienen latencia adicional en la primera solicitud tras un periodo de inactividad. Es
  comportamiento esperado de una arquitectura que escala a cero, no un defecto: se
  documenta en el README y no se intenta mitigar con instancias aprovisionadas, que
  tendrían coste.
- **Negativas**: la limitación de tasa por IP es **por instancia de Lambda**, no global.
  Con varias instancias concurrentes el límite efectivo se multiplica. Es una
  simplificación conocida y declarada; una solución global requeriría un almacén
  compartido (véase ADR-0006 sobre por qué no se añade ElastiCache).
- **Neutras**: `scripts/deploy-aws.sh` es idempotente y puede reejecutarse sin efectos
  secundarios; `scripts/destroy-aws.sh` elimina lo creado. La reversión en la nube
  consiste en borrar la función y la tabla, ambas recreables con un solo comando.
- El README incluye una sección dedicada al mapeo entre esta demostración y un
  despliegue real sobre EKS, para dejar constancia de que la elección responde al
  presupuesto y no al desconocimiento de la plataforma de destino.
