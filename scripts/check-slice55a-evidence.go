//go:build ignore

// check-slice55a-evidence validates the permanent Slice 5.5a source guards and
// the reproducible performance/platform evidence retained with the slice.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	baseCommit              = "c027fd1228af792203d3361a671a9b01017b2e23"
	baseHarness             = "35243d7f7672f28ddb39ac55cd404e5fa96ed990"
	candidateCommit         = "28326fa5bb05850a0d12c31afe0f334fa329b636"
	evidenceBaseCommit      = "320deef1ecb16db212cfee692128591359bebc70"
	evidenceBaseHarness     = "6efd6cd8e6df21a57886257552c31fd76be7c533"
	evidenceCandidateCommit = "295ef3f847c2be13f20fee250aeb06d39b67ccc7"
	baseManifestDigest      = "f06959254d1a16a107eac3def00804f087902c780db5f52a1f182e3a8ae56ea9"
	candidateManifestDigest = "73de78a927b3e29210306c641ed343093e6554655a528b0ca0f01aaf17ca0d6e"
	benchmarkRecordsDigest  = "7f50b1b2fef1e11e17d9107362bcd743a8e55c19d1aa7b755e4188e191f1d7e1"
	platformRecordsDigest   = "d5606957fd718b30a19c289ebcee913a71ad17ddf56beb482aba500367377c39"
	validationDocDigest     = "4375abd7229e77be55ba6bf2b10e9c7256788b7840bc414bf50ec3d54d945b0f"
	baseRawDigest           = "2ed8d9d91f5292e4fca959c0a8c6ac4c518ac3142a0e109fb507e97d6056ee97"
	candidateRawDigest      = "b0178b0f58855064510f6b54c81f351037f58394c957bc402db39db3438ebb07"
	interleavedRawDigest    = "2dbe3b4cd7b1539d3560a114a74d7f2e698a0e17c9d13e1710afa574b124a5e3"
	evidenceDir             = "docs/validation/architecture-maturity-slice-5.5a"
	validationDoc           = evidenceDir + ".md"
	binaryEvidence          = evidenceDir + "/benchmark-binaries.txt"
	baseManifest            = evidenceDir + "/source-manifest-base.txt"
	candidateManifest       = evidenceDir + "/source-manifest-candidate.txt"
	baseRaw                 = evidenceDir + "/benchmarks-base.txt"
	candidateRaw            = evidenceDir + "/benchmarks-candidate.txt"
	interleavedRaw          = evidenceDir + "/benchmarks-interleaved.txt"
	summaryEvidence         = evidenceDir + "/benchmark-summary.txt"
	platformEvidence        = evidenceDir + "/platform-gates.txt"
)

type finding struct{ path, reason string }

type declarationPin struct {
	path, receiver, name, hash string
}

var declarationPins = []declarationPin{
	{path: "internal/fontglyph/fontindex.go", name: "faceInfo", hash: "ae5249cc0aa279f92e9ce8842a2eb64c7e8608ea5f91bcd0fc3e126f567b6e16"},
	{path: "internal/fontglyph/fontindex.go", name: "FontIndex", hash: "f6afd8e7634884bd4735e56db1b70492a6b793824e9772f93c62660d336eb094"},
	{path: "internal/fontglyph/fontindex.go", name: "FontIndexDiagnostics", hash: "9410302e7088a4366f0d5c1ce799869178978e6f7600ff760cca1e10d195fb1f"},
	{path: "internal/fontglyph/fontindex.go", name: "FontResolution", hash: "82609ef4d99923e17ffa8278257ece3a1fdabcd65e9421eef1725e1be0c4f72c"},
	{path: "internal/fontglyph/fontindex.go", name: "BuildFontIndex", hash: "9220b163fa16e28197c6dba22fe52e09104a2bb93e1678a8368cdc8e267c4fe0"},
	{path: "internal/fontglyph/fontindex.go", name: "wrapDiscoveryIndex", hash: "c3b19cb92308ea6c416e3d9048c908e753a60435cd83f572f10f050b67182c29"},
	{path: "internal/fontglyph/fontindex.go", receiver: "FontIndex", name: "Diagnostics", hash: "da166d4ec84a97f9f3d162e4cf64f3c4bc6dd116f6880ffe8d9ab51e0eaa9843"},
	{path: "internal/fontglyph/fontindex.go", name: "rootFontIndexDiagnostics", hash: "fb17bfbe874df87a454ae03873901bde2b0c61197369c3bae5978485dcc4004f"},
	{path: "internal/fontglyph/fontindex.go", receiver: "FontIndex", name: "Lookup", hash: "0ffb6310e09615423a6e0fdf98f88e18749ccf1a93b1ac8377e1f82bb5fdbad9"},
	{path: "internal/fontglyph/fontindex.go", name: "rootFaceInfo", hash: "721b211399dd65386371b7a2bbdc35c1daba760ffdc77aa0a6cbca824255e4b7"},
	{path: "internal/fontglyph/fontindex.go", name: "fontIndexFaces", hash: "10fb4c3a07d1ca10b1a51cd0e81b55b33bdd228fd0678e5654f736da82c5d4ab"},
	{path: "internal/fontglyph/fontindex.go", name: "loadSystemFontIndex", hash: "2d6d572fc591c01f567c165e7d9bb8cb168d8c648af076d3bd29cabbb3d61321"},
	{path: "internal/fontglyph/fontindex.go", name: "ResolveSystemFont", hash: "a56245517d9d2baca67625b7e9b3d60fb426a4f97d374fbfd933b4cd0b8c7b35"},
	{path: "internal/fontglyph/fontindex.go", name: "rootFontResolution", hash: "ad10423d7d5ccd49cc5eb5bcf5c7e9307671d9136c6cd193bbb4b67b82988e1d"},
	{path: "internal/fontglyph/discovery/index.go", receiver: "Index", name: "Lookup", hash: "736cadbbccb114fdbc5ca3cd7488d51e04f76fade4a62ada6279516ec82b86ba"},
	{path: "internal/fontglyph/discovery/index.go", name: "detachedFace", hash: "8a595ced630364c1451a056f246c7f4b778d4201a5a67fbb6b5e95c5380bd484"},
	{path: "internal/fontglyph/discovery/index.go", receiver: "Index", name: "Faces", hash: "7f32b2ca0d342f88bbcaed7e8b3c54f1d25a3d1da8f458eea8f775fa8ac3cb08"},
	{path: "internal/fontglyph/cache/cache.go", receiver: "Manager", name: "Acquire", hash: "0ed245d4857400bcac30e623eb2c5e3108230108c46dca1ef48e39cda3593e6b"},
	{path: "internal/fontglyph/cache/cache.go", receiver: "Manager", name: "publishSource", hash: "cceb89ab97f62cf233ceaa12d6d5ceb4ef92396a7e22cd3b6814697901097407"},
	{path: "internal/fontglyph/cache/cache.go", receiver: "Manager", name: "releaseSourceLocked", hash: "0eb9f4b4eb6e0e9365cbc19db7315a222bc2c0472d1b0e4ad54e437e1bc25291"},
	{path: "internal/fontglyph/cache/cache.go", receiver: "Manager", name: "detachFaceLocked", hash: "4997023ce3c96041f55e306d83dc7091fd341c069214acf7f90edae33c5d72ee"},
	{path: "internal/fontglyph/cache/cache.go", name: "closeEvictedOwners", hash: "5050ea4c61325a48d98ac27e2364ac4db67c77530104508a998fb0fc9f59e295"},
	{path: "internal/fontglyph/cache/cache.go", receiver: "Manager", name: "makeRoomLocked", hash: "0d228d2d2255639e6c054d8d654179e29a2d4bb2b94da98718a0035a59871da7"},
	{path: "internal/fontglyph/backend.go", receiver: "OpenTypeBackend", name: "Close", hash: "b6608b1627e0ea0f2a57c31b8977f4899977a28109ae2a597612d6c183ef6ebd"},
	{path: "internal/fontglyph/backend.go", name: "closeLoadedFace", hash: "5c67cfae6673e75e279dba18dae6d4638abc3ddd1be274b2d04175db44b49b31"},
	{path: "internal/fontglyph/backend.go", name: "closeRasterFace", hash: "dbf6c4ee179d2b3b4be6c796acacc304428ece95e14a02533ce19f48eeaa3afd"},
	{path: "internal/fontglyph/fontindex_test.go", name: "TestFontIndexCompatibilityFacadeKeepsConcreteLegacyMethodSet", hash: "ce4a341c563241eea3a0875e6317404164be6dd2eaba80008dabdb08d304f856"},
	{path: "internal/fontglyph/fontindex_test.go", name: "TestFontIndexCompatibilityFacadeIsNilSafeAndDetached", hash: "15de871f22a20d04ece8a9d8e9dec478e16edb5b2a25dcd722b13ab8ab972c14"},
	{path: "internal/fontglyph/discovery/index_test.go", name: "TestIndexLookupAndFacesReturnDetachedValues", hash: "922078bd2fd5b7b221cc8f4dabf4a16a0780f2166e74f9caa0187363c08724fb"},
	{path: "internal/fontglyph/discovery/index_test.go", name: "TestIndexDetachedResultsConcurrentMutationIsolation", hash: "403de4af1c85404e020e39116601bab1559a9c97339c0d3b124d2686b6e6ff3e"},
	{path: "internal/fontglyph/cache/cache_test.go", name: "TestFontParseCacheEqualLastUsedEvictsLexicalKey", hash: "d74932e8d246260d8620e65cb25fd055d15bcfb48030c29aae8599652471163a"},
	{path: "internal/fontglyph/cache/cache_test.go", name: "TestFontParseCacheEvictionCloserCanReenterManager", hash: "3a4d2f9d3f0de203b87fdf0c7e34a6bf134de26c5351b0f80cf824a9b370eb67"},
	{path: "internal/fontglyph/cache/cache_test.go", name: "TestFontParseCacheSlowEvictionCloserDoesNotHoldManagerLock", hash: "8829e4f8ab12620e5ee755e8f232cbde121851c8573828554173ebd8a00f8416"},
	{path: "internal/fontglyph/font_cache_test.go", name: "TestOpenTypeBackendCloseReleasesPinAndRejectsRaster", hash: "0dc4a5dc7fed540920490502c5573668907e7860877def2ef979bf515ed929d2"},
	{path: "internal/fontglyph/font_cache_test.go", name: "TestOpenTypeBackendCloseReversesAcquisitionAndClearsRetainedFaces", hash: "5d040f2254df1d91672dd6c27d8a565a35d98aef49febfc5aebbd2346a9d34e1"},
}

