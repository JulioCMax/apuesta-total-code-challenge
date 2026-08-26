# ADR-0008: Semilla de fase y grupo a partir de un dato verificado

- **Estado**: aceptada
- **Ámbito**: `internal/adapters/memory/seed/groups.go`, `internal/adapters/memory/loader.go`

## Contexto

El reto pide que cada evento exponga su **fase** y su **grupo**. El dataset entregado
(`data.json`) **no contiene ninguna señal de fase ni de grupo**: se verificó
directamente que los 24 eventos comparten un único `LeagueGroupId`
(`724526620140138497`) y que ningún campo del evento, del mercado o de la selección
identifica el grupo.

Adicionalmente, el conjunto es una **muestra parcial** del calendario de fase de grupos:
la mayoría de los grupos aparece representada por un solo partido. Al reconstruir el
grafo de enfrentamientos por coincidencia de nombres de equipo, solo dos grupos quedan
completamente visibles con cuatro equipos cada uno (Brasil, Marruecos, Haití y Escocia;
y Bélgica, Egipto, Irán y Nueva Zelanda). El resto de asignaciones **no puede derivarse
del propio dato**.

Se trata, por tanto, de un punto no especificado sobre el que hay que tomar una decisión
explícita y documentarla, en lugar de improvisar valores o de omitir el requisito.

## Alternativas evaluadas

| Alternativa | Ventajas | Desventajas |
|---|---|---|
| Omitir `phase` y `group` de la respuesta | Ningún dato inventado | Incumple un requisito explícito del reto |
| Devolver `group` siempre vacío | Honesto respecto del dataset | La respuesta se vuelve inútil para el consumidor y no demuestra ninguna decisión |
| Derivar los grupos por agrupamiento de equipos sobre el propio dataset | Totalmente derivado del dato | Solo funciona para 2 de los 12 grupos; el resto quedaría sin asignar, y una letra deducida de una muestra parcial sería una invención con apariencia de dato |
| Asignar letras arbitrarias | Rellena el campo | Dato inventado presentado como real: el peor resultado posible |
| **Transcribir el sorteo oficial verificado a un mapa de semilla, con un camino de reserva explícito** | Dato real y trazable; cubre los 24 eventos; el origen queda documentado y comprobado por una prueba | Requiere mantenimiento si cambian los nombres de equipo en el dataset |

## Decisión

Se transcribe a `internal/adapters/memory/seed/groups.go` el mapa
`GroupByTeam map[string]string`, con **39 nombres de participante escritos exactamente
como aparecen en `data.json`**, asignados a su letra de grupo (A–L).

**Procedencia del dato**, en tres fuentes concordantes:

1. El **sorteo final oficial de la Copa Mundial de la FIFA 2026, celebrado el 5 de
   diciembre de 2025**, como fuente principal.
2. Las **capturas del enunciado del reto**, empleadas como verificación cruzada.
3. Los **dos agrupamientos de equipos que sí son derivables del propio `data.json`**
   (grupos C y G), que coinciden con el sorteo oficial y actúan como comprobación
   independiente de que la transcripción corresponde a este dataset y no a otro
   calendario.

**Resolución en tiempo de ejecución** (`memory.ResolveGroup`): se leen los dos
participantes del evento y **ambos deben coincidir en la misma letra**. Exigir la
concordancia de los dos equipos, en lugar de confiar en uno solo, convierte cada evento
en una verificación cruzada de la propia semilla.

**Camino de reserva, obligatorio y explícito**: si un equipo no está en el mapa, si los
dos equipos discrepan o si la lista de participantes no se puede interpretar, el
resultado es `Group = ""` y `Phase = group_stage` (la única fase presente en el
dataset), acompañado de un aviso `slog.Warn{event_id, event_name}` durante el arranque.
El evento **se sigue listando y sirviendo por completo**: nunca se produce un pánico ni
falla una solicitud. En la respuesta JSON, un grupo vacío se omite por completo
(`omitempty`) en lugar de enviarse como cadena vacía.

## Consecuencias

- **Positivas**: los 24 eventos resuelven a una letra real entre la A y la L, con origen
  trazable. La afirmación no depende de la confianza: la prueba
  `TestGroupSeed_CoversEveryEvent` falla si cualquier evento deja de resolver, lo que
  detecta tanto una regresión en la semilla como un cambio en la grafía de un equipo en
  el dataset.
- **Positivas**: `TestGroupSeed_UnknownTeamFallsBackToEmptyGroup` demuestra que el camino
  de reserva funciona, de modo que un dataset ampliado con equipos no sembrados degrada
  de forma controlada en lugar de romper el arranque.
- **Negativas**: la semilla está acoplada a la grafía exacta de los nombres en
  `data.json` (por ejemplo, `"EE.UU."` y `"Países Bajos"`). Un cambio en el proveedor de
  datos exigiría actualizarla; la prueba de cobertura convierte ese cambio en un fallo
  visible en lugar de en una degradación silenciosa.
- **Negativas**: se introduce un dato mantenido a mano en un repositorio que, por lo
  demás, deriva todo del dataset. Está confinado a un único archivo, con su procedencia
  documentada en su propio comentario de cabecera y en este ADR.
- **Neutras**: `Phase` es un tipo enumerado. Este dataset solo contiene fase de grupos,
  pero añadir fases eliminatorias es extender la enumeración sin tocar a ningún
  consumidor.
