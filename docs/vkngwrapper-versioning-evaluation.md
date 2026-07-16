# Evaluación del versionado de vkngwrapper

**Estado:** investigación archivada; la integración Vulkan está pausada indefinidamente  
**Fecha:** 2026-08-18  
**Alcance:** `github.com/vkngwrapper/core/v3` y `github.com/vkngwrapper/extensions/v3`

> **Nota de decisión:** CervTerm mantendrá OpenGL como único backend gráfico soportado. Este documento se conserva como referencia y no representa trabajo planificado. Véase [Decisión de backend gráfico](rendering-backend-decision.md).

## Resumen

| Característica | Resultado |
|---|---|
| Compatibilidad con drivers nuevos | Buena |
| Acceso inmediato a APIs nuevas | Malo |
| Seguridad de ABI | Buena |
| Automatización de actualizaciones | Baja |
| Trabajo para añadir una versión | Alto |
| Testabilidad | Buena |

> **Conclusión técnica archivada:** vkngwrapper optimiza para wrappers correctos, idiomáticos y testeables, pero sacrifica velocidad de actualización. Esta evaluación no cambia la decisión actual de no integrar Vulkan.

## Fundamento

### Compatibilidad con drivers nuevos: buena

Vulkan conserva compatibilidad hacia atrás. Una aplicación construida contra Vulkan 1.0–1.2 puede ejecutarse sobre drivers Vulkan 1.3 o 1.4 mientras no requiera capacidades nuevas. Para CervTerm, las operaciones necesarias para el primer backend —instancia, dispositivo, surface, swapchain, render pass, buffers, imágenes, pipelines, semáforos y fences— caben en Vulkan 1.0–1.2 más las extensiones de presentación.

### Acceso inmediato a APIs nuevas: malo

`vkngwrapper/core/v3` expone actualmente Vulkan core 1.0, 1.1 y 1.2. No existen todavía paquetes `core1_3` o `core1_4`. Una función nueva solo está disponible cuando:

1. ya existe como extensión soportada en `vkngwrapper/extensions/v3`; o
2. el proyecto implementa manualmente su wrapper core o de extensión.

Por tanto, un driver moderno no hace que las APIs Vulkan 1.3/1.4 aparezcan automáticamente en Go.

### Seguridad de ABI: buena

El proyecto usa wrappers C con firmas Vulkan concretas y carga los comandos mediante punteros obtenidos en el nivel correspondiente. Esto evita depender de un trampoline genérico que represente todos los argumentos como enteros. La contrapartida es que cada comando y estructura requiere implementación y pruebas específicas.

### Automatización de actualizaciones: baja

El core es un wrapper CGO escrito manualmente. Las herramientas `go generate` presentes se orientan principalmente a mocks y bibliotecas auxiliares; no regeneran todo el binding desde `vk.xml`. Los headers vendorizados corresponden a Vulkan 1.3.224, pero la API core pública termina en 1.2.

### Trabajo para añadir una versión: alto

Añadir Vulkan 1.3 o 1.4 exige, como mínimo:

- actualizar los headers vendorizados;
- declarar la nueva `APIVersion`;
- crear el paquete `core1_x`;
- añadir interfaces, tipos, features y estructuras;
- implementar drivers y wrappers C;
- actualizar la selección de drivers en `bootstrap.go`;
- promover las extensiones incorporadas al nuevo core;
- regenerar mocks;
- añadir pruebas y ejemplos.

### Testabilidad: buena

Las interfaces de drivers, loaders y extensiones son mockables. El proyecto publica mocks generados y mantiene pruebas detalladas del marshalling, de las cadenas `pNext` y de las operaciones por versión. Esto permite probar una parte importante de la integración sin depender de una GPU real.

## Implicaciones históricas de la integración evaluada

Estas condiciones se registraron como requisitos hipotéticos del backend investigado. No describen el roadmap actual y sólo serían aplicables si se reabre formalmente la decisión de Vulkan.

1. El primer backend debe fijar Vulkan 1.0 o 1.2 como contrato máximo requerido.
2. No debe depender de características exclusivas de Vulkan 1.3/1.4.
3. Todos los tipos de vkngwrapper deben permanecer encapsulados dentro del backend Vulkan.
4. El backend debe comprobar features y extensiones concretas, no inferirlas únicamente de la versión anunciada por el driver.
5. La dependencia debe fijarse a una versión o commit inmutable; no debe seguir `main` automáticamente.
6. Un fork solo debe mantener cambios mínimos, documentados y preferiblemente upstreamables.
7. La actualización de una versión Vulkan debe tratarse como trabajo explícito de mantenimiento, con validation layers y pruebas multiplataforma.

## Señales históricas para reevaluar vkngwrapper

Si la decisión de Vulkan se reabriera formalmente, se debería reevaluar vkngwrapper ante cualquiera de estas condiciones:

- CervTerm necesita una función exclusiva de Vulkan 1.3/1.4 no disponible como extensión soportada.
- El fork acumula cambios amplios o difíciles de sincronizar con upstream.
- Los headers vendorizados quedan incompatibles con los SDK o plataformas objetivo.
- Una alternativa generada desde `vk.xml` demuestra mejor seguridad de ABI y cobertura de pruebas.
- La falta de mantenimiento impide corregir errores críticos o soportar nuevas plataformas.

## Referencias

- [vkngwrapper/core](https://github.com/vkngwrapper/core)
- [Modelo de versiones y promoción](https://github.com/vkngwrapper/core/blob/227274a7d9aedcc6b16bffd977dd1ac4d34d795b/README.md#namespace-by-availability)
- [Versiones core declaradas](https://github.com/vkngwrapper/core/blob/227274a7d9aedcc6b16bffd977dd1ac4d34d795b/common/api_version.go#L13-L23)
- [Selección de drivers por versión](https://github.com/vkngwrapper/core/blob/227274a7d9aedcc6b16bffd977dd1ac4d34d795b/bootstrap.go#L27-L107)
- [vkngwrapper/extensions](https://github.com/vkngwrapper/extensions)
- [Extensiones y promociones soportadas](https://github.com/vkngwrapper/extensions/blob/6eb9201b5c6624b23c5624c5f74ca39cd0a9a4f5/README.md#L47-L56)
