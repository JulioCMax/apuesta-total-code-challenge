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
