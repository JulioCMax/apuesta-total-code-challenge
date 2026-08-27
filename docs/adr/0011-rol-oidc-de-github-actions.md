# ADR-0011: Rol OIDC de GitHub Actions para el despliegue

- **Estado**: aceptada
- **Ámbito**: `.github/workflows/deploy.yml`, `.github/workflows/ci.yml`
- **Decisiones de diseño relacionadas**: D30

## Contexto

`scripts/deploy-aws.sh` (ADR-0004) crea la infraestructura completa en AWS —tabla de
DynamoDB, rol de ejecución de IAM, función Lambda, grupo de logs y, en la reserva
descrita en ese mismo ADR, una API Gateway— y hasta ahora sólo se ejecuta a mano, con
las credenciales de AWS que el propio operador tenga resueltas localmente.

Automatizar ese despliegue desde GitHub Actions exige que el flujo de trabajo obtenga
credenciales de AWS por sí mismo. Este repositorio es **público**, lo que descarta de
entrada cualquier solución que dependa de guardar un secreto permanente: cualquier
persona puede leer el historial completo, cada *fork*, cada *pull request* y cada log de
ejecución pasado, de modo que una clave de acceso estática filtrada —por un log mal
redactado, un *fork* malicioso o un error humano— sigue siendo válida hasta que alguien
la revoque manualmente.

## Alternativas evaluadas

| Alternativa | Ventajas | Desventajas |
|---|---|---|
| **Usuario de IAM con `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY` como secretos del repositorio** | Configuración mínima; es el patrón más citado en tutoriales | La credencial es de **vida indefinida**: sigue siendo válida después de rotar el secreto en GitHub si alguien ya la copió, y su alcance depende enteramente de que nadie olvide revisar la política adjunta. En un repositorio público es un pasivo permanente que sobrevive a su propia fuga |
| Ejecutar el despliegue **sólo a mano**, sin ningún flujo de trabajo | Cero credenciales en GitHub | No es una automatización real: vuelve a depender de que el operador tenga la CLI de AWS configurada localmente y recuerde el comando exacto. No resuelve el problema que este ADR existe para resolver, sólo lo evita |
| **Asunción de rol federada por OIDC de GitHub (`aws-actions/configure-aws-credentials`)** | Las credenciales son un token emitido por AWS STS con vida de minutos, generado únicamente durante la ejecución del flujo de trabajo y nunca almacenado en ningún sitio; no hay secreto que rotar ni que pueda filtrarse de un log antiguo | Exige provisionar el proveedor OIDC y el rol de IAM una vez, con una política de confianza correctamente acotada (ver más abajo) — un error en esa condición es el error grave, no la ausencia de rotación |

## Decisión

Se adopta **asunción de rol vía OpenID Connect (OIDC)**, resuelta por
`aws-actions/configure-aws-credentials@v4` dentro de `.github/workflows/deploy.yml`. El
flujo de trabajo se dispara únicamente con `workflow_dispatch` —nunca con `push` ni
`pull_request`— y declara `environment: production`, de modo que cualquier protección de
entorno configurada en el repositorio (por ejemplo, revisores obligatorios) se aplica
antes de que se emita ninguna credencial.

### Un segundo límite de confianza, deliberadamente más amplio que el de ADR-0004

Es importante no confundir este rol con el que ADR-0004 documenta. Son dos identidades
distintas con propósitos distintos:

| | Rol de **ejecución** (ADR-0004) | Rol de **despliegue** (este ADR) |
|---|---|---|
| Quién lo asume | La función Lambda, en cada invocación | El flujo de trabajo `deploy.yml`, sólo durante el despliegue |
| Qué necesita hacer | Leer y escribir **una** tabla de DynamoDB ya existente | **Crear y modificar** la infraestructura misma: la propia tabla, el propio rol de ejecución, la propia función, sus logs y, en la reserva, la API Gateway |
| Alcance de permisos | `GetItem`, `PutItem`, `UpdateItem`, `Query`, `TransactWriteItems`, acotado al ARN exacto de la tabla y su índice | Necesariamente más amplio: `iam:CreateRole`, `iam:PutRolePolicy`, `iam:AttachRolePolicy`, `iam:GetRole` (para crear y configurar el rol de ejecución); `dynamodb:CreateTable`, `dynamodb:DescribeTable`, `dynamodb:UpdateTimeToLive`, `dynamodb:PutItem`, `dynamodb:GetItem`, `dynamodb:Query` (crear la tabla y sembrarla, verificado contra `internal/adapters/dynamo/userrepo.go` y `scripts/deploy-aws.sh`); `lambda:*` (crear, actualizar y publicar la Function URL); `logs:*` (grupo de logs y su retención); `apigatewayv2:*` (sólo se ejerce en la reserva de ADR-0004); `sts:GetCallerIdentity` (verificación previa en `scripts/lib/common.sh`) |