type benchmarkSpec struct {
	side, name, packagePath, goos, goarch, tags, gomaxprocs, warmup string
	sha256, benchPattern, benchmarks, compileCommand, runCommand    string
}

var expectedBenchmarkSpecs = []benchmarkSpec{
	{side: "base", name: "fontglyph", packagePath: "./internal/fontglyph", goos: "windows", goarch: "amd64", tags: "none", gomaxprocs: "1", warmup: "none", sha256: "49bb7c515503730dffb4594efe5b25d953bad68a9bc9e9c80d7410d3ca7d1c32", benchPattern: "^BenchmarkL402", benchmarks: "BenchmarkL402DiscoveryTopK,BenchmarkL402BuildIndexGoMono,BenchmarkL402CacheHitLease", compileCommand: `GOOS=windows+GOARCH=amd64+GOMAXPROCS=1+go+test+-c+-trimpath+-o+C:\dev\cervterm-55a-benchmark-artifacts\base\fontglyph.test.exe+./internal/fontglyph`, runCommand: `C:\dev\cervterm-55a-benchmark-artifacts\base\fontglyph.test.exe+-test.run=^$+-test.bench=^BenchmarkL402+-test.benchmem+-test.benchtime=1s+-test.count=1+-test.cpu=1`},
	{side: "base", name: "core", packagePath: "./internal/core", goos: "windows", goarch: "amd64", tags: "none", gomaxprocs: "1", warmup: "none", sha256: "215100d246e3184c0bec4b685a642c2c303e4bf6befe0d0c7a1298a5b06d9de6", benchPattern: "^BenchmarkPhase15TerminalStartupMemory$", benchmarks: "BenchmarkPhase15TerminalStartupMemory", compileCommand: `GOOS=windows+GOARCH=amd64+GOMAXPROCS=1+go+test+-c+-trimpath+-o+C:\dev\cervterm-55a-benchmark-artifacts\base\core.test.exe+./internal/core`, runCommand: `C:\dev\cervterm-55a-benchmark-artifacts\base\core.test.exe+-test.run=^$+-test.bench=^BenchmarkPhase15TerminalStartupMemory$+-test.benchmem+-test.benchtime=1s+-test.count=1+-test.cpu=1`},
	{side: "base", name: "render", packagePath: "./internal/render", goos: "windows", goarch: "amd64", tags: "none", gomaxprocs: "1", warmup: "none", sha256: "3535a8e405b1c4212dd5ee7f728d661a6e885eb35facd099922b703009a2c96e", benchPattern: "^BenchmarkPhase13TextOnlySnapshot$", benchmarks: "BenchmarkPhase13TextOnlySnapshot", compileCommand: `GOOS=windows+GOARCH=amd64+GOMAXPROCS=1+go+test+-c+-trimpath+-o+C:\dev\cervterm-55a-benchmark-artifacts\base\render.test.exe+./internal/render`, runCommand: `C:\dev\cervterm-55a-benchmark-artifacts\base\render.test.exe+-test.run=^$+-test.bench=^BenchmarkPhase13TextOnlySnapshot$+-test.benchmem+-test.benchtime=1s+-test.count=1+-test.cpu=1`},
	{side: "base", name: "glfwgl", packagePath: "./internal/frontend/glfwgl", goos: "windows", goarch: "amd64", tags: "glfw", gomaxprocs: "1", warmup: "none", sha256: "5303f0bc60b08c6fb17b9aecf40f8791d36fab89e4762a081116f4692883c306", benchPattern: "^BenchmarkPhase13DisabledDraw$", benchmarks: "BenchmarkPhase13DisabledDraw", compileCommand: `GOOS=windows+GOARCH=amd64+GOMAXPROCS=1+go+test+-c+-trimpath+-tags+glfw+-o+C:\dev\cervterm-55a-benchmark-artifacts\base\glfwgl.test.exe+./internal/frontend/glfwgl`, runCommand: `C:\dev\cervterm-55a-benchmark-artifacts\base\glfwgl.test.exe+-test.run=^$+-test.bench=^BenchmarkPhase13DisabledDraw$+-test.benchmem+-test.benchtime=1s+-test.count=1+-test.cpu=1`},
	{side: "candidate", name: "fontglyph", packagePath: "./internal/fontglyph", goos: "windows", goarch: "amd64", tags: "none", gomaxprocs: "1", warmup: "none", sha256: "23bc578d9784bc7a2bed88d5ef54fc586fa462c1fa7751f8ece7fabf412ae4fd", benchPattern: "^BenchmarkL402(BuildIndexGoMono|CacheHitLease)$", benchmarks: "BenchmarkL402BuildIndexGoMono,BenchmarkL402CacheHitLease", compileCommand: `GOOS=windows+GOARCH=amd64+GOMAXPROCS=1+go+test+-c+-trimpath+-o+C:\dev\cervterm-55a-benchmark-artifacts\candidate\fontglyph.test.exe+./internal/fontglyph`, runCommand: `C:\dev\cervterm-55a-benchmark-artifacts\candidate\fontglyph.test.exe+-test.run=^$+-test.bench=^BenchmarkL402(BuildIndexGoMono|CacheHitLease)$+-test.benchmem+-test.benchtime=1s+-test.count=1+-test.cpu=1`},
	{side: "candidate", name: "discovery", packagePath: "./internal/fontglyph/discovery", goos: "windows", goarch: "amd64", tags: "none", gomaxprocs: "1", warmup: "none", sha256: "1ccc60b2a9225f97b2d29381484faa75cbd58c948a1d8800fdad29340f7f788e", benchPattern: "^BenchmarkL402DiscoveryTopK$", benchmarks: "BenchmarkL402DiscoveryTopK", compileCommand: `GOOS=windows+GOARCH=amd64+GOMAXPROCS=1+go+test+-c+-trimpath+-o+C:\dev\cervterm-55a-benchmark-artifacts\candidate\discovery.test.exe+./internal/fontglyph/discovery`, runCommand: `C:\dev\cervterm-55a-benchmark-artifacts\candidate\discovery.test.exe+-test.run=^$+-test.bench=^BenchmarkL402DiscoveryTopK$+-test.benchmem+-test.benchtime=1s+-test.count=1+-test.cpu=1`},
	{side: "candidate", name: "core", packagePath: "./internal/core", goos: "windows", goarch: "amd64", tags: "none", gomaxprocs: "1", warmup: "none", sha256: "215100d246e3184c0bec4b685a642c2c303e4bf6befe0d0c7a1298a5b06d9de6", benchPattern: "^BenchmarkPhase15TerminalStartupMemory$", benchmarks: "BenchmarkPhase15TerminalStartupMemory", compileCommand: `GOOS=windows+GOARCH=amd64+GOMAXPROCS=1+go+test+-c+-trimpath+-o+C:\dev\cervterm-55a-benchmark-artifacts\candidate\core.test.exe+./internal/core`, runCommand: `C:\dev\cervterm-55a-benchmark-artifacts\candidate\core.test.exe+-test.run=^$+-test.bench=^BenchmarkPhase15TerminalStartupMemory$+-test.benchmem+-test.benchtime=1s+-test.count=1+-test.cpu=1`},
	{side: "candidate", name: "render", packagePath: "./internal/render", goos: "windows", goarch: "amd64", tags: "none", gomaxprocs: "1", warmup: "none", sha256: "3535a8e405b1c4212dd5ee7f728d661a6e885eb35facd099922b703009a2c96e", benchPattern: "^BenchmarkPhase13TextOnlySnapshot$", benchmarks: "BenchmarkPhase13TextOnlySnapshot", compileCommand: `GOOS=windows+GOARCH=amd64+GOMAXPROCS=1+go+test+-c+-trimpath+-o+C:\dev\cervterm-55a-benchmark-artifacts\candidate\render.test.exe+./internal/render`, runCommand: `C:\dev\cervterm-55a-benchmark-artifacts\candidate\render.test.exe+-test.run=^$+-test.bench=^BenchmarkPhase13TextOnlySnapshot$+-test.benchmem+-test.benchtime=1s+-test.count=1+-test.cpu=1`},
	{side: "candidate", name: "glfwgl", packagePath: "./internal/frontend/glfwgl", goos: "windows", goarch: "amd64", tags: "glfw", gomaxprocs: "1", warmup: "none", sha256: "4dd053ef3c393c2abcf530adfdde3eab9d86274f38b3d632510881e8c47e6647", benchPattern: "^BenchmarkPhase13DisabledDraw$", benchmarks: "BenchmarkPhase13DisabledDraw", compileCommand: `GOOS=windows+GOARCH=amd64+GOMAXPROCS=1+go+test+-c+-trimpath+-tags+glfw+-o+C:\dev\cervterm-55a-benchmark-artifacts\candidate\glfwgl.test.exe+./internal/frontend/glfwgl`, runCommand: `C:\dev\cervterm-55a-benchmark-artifacts\candidate\glfwgl.test.exe+-test.run=^$+-test.bench=^BenchmarkPhase13DisabledDraw$+-test.benchmem+-test.benchtime=1s+-test.count=1+-test.cpu=1`},
}

