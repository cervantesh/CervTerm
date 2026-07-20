# Decisión de backend gráfico: mantener OpenGL

**Estado:** aceptada  
**Fecha:** 2026-08-18  
**Decisión:** pausar indefinidamente el backend Vulkan y mantener OpenGL como único backend gráfico soportado.

## Contexto

CervTerm ya dispone de un frontend GLFW/OpenGL adecuado para su carga gráfica principal: composición 2D de celdas, atlas de glifos, color, cursor y actualización por filas dañadas. Se investigó un backend Vulkan y el posible uso o fork de `vkngwrapper`, pero la integración añadiría una segunda ruta completa de renderizado y una carga de mantenimiento desproporcionada respecto al beneficio demostrado.

La terminal no presenta actualmente una limitación medida que requiera Vulkan. Antes de la GPU, los posibles cuellos de botella incluyen shaping, rasterización de glifos, procesamiento del PTY, actualización del modelo de pantalla, construcción de buffers y sincronización accidental.

> **Nota personal:** continuar con Vulkan empezó a pesarme y a convertir una mejora opcional en una fuente de complejidad. Prefiero seguir afinando CervTerm sobre OpenGL. Me deja tranquila saber que ya abstrajimos OpenGL detrás de `gpu.Renderer` y dejamos stubs para Metal, Vulkan y WebGPU; también investigamos la viabilidad, los límites de las dependencias y los riesgos de sincronización y daño parcial. No estamos descartando Vulkan por desconocimiento; estamos tomando una decisión informada.

El trabajo de abstracción ya realizado se considera completo y útil por sí mismo. Mantiene las fronteras del renderer y una ruta documentada para el futuro, sin crear la obligación de implementar los backends alternativos.

## Decisión

1. OpenGL mediante GLFW continúa como backend principal y único backend soportado.
2. No se implementará el backend Vulkan ni se creará o mantendrá un fork de `vkngwrapper`.
3. La abstracción `gpu.Renderer` debe permanecer neutral para no acoplar el núcleo de la terminal a OpenGL.
4. El scaffold y la investigación Vulkan existentes se conservan únicamente como referencia histórica; no representan trabajo planificado ni una promesa de soporte futuro.
5. Las mejoras de rendimiento se dirigirán primero al backend OpenGL existente y se justificarán mediante mediciones.
6. La abstracción de OpenGL y los stubs de Metal, Vulkan y WebGPU se conservan como una frontera arquitectónica útil; no generan fases de implementación posteriores.

## Motivos

- OpenGL cubre el pipeline 2D requerido con batching, atlas persistente, buffers reutilizables y regiones de daño.
- Vulkan exigiría gestionar explícitamente instance, device, queues, swapchain, pipelines, memoria, command buffers, descriptors, semáforos, fences, resize y pérdida de dispositivo.
- El renderizado parcial requeriría además una imagen offscreen persistente; dibujar directamente sobre imágenes rotatorias del swapchain no preservaría las filas no dañadas.
- Dos backends multiplicarían pruebas, diagnóstico de drivers, shaders, sincronización y mantenimiento multiplataforma.
- No existe evidencia de que el overhead de OpenGL sea actualmente el factor que limita latencia, consumo de CPU o frecuencia de refresco.
- Reducir la carga mental y mantener el proyecto agradable de desarrollar es un criterio legítimo de sostenibilidad, además de los costes técnicos medibles.

## Consecuencias

### Positivas

- Menor complejidad arquitectónica y operativa.
- Una sola ruta gráfica que optimizar, probar y depurar.
- Menor riesgo de regresiones en resize, daño parcial, atlas y presentación.
- El esfuerzo puede concentrarse en estabilidad, rendimiento medido y experiencia diaria.

### Negativas

- No se obtiene el control explícito de memoria y sincronización de Vulkan.
- Algunas plataformas o drivers con OpenGL deficiente podrían requerir soluciones específicas.
- Si aparece una necesidad real de Vulkan, la investigación tendrá que revalidarse contra las versiones actuales de las dependencias.

## Criterios para reconsiderar Vulkan

La decisión sólo debe reabrirse si existe evidencia reproducible de al menos una de estas condiciones:

- OpenGL no cumple el objetivo de frame time, por ejemplo `p99 > 8.3 ms` a 120 Hz o `p99 > 16.7 ms` a 60 Hz.
- El trabajo de renderizado consume de forma sostenida más de 2–3 ms de CPU por frame después de optimizar batching, buffers y daño parcial.
- Existen fallos graves y reproducibles de drivers OpenGL en una plataforma soportada.
- Una función aprobada requiere capacidades que OpenGL no puede proporcionar razonablemente, como un pipeline HDR o compute específico.
- Un prototipo acotado demuestra una mejora material y medible que justifica el coste de una segunda implementación.

Reconsiderar Vulkan requerirá nuevas mediciones, un diseño aprobado, un plan de mantenimiento multiplataforma y una estrategia explícita para preservar OpenGL o sustituirlo. No debe retomarse únicamente por preferencia tecnológica.

## Referencias

- [Arquitectura de CervTerm](architecture.md)
- [Evaluación de versionado de vkngwrapper](vkngwrapper-versioning-evaluation.md)
- [Plan de daño por filas](row-damage-plan.md)
- [Plan de renderizado bajo demanda](on-demand-render-plan.md)
