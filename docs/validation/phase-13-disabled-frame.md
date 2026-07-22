# Phase 13 Disabled Image-Frame Baseline

`BenchmarkPhase13DisabledFrame` exercises the production `beginTerminalImageFrame` / `drawTerminalImages` / `finishTerminalImageFrame` dispatch seam with the default nil cache. It is context-free and performs no mux lookup, key collection, cache access, deadline registration, draw call, state mutation, or allocation.

## Reproducible command

```bash
GOMAXPROCS=1 go test -tags glfw ./internal/frontend/glfwgl \
  -run '^$' -bench '^BenchmarkPhase13DisabledFrame$' \
  -benchmem -benchtime=2s -count=10
```

Environment: Windows/amd64, Go 1.25.8, AMD Ryzen 9 7940HX with Radeon Graphics.

## First result and budget

- Median: **7.222 ns/op**
- Worst sample: **7.489 ns/op**
- Memory: **0 B/op, 0 allocs/op** in every sample
- Initial acceptance budget: median **<= 8.0 ns/op**, worst allocations **0 B/op / 0 allocs/op**

The paired `TestPhase13DisabledFrameIsAllocationAndMutationFree` and `TestPhase13DisabledFrameAddsNoRedrawOrIdleCadence` make the semantic requirements hard gates. `BenchmarkPhase13DisabledDraw` remains the independent row-grid gate; its candidate median was 44,022 ns/op versus the carried 43,100.5 ns/op baseline (+2.14%, within the 3% limit).
