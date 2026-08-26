# ADR-0002: División de la persistencia (catálogo en memoria, DynamoDB para el estado mutable)

- **Estado**: aceptada
- **Ámbito**: `internal/adapters/memory/`, `internal/adapters/dynamo/`, `internal/application/*/ports.go`
- **Decisiones de diseño relacionadas**: D2, D3, D4, D5

## Contexto

El servicio maneja dos clases de datos con perfiles de acceso opuestos:

1. **Datos de referencia**: 24 eventos con sus mercados y selecciones, entregados en
   `data.json` (1.7 MB). Se leen en cada consulta y **nunca se modifican en tiempo de
   ejecución**. No hay endpoint que los cree, actualice ni borre.
2. **Estado mutable**: usuarios, saldo, apuestas y registros de idempotencia. Se
   escriben bajo concurrencia y exigen atomicidad: el débito del saldo y el registro de
   la apuesta deben ocurrir juntos o no ocurrir.

Aplicar el mismo mecanismo de persistencia a ambos conjuntos obligaría a modelar en
una base de datos una estructura anidada de tres niveles (evento → mercado → selección)
con decenas de campos de presentación que ninguna regla de negocio consulta, y a
mantener migraciones para datos que jamás cambian.

## Alternativas evaluadas

| Alternativa | Ventajas | Desventajas |
|---|---|---|
| Todo en una base relacional | Una sola fuente de verdad; consultas uniformes | Modelado y migraciones extensos para metadatos de UI que no aportan valor evaluable; sobreingeniería |
| Todo relacional con columnas JSONB | Menos modelado relacional | Se pierde la ventaja de servir mercados ya ordenados; añade una dependencia de base de datos a datos que nunca mutan |
| Todo en DynamoDB, incluidos los eventos | Un único motor | Un elemento de DynamoDB está limitado a 400 KB; el dataset requeriría fragmentación artificial y varias lecturas por consulta, sin ganancia alguna |
| **Catálogo en memoria (`go:embed`) + DynamoDB solo para usuarios, saldo y apuestas** | Cada dato usa el mecanismo que corresponde a su forma de acceso; el esfuerzo de atomicidad se concentra donde es evaluado; mucho menos código | Dos paradigmas de persistencia coexisten: es necesario documentarlo como decisión, no como inconsistencia |

## Decisión

Se adopta la **división**: catálogo de eventos en memoria, cargado una única vez al
iniciar el proceso desde una copia de `data.json` embebida con `go:embed`; y DynamoDB
como única persistencia de usuarios, saldo, apuestas e idempotencia.

Los detalles que hacen que la división sea coherente y no arbitraria:

- **La frontera es la mutabilidad**, no la comodidad. Un dato de solo lectura cargado
  al arrancar no necesita una base de datos; un saldo que varía bajo concurrencia sí
  necesita condiciones atómicas.
- **Ambos lados implementan un puerto declarado por el consumidor**
  (`application/betslip.EventCatalog`, `application/betslip.BetRepository`,
  `application/auth.UserRepository`). No existe un paquete `ports/`: en Go, la interfaz
  la declara quien la consume, y la inversión de dependencias se garantiza por la
  dirección de los imports, no por el nombre de una carpeta.
- **Sustituir el catálogo en memoria por una base de datos es un archivo**: basta con
  otra implementación de `EventCatalog` y una línea distinta en `cmd/api/main.go`.
  Ningún caso de uso ni ningún handler cambia.
- **`go:embed` en lugar de leer un fichero en tiempo de ejecución**: elimina un modo de
  fallo (fichero ausente o ruta mal configurada) y un requisito de empaquetado en el
  ZIP de Lambda. El coste es 1.7 MB en el binario, procesados una sola vez al arrancar.
- **Filtrado en la carga (D5)**: de los 191 mercados del dataset solo se conservan los
  cuatro tipos por defecto (`ML0`, `OU200`, `QA158`, `ML235`), ya ordenados. El orden
  de presentación pasa a ser una invariante del dato en lugar de lógica de consulta,
  y el consumo de memoria residente se reduce de forma sustancial.

## Consecuencias

- **Positivas**: las consultas al catálogo son accesos a memoria (microsegundos, sin
  salto de red); no hay migraciones ni esquema que mantener para datos inmutables; el
  código de concurrencia queda concentrado en una sola función, legible de principio a
  fin (véase ADR-0003).
- **Negativas**: cambiar el catálogo obliga a reconstruir y redesplegar el binario. Es
  aceptable porque el dataset es material de referencia entregado con el reto, no
  contenido editable; en un sistema real esos datos llegarían de un proveedor externo y
  el adaptador correspondiente reemplazaría al de memoria.
- **Negativas**: cada instancia mantiene su propia copia del catálogo en memoria. A la
  escala de este servicio es irrelevante; a mayor escala, la alternativa natural es un
  adaptador respaldado por un servicio de catálogo compartido.
- **Neutras**: la coexistencia de dos paradigmas debe explicarse siempre que se presente
  la arquitectura. El README y el diagrama de `docs/diagrams/arquitectura.svg` lo hacen
  explícito con una etiqueta propia.
