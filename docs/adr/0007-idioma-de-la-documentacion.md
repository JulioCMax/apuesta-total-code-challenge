# ADR-0007: Idioma de la documentación y del código

- **Estado**: aceptada
- **Ámbito**: `README.md`, `docs/adr/`, `api/openapi.yaml`, mensajes de error de la API, código fuente

## Contexto

El proyecto tiene dos audiencias con necesidades distintas:

- **El equipo evaluador**, que lee la documentación para entender qué se construyó, por
  qué y cómo ejecutarlo. Es un equipo hispanohablante y el enunciado del reto está
  redactado en español.
- **Cualquier persona que lea o mantenga el código**, para quien las convenciones de la
  industria en Go, en HTTP y en AWS son en inglés: nombres de paquetes, identificadores,
  rutas, campos JSON, variables de entorno y códigos de error.

Mezclar ambos criterios sin una regla explícita produce el peor resultado posible:
documentación a medias y código con identificadores en español que rompen la
convención del ecosistema.

## Alternativas evaluadas

| Alternativa | Ventajas | Desventajas |
|---|---|---|
| Todo en inglés | Coherencia total con el ecosistema | La documentación de decisiones —lo que el reto valora de forma explícita— se lee con más esfuerzo del necesario para su audiencia real |
| Todo en español, incluido el código | Coherencia con el enunciado | Rompe las convenciones de Go, de HTTP y de AWS; nombres como `apuestaRepositorio` o campos JSON como `montoApostado` dificultarían cualquier integración y desentonarían en una revisión técnica |
| **Documentación en español, código e identificadores en inglés** | Cada audiencia recibe el material en el registro que le corresponde | Requiere una regla explícita y aplicada de forma consistente |

## Decisión

Se adopta la **separación por audiencia**, con una frontera precisa:

**En español (registro formal y neutro):**

- `README.md` y los nueve documentos de `docs/adr/`.
- Descripciones, resúmenes y ejemplos de `api/openapi.yaml`.
- Los mensajes de error orientados al usuario final que viajan en el campo
  `error.message` de la respuesta (por ejemplo, `"Saldo insuficiente para realizar la
  apuesta."`).
- Las etiquetas del diagrama de arquitectura (`docs/diagrams/arquitectura.svg`).

**En inglés (convención de la industria, sin traducir):**

- Todos los identificadores de Go: paquetes, tipos, funciones, variables.
- Los comentarios del código fuente.
- Las rutas de la API (`/api/v1/betslip/place`), los nombres de campo JSON
  (`potentialReturns`, `combinedOdds`), las variables de entorno
  (`BETSLIP_MIN_STAKE_AMOUNT`), los códigos de error legibles por máquina
  (`INSUFFICIENT_FUNDS`), los nombres de esquema de OpenAPI y las claves de DynamoDB.
- Los mensajes de commit.

La regla que resuelve cualquier caso dudoso: **la prosa explicativa se escribe en
español; todo lo que un programa lee, compara o enruta se escribe en inglés.** Por eso
una respuesta de error combina ambos idiomas de forma deliberada:

```json
{
  "error": {
    "code": "SAME_EVENT_COMBO",
    "message": "La combinada no puede incluir dos selecciones del mismo evento.",
    "details": { "eventId": "784926067864698880" }
  },
  "requestId": "b28c0eee480b0be6"
}
```

`code` es un contrato estable sobre el que un cliente puede ramificar; `message` es
texto para una persona.

Se emplea un **registro formal y neutro**: tratamiento impersonal o de usted, sin
localismos ni voseo, de modo que el texto resulte natural para cualquier lector
hispanohablante.

Además, el material interno de proceso (`openspec/`, notas personales y el PDF del
enunciado) está excluido del repositorio mediante `.gitignore`: la entrega contiene el
producto y sus decisiones, no el andamiaje empleado para producirlo.

## Consecuencias

- **Positivas**: el evaluador lee las decisiones sin fricción idiomática, y el código
  mantiene las convenciones que cualquier persona que trabaje con Go, HTTP o AWS espera
  encontrar.
- **Negativas**: la frontera debe respetarse de forma consistente. Un identificador en
  español o un párrafo de documentación en inglés delatarían de inmediato una regla no
  aplicada.
- **Neutras**: el README abre con una nota breve que enuncia esta regla, para que el
  criterio quede explícito desde la primera línea en lugar de deducirse.