type platformSpec struct {
	name, mode, goos, goarch, cgo, tags, packagePath, gomaxprocs, warmup string
	run, count, benchtime, cpu, buildCommand, runCommand                 string
}

var expectedPlatformSpecs = []platformSpec{
	{name: "linux_fontglyph_focus", mode: "execution", goos: "linux", goarch: "amd64", cgo: "0", tags: "none", packagePath: "./internal/fontglyph", gomaxprocs: "1", warmup: "none", run: `^(TestFontIndexCompatibilityFacade.*|TestOpenTypeBackendClose(ReleasesPinAndRejectsRaster|ReversesAcquisitionAndClearsRetainedFaces)|TestL402(DiscoveryFailureContract|DiscoveryIdentityAndPlatformRootsContract|CachedFaceIdentityAndOutputParity))$`, count: "1", benchtime: "none", cpu: "none", buildCommand: `GOOS=linux+GOARCH=amd64+CGO_ENABLED=0+GOMAXPROCS=1+go+test+-c+-trimpath+-o+<artifact-root>/linux/fontglyph.test+./internal/fontglyph`, runCommand: `MSYS_NO_PATHCONV=1+wsl.exe+-d+Ubuntu-24.04+--+<artifact-root>/linux/fontglyph.test+-test.run=^(TestFontIndexCompatibilityFacade.*|TestOpenTypeBackendClose(ReleasesPinAndRejectsRaster|ReversesAcquisitionAndClearsRetainedFaces)|TestL402(DiscoveryFailureContract|DiscoveryIdentityAndPlatformRootsContract|CachedFaceIdentityAndOutputParity))$+-test.count=1`},
	{name: "linux_cache", mode: "execution", goos: "linux", goarch: "amd64", cgo: "0", tags: "none", packagePath: "./internal/fontglyph/cache", gomaxprocs: "1", warmup: "none", run: ".*", count: "1", benchtime: "none", cpu: "none", buildCommand: `GOOS=linux+GOARCH=amd64+CGO_ENABLED=0+GOMAXPROCS=1+go+test+-c+-trimpath+-o+<artifact-root>/linux/cache.test+./internal/fontglyph/cache`, runCommand: `MSYS_NO_PATHCONV=1+wsl.exe+-d+Ubuntu-24.04+--+<artifact-root>/linux/cache.test+-test.run=.*+-test.count=1`},
	{name: "linux_discovery", mode: "execution", goos: "linux", goarch: "amd64", cgo: "0", tags: "none", packagePath: "./internal/fontglyph/discovery", gomaxprocs: "1", warmup: "none", run: ".*", count: "1", benchtime: "none", cpu: "none", buildCommand: `GOOS=linux+GOARCH=amd64+CGO_ENABLED=0+GOMAXPROCS=1+go+test+-c+-trimpath+-o+<artifact-root>/linux/discovery.test+./internal/fontglyph/discovery`, runCommand: `MSYS_NO_PATHCONV=1+wsl.exe+-d+Ubuntu-24.04+--+<artifact-root>/linux/discovery.test+-test.run=.*+-test.count=1`},
	{name: "linux_face", mode: "execution", goos: "linux", goarch: "amd64", cgo: "0", tags: "none", packagePath: "./internal/fontglyph/internal/face", gomaxprocs: "1", warmup: "none", run: ".*", count: "1", benchtime: "none", cpu: "none", buildCommand: `GOOS=linux+GOARCH=amd64+CGO_ENABLED=0+GOMAXPROCS=1+go+test+-c+-trimpath+-o+<artifact-root>/linux/face.test+./internal/fontglyph/internal/face`, runCommand: `MSYS_NO_PATHCONV=1+wsl.exe+-d+Ubuntu-24.04+--+<artifact-root>/linux/face.test+-test.run=.*+-test.count=1`},
	{name: "windows_amd64", mode: "compile-only", goos: "windows", goarch: "amd64", cgo: "0", tags: "none", packagePath: "./...", gomaxprocs: "1", warmup: "none", run: "^$", count: "1", benchtime: "none", cpu: "none", buildCommand: `GOOS=windows+GOARCH=amd64+CGO_ENABLED=0+GOMAXPROCS=1+go+test+-run+^$+-count=1+-exec=true+./...`, runCommand: "none"},
	{name: "windows_arm64", mode: "compile-only", goos: "windows", goarch: "arm64", cgo: "0", tags: "none", packagePath: "./...", gomaxprocs: "1", warmup: "none", run: "^$", count: "1", benchtime: "none", cpu: "none", buildCommand: `GOOS=windows+GOARCH=arm64+CGO_ENABLED=0+GOMAXPROCS=1+go+test+-run+^$+-count=1+-exec=true+./...`, runCommand: "none"},
	{name: "darwin_amd64", mode: "compile-only", goos: "darwin", goarch: "amd64", cgo: "0", tags: "none", packagePath: "./...", gomaxprocs: "1", warmup: "none", run: "^$", count: "1", benchtime: "none", cpu: "none", buildCommand: `GOOS=darwin+GOARCH=amd64+CGO_ENABLED=0+GOMAXPROCS=1+go+test+-run+^$+-count=1+-exec=true+./...`, runCommand: "none"},
	{name: "darwin_arm64", mode: "compile-only", goos: "darwin", goarch: "arm64", cgo: "0", tags: "none", packagePath: "./...", gomaxprocs: "1", warmup: "none", run: "^$", count: "1", benchtime: "none", cpu: "none", buildCommand: `GOOS=darwin+GOARCH=arm64+CGO_ENABLED=0+GOMAXPROCS=1+go+test+-run+^$+-count=1+-exec=true+./...`, runCommand: "none"},
}