Que este segundo rol sea más amplio no es un descuido: es la identidad que **construye**
el despliegue, no la que el propio código desplegado usa para atender peticiones. Ambos
límites de confianza quedan documentados por separado para que una revisión de
seguridad no los confunda ni asuma que uno acota al otro.

### La condición que hace segura la política de confianza

La política de confianza del rol de despliegue debe autorizar únicamente al proveedor
OIDC de GitHub (`token.actions.githubusercontent.com`) y, dentro de él, únicamente a
este repositorio:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::<ACCOUNT_ID>:oidc-provider/token.actions.githubusercontent.com"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "token.actions.githubusercontent.com:aud": "sts.amazonaws.com"
        },
        "StringLike": {
          "token.actions.githubusercontent.com:sub": "repo:JulioCMax/apuesta-total-code-challenge:*"
        }
      }
    }
  ]
}
```

**El filo cortante de esta política es la condición `sub`, no la existencia del rol.**
GitHub incluye en cada token OIDC un reclamo `sub` con la forma
`repo:<propietario>/<repositorio>:<contexto>` (rama, entorno o *pull request*, según qué
disparó el flujo de trabajo). Si esa condición se omite o se deja como comodín total, el
rol queda abierto a **cualquier repositorio de GitHub del planeta** que sepa el ARN del
rol: el proveedor OIDC en sí mismo no distingue repositorios, sólo el `sub` que cada uno
presenta. Fijar `sub` a
`repo:JulioCMax/apuesta-total-code-challenge:*` limita la asunción a este repositorio;
acotarla más —a `repo:JulioCMax/apuesta-total-code-challenge:ref:refs/heads/main` o a
`repo:JulioCMax/apuesta-total-code-challenge:environment:production`— la limita además a
la rama principal o al entorno protegido, cerrando también la ventana de que una rama
cualquiera con `workflow_dispatch` habilitado pueda disparar un despliegue. La audiencia
(`aud`) debe ser exactamente `sts.amazonaws.com`, el valor que
`aws-actions/configure-aws-credentials` solicita por defecto.

### Separación de autoridad entre los dos flujos de trabajo (D30)

`.github/workflows/ci.yml` —que se ejecuta en cada `push` y cada `pull_request`, incluidos
los de *forks*— **no** declara el permiso `id-token` y por tanto no puede obtener ninguna
credencial de AWS bajo ninguna circunstancia, ni siquiera si un *pull request* logra
inyectar pasos adicionales. Sólo `deploy.yml`, disparado exclusivamente a mano, solicita
`id-token: write`. Esta separación entre un flujo rápido y siempre activo y uno lento,
credenciado y disparado por una persona es la misma que documenta el proyecto en su
propia arquitectura de flujos de trabajo, y evita que la superficie de un *pull request*
hostil incluya la posibilidad de desplegar nada.

## Consecuencias

- **Positivas**: no existe ningún secreto de AWS de vida larga en este repositorio
  público. Una credencial filtrada de un log de ejecución expira en minutos y sólo pudo
  haber existido durante una ejecución concreta de `deploy.yml`.
- **Positivas**: la condición `sub` deja un rastro auditable en CloudTrail —cada
  asunción de rol queda asociada al repositorio y al contexto exactos que la
  originaron— algo que una clave de acceso compartida nunca ofrece.
- **Negativas**: la puesta en marcha exige un paso manual, fuera del alcance de este
  proyecto y de cualquier automatización de SDD: crear el proveedor OIDC (si la cuenta
  no lo tiene ya) y el rol de IAM con la política de confianza de arriba, y definir las
  variables `AWS_DEPLOY_ROLE_ARN` y `AWS_REGION` en la configuración del repositorio de
  GitHub. Ningún script de este repositorio crea ese rol: documentarlo aquí es
  deliberado, en lugar de automatizarlo, exactamente por la misma razón que
  `scripts/deploy-aws.sh` nunca se ejecuta por sí solo (véase su comentario de cabecera).
- **Negativas**: el rol de despliegue es, por necesidad, más amplio que el rol de
  ejecución de ADR-0004. Un error en su política de confianza —una condición `sub` mal
  escrita o ausente— es más grave que el equivalente en el rol de ejecución, porque su
  alcance de permisos es mayor. Es el precio explícito de automatizar la propia
  construcción de la infraestructura, y la razón por la que esta decisión documenta la
  condición `sub` con el detalle de arriba en lugar de darla por sabida.
- **Neutras**: si en el futuro se necesitara reducir aún más esa superficie, el siguiente
  paso natural sería separar la creación de infraestructura (`iam:CreateRole`,
  `dynamodb:CreateTable`, …) de las actualizaciones rutinarias
  (`lambda:UpdateFunctionCode`), con dos roles distintos. Se descarta por ahora: añadiría
  una segunda política de confianza que mantener sincronizada a cambio de un beneficio
  que sólo se materializa si el rol de despliegue se ve comprometido, y la condición
  `sub` ya es la barrera que importa.