var benchmarkBinary = map[string]map[string]string{
	"base": {
		"BenchmarkL402DiscoveryTopK": "fontglyph", "BenchmarkL402BuildIndexGoMono": "fontglyph", "BenchmarkL402CacheHitLease": "fontglyph",
		"BenchmarkPhase15TerminalStartupMemory": "core", "BenchmarkPhase13TextOnlySnapshot": "render", "BenchmarkPhase13DisabledDraw": "glfwgl",
	},
	"candidate": {
		"BenchmarkL402DiscoveryTopK": "discovery", "BenchmarkL402BuildIndexGoMono": "fontglyph", "BenchmarkL402CacheHitLease": "fontglyph",
		"BenchmarkPhase15TerminalStartupMemory": "core", "BenchmarkPhase13TextOnlySnapshot": "render", "BenchmarkPhase13DisabledDraw": "glfwgl",
	},
}

type benchmarkValue struct {
	ns     float64
	bytes  int64
	allocs int64
}

type rawKey struct {
	side      string
	sample    int
	benchmark string
}

type rawHeader struct {
	side   string
	sample int
	order  string
}

type rawCapture struct {
	values        map[rawKey]benchmarkValue
	headers       map[string]map[int]string
	physicalOrder []rawHeader
}

type summaryValue struct {
	baseMedian, candidateMedian, delta float64
	baseMaxBytes, candidateMaxBytes    int64
	baseMaxAllocs, candidateMaxAllocs  int64
	result                             string
}

func main() {
	printHashes := flag.Bool("print-decl-hashes", false, "print current canonical declaration hashes")
	flag.Parse()
	if *printHashes {
		for _, pin := range declarationPins {
			_, hash, err := declarationTextAndHash(pin)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s %s.%s: %v\n", pin.path, pin.receiver, pin.name, err)
				os.Exit(1)
			}
			fmt.Printf("%s\t%s\t%s\t%s\n", pin.path, pin.receiver, pin.name, hash)
		}
		return
	}
	var findings []finding
	findings = append(findings, checkRootInventory()...)
	findings = append(findings, checkDeclarationPins()...)
	findings = append(findings, checkAdversarialSelfFixtures()...)
	findings = append(findings, checkBenchmarkEvidence()...)
	findings = append(findings, checkPlatformEvidence()...)
	if len(findings) != 0 {
		for _, item := range findings {
			fmt.Printf("FAIL %-72s %s\n", item.path, item.reason)
		}
		os.Exit(1)
	}
	fmt.Println("Slice 5.5a evidence guard ok")
}

func checkRootInventory() []finding {
	const path = "internal/fontglyph/fontindex.go"
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return []finding{{path, err.Error()}}
	}
	actual := make(map[string]int)
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			key := "func:" + declaration.Name.Name
			if receiver := receiverName(declaration); receiver != "" {
				key = "method:" + receiver + "." + declaration.Name.Name
			}
			actual[key]++
		case *ast.GenDecl:
			for _, raw := range declaration.Specs {
				switch spec := raw.(type) {
				case *ast.TypeSpec:
					actual["type:"+spec.Name.Name]++
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						actual["value:"+name.Name]++
					}
				}
			}
		}
	}
	expected := []string{
		"type:faceInfo", "type:FontIndex", "type:FontIndexDiagnostics", "type:FontResolution",
		"value:rootSystemIndexOnce", "value:rootSystemIndex",
		"func:BuildFontIndex", "func:wrapDiscoveryIndex", "method:FontIndex.Diagnostics", "func:rootFontIndexDiagnostics", "method:FontIndex.Lookup",
		"func:rootFaceInfo", "func:fontIndexFaces", "func:loadSystemFontIndex", "func:ResolveSystemFont", "func:rootFontResolution", "func:systemFontDirs", "func:isEmbeddedFamily",
	}
	var findings []finding
	if len(actual) != len(expected) {
		findings = append(findings, finding{path, fmt.Sprintf("concrete root declaration/method inventory=%v want exact %v", actual, expected)})
	}
	for _, key := range expected {
		if actual[key] != 1 {
			findings = append(findings, finding{path, fmt.Sprintf("concrete root inventory %s count=%d want 1", key, actual[key])})
		}
	}
	if actual["method:FontIndex.Faces"] != 0 || actual["method:faceInfo.Path"] != 0 {
		findings = append(findings, finding{path, "compatibility facade exposed discovery-owned mutable/method authority"})
	}
	return findings
}

func checkDeclarationPins() []finding {
	var findings []finding
	for _, pin := range declarationPins {
		_, got, err := declarationTextAndHash(pin)
		if err != nil {
			findings = append(findings, finding{pin.path, err.Error()})
			continue
		}
		if pin.hash == "PRINT" || got != pin.hash {
			findings = append(findings, finding{pin.path, fmt.Sprintf("canonical declaration drift %s.%s hash=%s want=%s", pin.receiver, pin.name, got, pin.hash)})
		}
	}
	return findings
}

func declarationTextAndHash(pin declarationPin) (string, string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, pin.path, nil, 0)
	if err != nil {
		return "", "", err
	}
	var matches []ast.Node
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			if declaration.Name.Name == pin.name && receiverName(declaration) == pin.receiver {
				matches = append(matches, declaration)
			}
		case *ast.GenDecl:
			if pin.receiver != "" {
				continue
			}
			for _, raw := range declaration.Specs {
				if spec, ok := raw.(*ast.TypeSpec); ok && spec.Name.Name == pin.name {
					matches = append(matches, spec)
				}
			}
		}
	}
	if len(matches) != 1 {
		return "", "", fmt.Errorf("declaration %s.%s count=%d want 1", pin.receiver, pin.name, len(matches))
	}
	var rendered bytes.Buffer
	if err := format.Node(&rendered, fset, matches[0]); err != nil {
		return "", "", err
	}
	text := strings.ReplaceAll(string(rendered.Bytes()), "\r\n", "\n")
	hash := sha256.Sum256([]byte(text))
	return text, fmt.Sprintf("%x", hash[:]), nil
}

func receiverName(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) != 1 {
		return ""
	}
	expression := function.Recv.List[0].Type
	if star, ok := expression.(*ast.StarExpr); ok {
		expression = star.X
	}
	if index, ok := expression.(*ast.IndexExpr); ok {
		expression = index.X
	}
	if index, ok := expression.(*ast.IndexListExpr); ok {
		expression = index.X
	}
	if name, ok := expression.(*ast.Ident); ok {
		return name.Name
	}
	return ""
}

func checkAdversarialSelfFixtures() []finding {
	fixtures := []struct {
		label, path, receiver, name, old, replacement string
	}{
		{"equal-lastUsed lexical reversal", "internal/fontglyph/cache/cache.go", "Manager", "makeRoomLocked", "key < oldestKey", "key > oldestKey"},
		{"eviction callback moved under lock", "internal/fontglyph/cache/cache.go", "Manager", "Acquire", "blob.refs--\n\t\t\tm.mu.Unlock()\n\t\t\tcloseEvictedOwners(evicted)", "blob.refs--\n\t\t\tcloseEvictedOwners(evicted)\n\t\t\tm.mu.Unlock()"},
		{"source reference retained", "internal/fontglyph/cache/cache.go", "Manager", "detachFaceLocked", "entry.source = nil", "_ = entry.source"},
		{"source bytes retained", "internal/fontglyph/cache/cache.go", "Manager", "releaseSourceLocked", "blob.data = nil", "_ = blob.data"},
		{"reverse close changed to forward", "internal/fontglyph/backend.go", "OpenTypeBackend", "Close", "for i := len(b.faces) - 1; i >= 0; i--", "for i := 0; i < len(b.faces); i++"},
		{"loaded face not cleared before lease callback", "internal/fontglyph/backend.go", "", "closeLoadedFace", "*loaded = loadedFace{}\n\tlease.Close()", "lease.Close()\n\t*loaded = loadedFace{}"},
		{"discovery slice aliases immutable storage", "internal/fontglyph/discovery/index.go", "Index", "Faces", "return append([]Face(nil), index.families[normalizeFamily(family)]...)", "return index.families[normalizeFamily(family)]"},
		{"discovery lookup returns indexed address", "internal/fontglyph/discovery/index.go", "Index", "Lookup", "face := faces[i]", "face := &faces[i]"},
	}
	var findings []finding
	for _, fixture := range fixtures {
		text, original, err := declarationTextAndHash(declarationPin{path: fixture.path, receiver: fixture.receiver, name: fixture.name})
		if err != nil {
			findings = append(findings, finding{"scripts/check-slice55a-evidence.go", fixture.label + ": " + err.Error()})
			continue
		}
		if strings.Count(text, fixture.old) != 1 {
			findings = append(findings, finding{"scripts/check-slice55a-evidence.go", fixture.label + " fixture did not match exactly once"})
			continue
		}
		mutated := strings.Replace(text, fixture.old, fixture.replacement, 1)
		hash := sha256.Sum256([]byte(mutated))
		if fmt.Sprintf("%x", hash[:]) == original {
			findings = append(findings, finding{"scripts/check-slice55a-evidence.go", fixture.label + " mutation preserved declaration hash"})
		}
	}
	for _, fixture := range []struct {
		name, source string
		fields       int
	}{
		{"FontIndex", "package fontglyph\nimport \"cervterm/internal/fontglyph/discovery\"\ntype FontIndex = discovery.Index\n", 1},
		{"FontIndexDiagnostics", "package fontglyph\nimport \"cervterm/internal/fontglyph/discovery\"\ntype FontIndexDiagnostics = discovery.Diagnostics\n", 11},
		{"FontResolution", "package fontglyph\nimport \"cervterm/internal/fontglyph/discovery\"\ntype FontResolution = discovery.Resolution\n", 11},
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", fixture.source, 0)
		if err != nil || concreteNamedStruct(file, fixture.name, fixture.fields) {
			findings = append(findings, finding{"scripts/check-slice55a-evidence.go", "concrete-root adversarial alias fixture escaped for " + fixture.name})
		}
	}
	wantHeaders := expectedRawHeaders(interleavedRaw)
	headerMutations := [][]rawHeader{append([]rawHeader(nil), wantHeaders[:len(wantHeaders)-1]...), append(append([]rawHeader(nil), wantHeaders...), wantHeaders[0])}
	reordered := append([]rawHeader(nil), wantHeaders...)
	reordered[0], reordered[1] = reordered[1], reordered[0]
	headerMutations = append(headerMutations, reordered)
	for _, mutated := range headerMutations {
		if len(validateRawHeaderSequence("fixture", mutated, wantHeaders)) == 0 {
			findings = append(findings, finding{"scripts/check-slice55a-evidence.go", "raw missing/duplicate/reordered sample fixture escaped"})
		}
	}
	records, parseFindings := parseBinaryEvidence(binaryEvidence)
	if len(parseFindings) == 0 {
		for key, value := range map[string]string{"base_prepare_command": strings.Replace(records.metadata["base_prepare_command"], evidenceBaseCommit, evidenceCandidateCommit, 1), "candidate_prepare_command": strings.Replace(records.metadata["candidate_prepare_command"], evidenceCandidateCommit, evidenceBaseCommit, 1)} {
			mutated := cloneStrings(records.metadata)
			mutated[key] = value
			if len(benchmarkMetadataFindings("fixture", mutated)) == 0 {
				findings = append(findings, finding{"scripts/check-slice55a-evidence.go", "benchmark metadata mutation escaped: " + key})
			}
		}
		digestMutation := cloneBinaryEvidenceRecords(records)
		digestMutation.binaries["candidate"]["discovery"] = strings.Repeat("a", 64)
		if len(validateBinaryEvidenceRecords("fixture", digestMutation)) == 0 {
			findings = append(findings, finding{"scripts/check-slice55a-evidence.go", "arbitrary valid coordinated binary digest fixture escaped"})
		}
		targetMutation := cloneBinaryEvidenceRecords(records)
		key := benchmarkRecordKey("candidate", "discovery")
		targetMutation.compile[key]["package"] = "./internal/fontglyph/cache"
		targetMutation.compile[key]["command"] = strings.Replace(targetMutation.compile[key]["command"], "./internal/fontglyph/discovery", "./internal/fontglyph/cache", 1)
		targetMutation.run[key]["package"] = "./internal/fontglyph/cache"
		if len(validateBinaryEvidenceRecords("fixture", targetMutation)) == 0 {
			findings = append(findings, finding{"scripts/check-slice55a-evidence.go", "discovery/cache target-swap fixture escaped"})
		}
	}
	manifestFixture := map[string]string{"internal/fontglyph/fontindex.go": strings.Repeat("a", 64), "internal/fontglyph/discovery_cache_characterization_test.go": strings.Repeat("b", 64)}
	for _, sourcePath := range []string{"internal/fontglyph/fontindex.go", "internal/fontglyph/discovery_cache_characterization_test.go"} {
		mutated := cloneStrings(manifestFixture)
		mutated[sourcePath] = strings.Repeat("c", 64)
		if len(compareManifestEntries("fixture", mutated, manifestFixture)) == 0 {
			findings = append(findings, finding{"scripts/check-slice55a-evidence.go", "candidate production/benchmark manifest mutation escaped: " + sourcePath})
		}
	}
	return findings
}

func concreteNamedStruct(file *ast.File, name string, fields int) bool {
	for _, declaration := range file.Decls {
		generic, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, raw := range generic.Specs {
			spec, ok := raw.(*ast.TypeSpec)
			if !ok || spec.Name.Name != name || spec.Assign.IsValid() {
				continue
			}
			structure, ok := spec.Type.(*ast.StructType)
			return ok && structureFieldCount(structure) == fields
		}
	}
	return false
}

func structureFieldCount(structure *ast.StructType) int {
	count := 0
	for _, field := range structure.Fields.List {
		count += max(1, len(field.Names))
	}
	return count
}

func checkBenchmarkEvidence() []finding {
	records, findings := parseBinaryEvidence(binaryEvidence)
	findings = append(findings, validateArtifactDigest(binaryEvidence, benchmarkRecordsDigest)...)
	findings = append(findings, validateArtifactDigest(validationDoc, validationDocDigest)...)
	findings = append(findings, validateArtifactDigest(baseRaw, baseRawDigest)...)
	findings = append(findings, validateArtifactDigest(candidateRaw, candidateRawDigest)...)
	findings = append(findings, validateArtifactDigest(interleavedRaw, interleavedRawDigest)...)
	metadata := records.metadata
	findings = append(findings, benchmarkMetadataFindings(binaryEvidence, metadata)...)
	findings = append(findings, validateBinaryEvidenceRecords(binaryEvidence, records)...)
	if metadata["base_production_commit"] != evidenceBaseCommit || metadata["base_harness_commit"] != evidenceBaseHarness || metadata["base_overlay_path"] != "internal/fontglyph/discovery_cache_characterization_test.go" {
		findings = append(findings, finding{binaryEvidence, "immutable base/overlay identity drift"})
	}
	if metadata["candidate_head"] != evidenceCandidateCommit {
		findings = append(findings, finding{binaryEvidence, "immutable candidate W identity drift"})
	}
	if metadata["base_manifest_sha256"] != baseManifestDigest || metadata["candidate_manifest_sha256"] != candidateManifestDigest {
		findings = append(findings, finding{binaryEvidence, "pinned source-manifest digest drift"})
	}
	findings = append(findings, validateManifest(baseManifest, metadata["base_manifest_sha256"], "base", metadata)...)
	findings = append(findings, validateManifest(candidateManifest, metadata["candidate_manifest_sha256"], "candidate", metadata)...)
	baseCapture, baseFindings := parseRawEvidence(baseRaw, metadata)
	candidateCapture, candidateFindings := parseRawEvidence(candidateRaw, metadata)
	interleavedCapture, interleavedFindings := parseRawEvidence(interleavedRaw, metadata)
	findings = append(findings, baseFindings...)
	findings = append(findings, candidateFindings...)
	findings = append(findings, interleavedFindings...)
	if len(baseFindings)+len(candidateFindings)+len(interleavedFindings) == 0 {
		findings = append(findings, compareRawCaptures(baseCapture, candidateCapture, interleavedCapture)...)
		findings = append(findings, validateBenchmarkSummary(interleavedCapture)...)
	}
	return findings
}

func expectedBenchmarkMetadata() map[string]string {
	return map[string]string{
		"schema": "2", "base_production_commit": evidenceBaseCommit, "base_harness_commit": evidenceBaseHarness,
		"base_overlay_path":    "internal/fontglyph/discovery_cache_characterization_test.go",
		"base_manifest_sha256": baseManifestDigest, "candidate_head": evidenceCandidateCommit,
		"candidate_manifest_sha256": candidateManifestDigest,
		"normalization":             "path-slash+content-CRLF-or-CR-to-LF+sorted-SHA256-lines",
		"base_prepare_command":      "git+worktree+add+--detach+<base-worktree>+" + evidenceBaseCommit + ";git+show+" + evidenceBaseHarness + ":internal/fontglyph/discovery_cache_characterization_test.go+>+<base-worktree>/internal/fontglyph/discovery_cache_characterization_test.go",
		"candidate_prepare_command": "git+worktree+add+--detach+<candidate-worktree>+" + evidenceCandidateCommit,
	}
}

func benchmarkMetadataFindings(path string, metadata map[string]string) []finding {
	return compareRecordFields(path, "metadata", metadata, expectedBenchmarkMetadata())
}

func cloneStrings(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

type binaryEvidenceRecords struct {
	metadata     map[string]string
	binaries     map[string]map[string]string
	compile, run map[string]map[string]string
}

func newBinaryEvidenceRecords() binaryEvidenceRecords {
	return binaryEvidenceRecords{metadata: make(map[string]string), binaries: map[string]map[string]string{"base": {}, "candidate": {}}, compile: make(map[string]map[string]string), run: make(map[string]map[string]string)}
}

func cloneBinaryEvidenceRecords(source binaryEvidenceRecords) binaryEvidenceRecords {
	clone := newBinaryEvidenceRecords()
	clone.metadata = cloneStrings(source.metadata)
	for side, values := range source.binaries {
		clone.binaries[side] = cloneStrings(values)
	}
	for key, values := range source.compile {
		clone.compile[key] = cloneStrings(values)
	}
	for key, values := range source.run {
		clone.run[key] = cloneStrings(values)
	}
	return clone
}

func benchmarkRecordKey(side, name string) string { return side + "/" + name }

func expectedCompileFields(spec benchmarkSpec) map[string]string {
	return map[string]string{"side": spec.side, "name": spec.name, "package": spec.packagePath, "goos": spec.goos, "goarch": spec.goarch, "tags": spec.tags, "gomaxprocs": spec.gomaxprocs, "warmup": spec.warmup, "command": spec.compileCommand}
}

func expectedRunFields(spec benchmarkSpec) map[string]string {
	return map[string]string{"side": spec.side, "name": spec.name, "package": spec.packagePath, "goos": spec.goos, "goarch": spec.goarch, "tags": spec.tags, "gomaxprocs": spec.gomaxprocs, "warmup": spec.warmup, "run": "^$", "bench": spec.benchPattern, "benchmarks": spec.benchmarks, "benchmem": "true", "benchtime": "1s", "count": "1", "cpu": "1", "command": spec.runCommand}
}

func benchmarkSpecFor(side, name string) (benchmarkSpec, bool) {
	for _, spec := range expectedBenchmarkSpecs {
		if spec.side == side && spec.name == name {
			return spec, true
		}
	}
	return benchmarkSpec{}, false
}

func benchmarkSpecsForSide(side string) []benchmarkSpec {
	var result []benchmarkSpec
	for _, spec := range expectedBenchmarkSpecs {
		if spec.side == side {
			result = append(result, spec)
		}
	}
	return result
}

func validateBinaryEvidenceRecords(path string, records binaryEvidenceRecords) []finding {
	var findings []finding
	expectedKeys := make(map[string]bool)
	for _, spec := range expectedBenchmarkSpecs {
		key := benchmarkRecordKey(spec.side, spec.name)
		expectedKeys[key] = true
		if records.binaries[spec.side][spec.name] != spec.sha256 {
			findings = append(findings, finding{path, "binary digest drift " + key})
		}
		findings = append(findings, compareRecordFields(path, "compile "+key, records.compile[key], expectedCompileFields(spec))...)
		findings = append(findings, compareRecordFields(path, "run "+key, records.run[key], expectedRunFields(spec))...)
	}
	for side, values := range records.binaries {
		for name := range values {
			if !expectedKeys[benchmarkRecordKey(side, name)] {
				findings = append(findings, finding{path, "unexpected binary record " + benchmarkRecordKey(side, name)})
			}
		}
	}
	for key := range records.compile {
		if !expectedKeys[key] {
			findings = append(findings, finding{path, "unexpected compile record " + key})
		}
	}
	for key := range records.run {
		if !expectedKeys[key] {
			findings = append(findings, finding{path, "unexpected run record " + key})
		}
	}
	return findings
}

func compareRecordFields(path, label string, actual, expected map[string]string) []finding {
	var findings []finding
	if len(actual) != len(expected) {
		findings = append(findings, finding{path, fmt.Sprintf("%s field count=%d want=%d", label, len(actual), len(expected))})
	}
	for key, want := range expected {
		if actual[key] != want {
			findings = append(findings, finding{path, fmt.Sprintf("%s field %s=%q want exact %q", label, key, actual[key], want)})
		}
	}
	for key := range actual {
		if _, ok := expected[key]; !ok {
			findings = append(findings, finding{path, label + " unexpected field " + key})
		}
	}
	return findings
}

func parseBinaryEvidence(path string) (binaryEvidenceRecords, []finding) {
	records := newBinaryEvidenceRecords()
	file, err := os.Open(path)
	if err != nil {
		return records, []finding{{path, err.Error()}}
	}
	defer file.Close()
	var findings []finding
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "binary "):
			fields := keyFields(strings.TrimPrefix(line, "binary "))
			side, name, hash := fields["side"], fields["name"], fields["sha256"]
			if len(fields) != 3 || records.binaries[side] == nil || name == "" || !validSHA256(hash) || records.binaries[side][name] != "" {
				findings = append(findings, finding{path, "malformed/duplicate binary row: " + line})
				continue
			}
			records.binaries[side][name] = hash
		case strings.HasPrefix(line, "compile "), strings.HasPrefix(line, "run "):
			kind := "compile"
			if strings.HasPrefix(line, "run ") {
				kind = "run"
			}
			fields := keyFields(strings.TrimPrefix(line, kind+" "))
			key := benchmarkRecordKey(fields["side"], fields["name"])
			target := records.compile
			if kind == "run" {
				target = records.run
			}
			if fields["side"] == "" || fields["name"] == "" || target[key] != nil {
				findings = append(findings, finding{path, "malformed/duplicate " + kind + " row: " + line})
				continue
			}
			target[key] = fields
		default:
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 || records.metadata[parts[0]] != "" {
				findings = append(findings, finding{path, "malformed/duplicate metadata: " + line})
				continue
			}
			records.metadata[parts[0]] = parts[1]
		}
	}
	if err := scanner.Err(); err != nil {
		findings = append(findings, finding{path, err.Error()})
	}
	return records, findings
}

func validateArtifactDigest(path, expected string) []finding {
	data, err := os.ReadFile(path)
	if err != nil {
		return []finding{{path, err.Error()}}
	}
	digest := sha256.Sum256(data)
	if fmt.Sprintf("%x", digest[:]) != expected {
		return []finding{{path, "artifact SHA-256 does not match executable guard pin"}}
	}
	return nil
}

func validateManifest(path, expectedHash, side string, metadata map[string]string) []finding {
	data, err := os.ReadFile(path)
	if err != nil {
		return []finding{{path, err.Error()}}
	}
	actualHash := sha256.Sum256(data)
	actualDigest := fmt.Sprintf("%x", actualHash[:])
	pinnedDigest := baseManifestDigest
	if side == "candidate" {
		pinnedDigest = candidateManifestDigest
	}
	if actualDigest != expectedHash || actualDigest != pinnedDigest {
		return []finding{{path, "raw manifest SHA-256 does not match pinned source identity"}}
	}
	entries, findings := parseManifest(path, data)
	if len(findings) != 0 {
		return findings
	}
	expected, available, immutableFindings := immutableManifestEntries(side, metadata)
	findings = append(findings, immutableFindings...)
	if available {
		findings = append(findings, compareManifestEntries(path, entries, expected)...)
	}
	return findings
}

func immutableManifestEntries(side string, metadata map[string]string) (map[string]string, bool, []finding) {
	commit := candidateCommit
	if side == "base" {
		commit = baseCommit
		if !gitObjectExists(baseCommit) || !gitObjectExists(baseHarness) {
			return nil, false, nil
		}
	} else if !gitObjectExists(candidateCommit) {
		return nil, false, nil
	}
	entries, err := manifestEntriesAtCommit(commit)
	if err != nil {
		return nil, true, []finding{{side + " manifest", err.Error()}}
	}
	if side == "base" {
		overlay := metadata["base_overlay_path"]
		content, showErr := gitOutput("show", baseHarness+":"+overlay)
		if showErr != nil {
			return nil, true, []finding{{baseManifest, "cannot read immutable benchmark overlay"}}
		}
		entries[overlay] = normalizedContentSHA(content)
	}
	return entries, true, nil
}

func manifestEntriesAtCommit(commit string) (map[string]string, error) {
	listing, err := gitOutput("ls-tree", "-r", "--name-only", commit)
	if err != nil {
		return nil, err
	}
	entries := make(map[string]string)
	for _, rawPath := range strings.Split(strings.TrimSpace(string(listing)), "\n") {
		entryPath := filepath.ToSlash(strings.TrimSpace(rawPath))
		if !sourceManifestPath(entryPath) {
			continue
		}
		content, showErr := gitOutput("show", commit+":"+entryPath)
		if showErr != nil {
			return nil, showErr
		}
		entries[entryPath] = normalizedContentSHA(content)
	}
	return entries, nil
}

func sourceManifestPath(path string) bool {
	return strings.HasSuffix(path, ".go") || path == "go.mod" || path == "go.sum"
}

func compareManifestEntries(path string, actual, expected map[string]string) []finding {
	var findings []finding
	if len(actual) != len(expected) {
		findings = append(findings, finding{path, fmt.Sprintf("source path count=%d want=%d", len(actual), len(expected))})
	}
	for entryPath, want := range expected {
		if actual[entryPath] != want {
			findings = append(findings, finding{path, "source snapshot drift: " + entryPath})
		}
	}
	for entryPath := range actual {
		if expected[entryPath] == "" {
			findings = append(findings, finding{path, "unexpected source snapshot path: " + entryPath})
		}
	}
	return findings
}

func parseManifest(path string, data []byte) (map[string]string, []finding) {
	entries := make(map[string]string)
	var previous string
	var findings []finding
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 || !validSHA256(parts[0]) || parts[1] == "" || filepath.ToSlash(parts[1]) != parts[1] || entries[parts[1]] != "" || previous >= parts[1] {
			findings = append(findings, finding{path, "malformed, duplicate, or unsorted manifest line: " + line})
			continue
		}
		entries[parts[1]] = parts[0]
		previous = parts[1]
	}
	if err := scanner.Err(); err != nil {
		findings = append(findings, finding{path, err.Error()})
	}
	if len(entries) == 0 {
		findings = append(findings, finding{path, "empty source manifest"})
	}
	return entries, findings
}

func normalizedContentSHA(data []byte) string {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	data = bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash[:])
}

func parseRawEvidence(path string, metadata map[string]string) (rawCapture, []finding) {
	capture := rawCapture{values: make(map[rawKey]benchmarkValue), headers: map[string]map[int]string{"base": {}, "candidate": {}}}
	file, err := os.Open(path)
	if err != nil {
		return capture, []finding{{path, err.Error()}}
	}
	defer file.Close()
	var findings []finding
	var side, order, binary string
	var sample int
	var currentSpec benchmarkSpec
	identityCounts := make(map[string]int)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "sample=") {
			fields := keyFields(line)
			candidateSide, candidateOrder := fields["side"], fields["order"]
			candidateSample, sampleErr := strconv.Atoi(fields["sample"])
			side, order, binary, sample, currentSpec = "", "", "", 0, benchmarkSpec{}
			if len(fields) != 4 || sampleErr != nil || (candidateSide != "base" && candidateSide != "candidate") || candidateSample < 1 || candidateSample > 10 || (candidateOrder != "AB" && candidateOrder != "BA") || fields["manifest_sha256"] != metadata[candidateSide+"_manifest_sha256"] || capture.headers[candidateSide][candidateSample] != "" {
				findings = append(findings, finding{path, "malformed/duplicate sample header: " + line})
				continue
			}
			side, order, sample = candidateSide, candidateOrder, candidateSample
			capture.headers[side][sample] = order
			capture.physicalOrder = append(capture.physicalOrder, rawHeader{side: side, sample: sample, order: order})
			continue
		}
		if strings.HasPrefix(line, "binary=") {
			fields := keyFields(line)
			binary = fields["binary"]
			spec, ok := benchmarkSpecFor(side, binary)
			currentSpec = spec
			expected := map[string]string{"binary": spec.name, "sha256": spec.sha256, "command": spec.runCommand}
			if side == "" || !ok {
				findings = append(findings, finding{path, "unknown raw binary target: " + line})
			} else {
				findings = append(findings, compareRecordFields(path, fmt.Sprintf("raw side=%s sample=%d binary=%s", side, sample, binary), fields, expected)...)
				identityCounts[fmt.Sprintf("%s/%d/%s/binary", side, sample, binary)]++
			}
			continue
		}
		for _, identity := range []struct{ prefix, field, want string }{{"goos: ", "goos", currentSpec.goos}, {"goarch: ", "goarch", currentSpec.goarch}, {"pkg: ", "pkg", "cervterm/" + strings.TrimPrefix(currentSpec.packagePath, "./")}} {
			if strings.HasPrefix(line, identity.prefix) {
				got := strings.TrimPrefix(line, identity.prefix)
				if currentSpec.name == "" || got != identity.want {
					findings = append(findings, finding{path, fmt.Sprintf("raw %s identity=%q want exact %q", identity.field, got, identity.want)})
				} else {
					identityCounts[fmt.Sprintf("%s/%d/%s/%s", side, sample, binary, identity.field)]++
				}
				line = ""
				break
			}
		}
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "Benchmark") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 8 || fields[3] != "ns/op" || fields[5] != "B/op" || fields[7] != "allocs/op" {
			findings = append(findings, finding{path, "malformed benchmark row: " + line})
			continue
		}
		name := strings.SplitN(fields[0], "-", 2)[0]
		ns, nsErr := strconv.ParseFloat(fields[2], 64)
		byteCount, byteErr := strconv.ParseInt(fields[4], 10, 64)
		allocs, allocErr := strconv.ParseInt(fields[6], 10, 64)
		key := rawKey{side, sample, name}
		if side == "" || sample == 0 || benchmarkBinary[side][name] != binary || nsErr != nil || byteErr != nil || allocErr != nil || math.IsNaN(ns) || math.IsInf(ns, 0) || ns <= 0 || byteCount < 0 || allocs < 0 || capture.values[key].ns != 0 {
			findings = append(findings, finding{path, "invalid, duplicate, or unbound benchmark row: " + line})
			continue
		}
		capture.values[key] = benchmarkValue{ns: ns, bytes: byteCount, allocs: allocs}
	}
	if err := scanner.Err(); err != nil {
		findings = append(findings, finding{path, err.Error()})
	}
	wantHeaders := expectedRawHeaders(path)
	findings = append(findings, validateRawHeaderSequence(path, capture.physicalOrder, wantHeaders)...)
	seenSides := make(map[string]bool)
	for _, header := range wantHeaders {
		seenSides[header.side] = true
	}
	for capturedSide := range seenSides {
		for index := 1; index <= 10; index++ {
			wantOrder := "AB"
			if index%2 == 0 {
				wantOrder = "BA"
			}
			if capture.headers[capturedSide][index] != wantOrder {
				findings = append(findings, finding{path, fmt.Sprintf("sample %d side %s order=%s want=%s", index, capturedSide, capture.headers[capturedSide][index], wantOrder)})
			}
			for benchmark := range benchmarkBinary[capturedSide] {
				if capture.values[rawKey{capturedSide, index, benchmark}].ns == 0 {
					findings = append(findings, finding{path, fmt.Sprintf("missing sample=%d side=%s benchmark=%s", index, capturedSide, benchmark)})
				}
			}
		}
	}
	for capturedSide := range seenSides {
		for index := 1; index <= 10; index++ {
			for _, spec := range benchmarkSpecsForSide(capturedSide) {
				for _, field := range []string{"binary", "goos", "goarch", "pkg"} {
					key := fmt.Sprintf("%s/%d/%s/%s", capturedSide, index, spec.name, field)
					if identityCounts[key] != 1 {
						findings = append(findings, finding{path, fmt.Sprintf("raw identity count %s=%d want 1", key, identityCounts[key])})
					}
				}
			}
		}
	}
	return capture, findings
}

func expectedRawHeaders(path string) []rawHeader {
	var headers []rawHeader
	for sample := 1; sample <= 10; sample++ {
		order := "AB"
		if sample%2 == 0 {
			order = "BA"
		}
		sides := []string{"base"}
		switch path {
		case candidateRaw:
			sides = []string{"candidate"}
		case interleavedRaw:
			sides = []string{"base", "candidate"}
			if order == "BA" {
				sides[0], sides[1] = sides[1], sides[0]
			}
		}
		for _, side := range sides {
			headers = append(headers, rawHeader{side: side, sample: sample, order: order})
		}
	}
	return headers
}

func validateRawHeaderSequence(path string, actual, expected []rawHeader) []finding {
	if reflectRawHeadersEqual(actual, expected) {
		return nil
	}
	return []finding{{path, fmt.Sprintf("physical sample order=%v want exact %v", actual, expected)}}
}

func reflectRawHeadersEqual(left, right []rawHeader) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func compareRawCaptures(base, candidate, interleaved rawCapture) []finding {
	var findings []finding
	for key, value := range base.values {
		if key.side != "base" {
			findings = append(findings, finding{baseRaw, "contains non-base sample"})
			continue
		}
		if interleaved.values[key] != value {
			findings = append(findings, finding{interleavedRaw, fmt.Sprintf("base projection mismatch %+v", key)})
		}
	}
	for key, value := range candidate.values {
		if key.side != "candidate" {
			findings = append(findings, finding{candidateRaw, "contains non-candidate sample"})
			continue
		}
		if interleaved.values[key] != value {
			findings = append(findings, finding{interleavedRaw, fmt.Sprintf("candidate projection mismatch %+v", key)})
		}
	}
	if len(interleaved.values) != len(base.values)+len(candidate.values) {
		findings = append(findings, finding{interleavedRaw, "interleaved raw set is not exact base+candidate union"})
	}
	for benchmark := range benchmarkBinary["base"] {
		baseValues := benchmarkSamples(interleaved, "base", benchmark)
		candidateValues := benchmarkSamples(interleaved, "candidate", benchmark)
		baseMedian, candidateMedian := medianNS(baseValues), medianNS(candidateValues)
		if deltaPercent(baseMedian, candidateMedian) > 3.0+1e-9 {
			findings = append(findings, finding{interleavedRaw, fmt.Sprintf("%s median regression %.6f%% exceeds 3%%", benchmark, deltaPercent(baseMedian, candidateMedian))})
		}
		baseBytes, baseAllocs := allocationMaxima(baseValues)
		candidateBytes, candidateAllocs := allocationMaxima(candidateValues)
		if candidateBytes > baseBytes || candidateAllocs > baseAllocs {
			findings = append(findings, finding{interleavedRaw, fmt.Sprintf("%s allocation maxima increased bytes %d>%d allocs %d>%d", benchmark, candidateBytes, baseBytes, candidateAllocs, baseAllocs)})
		}
	}
	return findings
}

func validateBenchmarkSummary(capture rawCapture) []finding {
	file, err := os.Open(summaryEvidence)
	if err != nil {
		return []finding{{summaryEvidence, err.Error()}}
	}
	defer file.Close()
	actual := make(map[string]summaryValue)
	var findings []finding
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := keyFields(line)
		name := fields["benchmark"]
		value := summaryValue{result: fields["result"]}
		var parseErr error
		value.baseMedian, parseErr = strconv.ParseFloat(fields["base_median_ns"], 64)
		value.candidateMedian, _ = strconv.ParseFloat(fields["candidate_median_ns"], 64)
		value.delta, _ = strconv.ParseFloat(fields["delta_percent"], 64)
		value.baseMaxBytes, _ = strconv.ParseInt(fields["base_max_bytes"], 10, 64)
		value.candidateMaxBytes, _ = strconv.ParseInt(fields["candidate_max_bytes"], 10, 64)
		value.baseMaxAllocs, _ = strconv.ParseInt(fields["base_max_allocs"], 10, 64)
		value.candidateMaxAllocs, _ = strconv.ParseInt(fields["candidate_max_allocs"], 10, 64)
		if name == "" || parseErr != nil || actual[name].result != "" {
			findings = append(findings, finding{summaryEvidence, "malformed/duplicate summary row: " + line})
			continue
		}
		actual[name] = value
	}
	for benchmark := range benchmarkBinary["base"] {
		baseValues, candidateValues := benchmarkSamples(capture, "base", benchmark), benchmarkSamples(capture, "candidate", benchmark)
		baseBytes, baseAllocs := allocationMaxima(baseValues)
		candidateBytes, candidateAllocs := allocationMaxima(candidateValues)
		want := summaryValue{medianNS(baseValues), medianNS(candidateValues), deltaPercent(medianNS(baseValues), medianNS(candidateValues)), baseBytes, candidateBytes, baseAllocs, candidateAllocs, "PASS"}
		got := actual[benchmark]
		if math.Abs(got.baseMedian-want.baseMedian) > 0.0005 || math.Abs(got.candidateMedian-want.candidateMedian) > 0.0005 || math.Abs(got.delta-want.delta) > 0.0005 || got.baseMaxBytes != want.baseMaxBytes || got.candidateMaxBytes != want.candidateMaxBytes || got.baseMaxAllocs != want.baseMaxAllocs || got.candidateMaxAllocs != want.candidateMaxAllocs || got.result != want.result {
			findings = append(findings, finding{summaryEvidence, fmt.Sprintf("summary drift %s got=%+v want=%+v", benchmark, got, want)})
		}
	}
	if len(actual) != len(benchmarkBinary["base"]) {
		findings = append(findings, finding{summaryEvidence, "summary benchmark set is not exact"})
	}
	return findings
}

func benchmarkSamples(capture rawCapture, side, benchmark string) []benchmarkValue {
	values := make([]benchmarkValue, 0, 10)
	for sample := 1; sample <= 10; sample++ {
		values = append(values, capture.values[rawKey{side, sample, benchmark}])
	}
	return values
}

func medianNS(values []benchmarkValue) float64 {
	numbers := make([]float64, len(values))
	for index, value := range values {
		numbers[index] = value.ns
	}
	sort.Float64s(numbers)
	return (numbers[4] + numbers[5]) / 2
}

func allocationMaxima(values []benchmarkValue) (int64, int64) {
	var bytesMax, allocsMax int64
	for _, value := range values {
		if value.bytes > bytesMax {
			bytesMax = value.bytes
		}
		if value.allocs > allocsMax {
			allocsMax = value.allocs
		}
	}
	return bytesMax, allocsMax
}

func deltaPercent(base, candidate float64) float64 { return (candidate/base - 1) * 100 }

func platformFields(spec platformSpec) map[string]string {
	return map[string]string{"name": spec.name, "mode": spec.mode, "goos": spec.goos, "goarch": spec.goarch, "cgo": spec.cgo, "tags": spec.tags, "package": spec.packagePath, "gomaxprocs": spec.gomaxprocs, "warmup": spec.warmup, "run": spec.run, "count": spec.count, "benchtime": spec.benchtime, "cpu": spec.cpu, "build_command": spec.buildCommand, "run_command": spec.runCommand}
}

func checkPlatformEvidence() []finding {
	findings := validateArtifactDigest(platformEvidence, platformRecordsDigest)
	file, err := os.Open(platformEvidence)
	if err != nil {
		return append(findings, finding{platformEvidence, err.Error()})
	}
	defer file.Close()
	records := make(map[string]map[string]string)
	metadata := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "platform ") {
			fields := keyFields(strings.TrimPrefix(line, "platform "))
			name := fields["name"]
			if name == "" || records[name] != nil {
				findings = append(findings, finding{platformEvidence, "malformed/duplicate platform row: " + line})
				continue
			}
			records[name] = fields
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			metadata[parts[0]] = parts[1]
		}
	}
	if err := scanner.Err(); err != nil {
		findings = append(findings, finding{platformEvidence, err.Error()})
	}
	for _, spec := range expectedPlatformSpecs {
		findings = append(findings, compareRecordFields(platformEvidence, "platform "+spec.name, records[spec.name], platformFields(spec))...)
		delete(records, spec.name)
	}
	for name := range records {
		findings = append(findings, finding{platformEvidence, "unexpected platform record " + name})
	}
	expectedMetadata := map[string]string{"host": "windows/amd64", "compiler": "go version go1.25.8 windows/amd64", "candidate_head": evidenceCandidateCommit, "linux_runtime": "WSL2 Ubuntu-24.04", "linux_mode": "execution", "linux_result": "PASS", "windows_amd64_result": "PASS", "windows_arm64_result": "PASS", "darwin_amd64_result": "PASS", "darwin_arm64_result": "PASS"}
	for key, want := range expectedMetadata {
		if metadata[key] != want {
			findings = append(findings, finding{platformEvidence, fmt.Sprintf("platform metadata %s=%q want exact %q", key, metadata[key], want)})
		}
	}
	return findings
}

func keyFields(line string) map[string]string {
	fields := make(map[string]string)
	for _, field := range strings.Fields(line) {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) == 2 {
			fields[parts[0]] = parts[1]
		}
	}
	return fields
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := strconv.ParseUint(value[:16], 16, 64)
	if err != nil {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func gitObjectExists(commit string) bool {
	command := exec.Command("git", "cat-file", "-e", commit+"^{commit}")
	return command.Run() == nil
}

func gitOutput(args ...string) ([]byte, error) {
	command := exec.Command("git", args...)
	return command.Output()
}
