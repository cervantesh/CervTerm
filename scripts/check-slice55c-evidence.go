//go:build ignore

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

const (
	baseCommit         = "09ebf4f3e668c0bba4c94f1aa7bf2e2c3e55883d"
	commitT            = "95f7269bc3ce438579c3fe256bf87edbff9e5cc3"
	commitA            = "d9885d18d3980ae1e90a7aa4df0ffaa97284167b"
	commitM            = "5c9714e9da95a7685c37aca318d8e0486c5a9413"
	commitW            = "b6d724c363bd7aa28119fd4ed208ab0b0d11f650"
	commitG            = "83be41ff1ff0c6dbc1cd67b0af1ca87f41a58def"
	evidenceBaseCommit = "504f5f54187ef9eb696bd14bd90a8c18940fb586"
	evidenceCommitT    = "8074203cfdc516be4c887c8c4ec800b187cd85bf"
	evidenceCommitW    = "7df9952db6d35a031da783479eb4d5c7f7d30ba3"
	gSubject           = "refactor(fontglyph): guard raster color and platform extraction"
	evidenceDir        = "docs/validation/architecture-maturity-slice-5.5c"
	cleanupHash        = "9a07bbf9e544ba50789083adda46849209a0393016121a3c1a51dd170277c5af"
)

var artifactHashes = map[string]string{
	"benchmark-binaries.txt":               "4172e5bf92458a2049191e92ba260821a920c2a6edd87f2d6eca98003bd987b8",
	"benchmark-summary.txt":                "7c17132af88bd65e8560b651dc6343513fdccbd8835fe308e1c9b6b7f0f774cc",
	"benchmarks-base.txt":                  "384cafde996601e0d1d61bf4befb3eb3663caef2293c10f00e9ffaee3d9ffe64",
	"benchmarks-candidate.txt":             "2fe62d89d011726eb447fa84621597a943eda980e538c1495a30ef864dbe98d6",
	"benchmarks-interleaved.txt":           "75274d589fd0e40ab86b48e9eec7c59084acb21ecf723020260b88a3b91c9b5e",
	"directwrite-allocation-benchmark.txt": "1c3b640b804bf5ebda6d21d3cec520bc466cac05a144c9b2f5e121657488befc",
	"gates.txt":                            "531e5e763bea032061e85864603cae9f0832cc7507be694e979cf13483fb86cd",
	"platform-gates.txt":                   "be8bcaa98762ad0d10d19b22dd82cec845295eb457bd6f54612c4de04adff570",
	"scope-and-commits.txt":                "a909af46b9ecb5764bbcc22fc7659da72e369ca6cda46aac5cd3dbeb431f7459",
	"source-manifest-base.txt":             "94a9328d11d8e83fc4132f39b8ba1c985a43c269017befde2359e72107110912",
	"source-manifest-candidate.txt":        "3225257f40be27a13a9022e253ed4c1b0995bf08679b1ad312e68a6b19e5d268",
}

var binaryHashes = map[string]string{
	"base/fontglyph":      "1c817fbd03e75374d15d2e4bcae47b532a4eee9cadb6ba97703200a2b79465bd",
	"base/core":           "215100d246e3184c0bec4b685a642c2c303e4bf6befe0d0c7a1298a5b06d9de6",
	"base/render":         "3535a8e405b1c4212dd5ee7f728d661a6e885eb35facd099922b703009a2c96e",
	"base/glfwgl":         "e40b5863f3fc2e42792211b6c2dd1f72e5ff783a05a39ed8d110d59f5c7ea299",
	"candidate/fontglyph": "6391475aed23a6a1c3af1a69f191d418d895bbab0444c37c045c0d0d541968b7",
	"candidate/core":      "215100d246e3184c0bec4b685a642c2c303e4bf6befe0d0c7a1298a5b06d9de6",
	"candidate/render":    "3535a8e405b1c4212dd5ee7f728d661a6e885eb35facd099922b703009a2c96e",
	"candidate/glfwgl":    "277af3d269596263999e8e39c1a4711c700eccb47b0569f5edc8e4fcf998060e",
}

var bodyPins = map[string]string{
	"internal/fontglyph/backend.go:OpenTypeBackend.Rasterize":                                                                        "2ad81df261ac9369c7a9a50d12cfac67e4c0f814518a0d4dece2d320de740e96",
	"internal/fontglyph/backend.go:bitmapColorGlyph":                                                                                 "d98dffcdcac84fe28a5de190f2bf81e0c3a7a862e1f19e3220afb7f12835eddb",
	"internal/fontglyph/backend.go:closeLoadedFace":                                                                                  "5c67cfae6673e75e279dba18dae6d4638abc3ddd1be274b2d04175db44b49b31",
	"internal/fontglyph/backend_svg.go:OpenTypeBackend.rasterizeSVGColorGlyph":                                                       "71ab4e7250c50592d9f91b93852c7b8bc7ef1a90a99389ee0d2e79a8e10de716",
	"internal/fontglyph/color_colr_render.go:OpenTypeBackend.rasterizeCOLRGlyph":                                                     "a0ea6a4b88fed2fa4694389e4e18f1a62d7d54bef0c90db832d7238c2fc18098",
	"internal/fontglyph/color_colr_render.go:OpenTypeBackend.rasterizeShapedColorCluster":                                            "ad16c58b56184896c2b9524d8e9a8f33284e99bf1de4a6c18e52de8c50a2d6d9",
	"internal/fontglyph/color_colr_render.go:rasterFace":                                                                             "792870cfbddada130413fe85743eea2f6c27cd9d7b45559f077aa804e5b00cfb",
	"internal/fontglyph/color_colr_render.go:rasterGlyphToRoot":                                                                      "37a694925e0897fca97bb4527b0c56934624b160b24db5ed9f1baead0ff97cd4",
	"internal/fontglyph/color_tables.go:DetectColorTables":                                                                           "0781038cfbf815decb13f8a0bfcd9ffb34a6c0b52c26e731ea96e22544f5fb11",
	"internal/fontglyph/directwrite_raster_facade_windows.go:dwriteRasterizer.Close":                                                 "13a4e28d99090a26baa32c9e06141adf497f767069ccd00ab28d785ca8f5cbe8",
	"internal/fontglyph/directwrite_raster_facade_windows.go:dwriteRasterizer.RasterizeGlyph":                                        "e72a8c8009692ff35832de610f01f25bf862d9b92fa449678e6b09fd1cb54caf",
	"internal/fontglyph/directwrite_raster_facade_windows.go:newDWriteRasterizer":                                                    "21f7dca8c676ebf4e5ee986fa2fb152e898eab0bce40771806a20991d494489f",
	"internal/fontglyph/directwrite_raster_facade_windows.go:newPlatformTextRasterizer":                                              "82ba73f98982a9aa9abeb678d49b102fbf856e2a5aa54eaf72f7d48f30b8f9b6",
	"internal/fontglyph/directwrite_shaper_windows.go:DirectWriteShaper.ShapeFeatures":                                               "d151f2f8eb0e677b3b44ffae1050b4e0380826d9391457a644727af6209d4abd",
	"internal/fontglyph/directwrite_shaper_windows.go:shapeWithDirectWrite":                                                          "240e708f923d474df93003a9cc6e3035ef8bebcec6843a5d30c03fa4b6b08133",
	"internal/fontglyph/platform/abi_windows.go:ABI":                                                                                 "67bfbf0c4c878d84c1e25f4444939629ead3901e38efc9e1d80bff92e947da92",
	"internal/fontglyph/platform/api_windows.go:NewDirectWriteRasterizer":                                                            "03580142044e1b3f09ca7504e2f04bf9b9588c2656512065fab721c058ace202",
	"internal/fontglyph/platform/api_windows.go:NewTextRasterizer":                                                                   "645c2af02f80f4026513c3526ae4c26d20a4bfd2a4ec81ed2a0bf6bf2ee4aa03",
	"internal/fontglyph/platform/api_windows.go:Shape":                                                                               "841b74762f596c6e7860ab3a6cfb7ddc76016410cf24416b637d7c9e77e2e2e8",
	"internal/fontglyph/platform/directwrite_raster_windows.go:dwriteRasterizer.Close":                                               "a4ca5bb4e9e331fc59e0f423c45a12895067f7d0914f4921d4a2154d67a9f3d0",
	"internal/fontglyph/raster/api.go:ColorFace.Bitmap":                                                                              "38739806dc8ba88af3c5037a3dce788b9213bab694bb7da2b46de7e29a690473",
	"internal/fontglyph/raster/api.go:ColorFace.COLRGlyph":                                                                           "78c4349b6afe8a321b3ebb1d58a31bc02fc9bc28b997674152300784175a5376",
	"internal/fontglyph/raster/api.go:ColorFace.RasterizeSVG":                                                                        "dcdb4b03ba9aba8b0898040977fe312a3a4bdae79f457ac9f62b682d155e3835",
	"internal/fontglyph/raster/api.go:NewColorFace":                                                                                  "bac41e814ba13f51029a077e46a86a021454327c5125213ced57c575331c1f52",
	"internal/fontglyph/raster/bitmap_sbix.go:sbixExtractor.glyph":                                                                   "f1951d1b5d1f2ab386494378d2dd17d3cb15a99267bd75bc98e9489a0f3ac515",
	"internal/fontglyph/raster/color_colr.go:newCOLRParser":                                                                          "d8d79b065317f051c6d2b9cc10a1ba105f2fde6e3c8a3654e0dc04b824bc634e",
	"internal/fontglyph/raster/composite.go:compositeCOLRPixel":                                                                      "b9dc855a186b0923d0d03665d59954253ab8d54d3b2ad657787f5be315b61a37",
	"internal/fontglyph/raster/portable.go:RasterizeBitmap":                                                                          "6b62273bef5d2837c118c7d06f5875382f2701fefdd9be08fdcef25aa137bbf0",
	"internal/fontglyph/raster/portable.go:RasterizeShapedBitmaps":                                                                   "62eb90206474020c71af93ff520cd22c0c698acd1ef035625fc7a85e9d4f681d",
	"internal/fontglyph/raster/portable.go:RasterizeSubpixel":                                                                        "587c65b84f0377b6533f358c0db0253bd31ee08bac61b409e5a1378eb2209c5f",
	"internal/fontglyph/raster/render.go:Renderer.RasterizeCOLRGlyph":                                                                "11465f3f3bfcb18e92035c3679afa5fa81b92b1d0cc09f8e719985d0c4312a65",
	"internal/fontglyph/raster/render.go:Renderer.RasterizeShapedColorCluster":                                                       "b9fcf4ba01380aab97fce0c7b2d1b0aad152f3bbd7609a26a8e98fd28a7402a6",
	"internal/fontglyph/raster/render.go:Renderer.RasterizeShapedMonochrome":                                                         "8dcbafcc0838e244bf9fa795d7d14488a94b029a908d7a9f9f93b7e9218e2aa5",
	"internal/fontglyph/raster_platform_characterization_test.go:TestL402RasterColorFixtureCharacterization":                         "dc00b8de240e1f94beaeabb78a11536da3df930c9f6fff8b8efdbb16f4ae44b3",
	"internal/fontglyph/raster_platform_characterization_test.go:TestL402SVGGradientFixtureCharacterization":                         "d958c5dfbf45673bffae0ff9d4b6ee7f1e7394736698d2d77acec678fc2f2fb7",
	"internal/fontglyph/raster/fuzz_test.go:FuzzPortableRasterInputs":                                                                "1eea90f30a241d8a821c458601ebabb4feb0dc6f1348739443807abb973bcb0b",
	"internal/fontglyph/raster/fuzz_test.go:TestPortableRasterFuzzSeedInventory":                                                     "9bc7a94f115df40114c079f55ddfdcd50118472f5a875ab4986f569d90b4e62f",
	"internal/fontglyph/raster/fuzz_test.go:exercisePortableRasterInput":                                                             "dcdab96cb26dbd7f306feb357c329c00a473ef4274b56a2d578d713917944b5c",
	"internal/fontglyph/directwrite_allocation_windows_test.go:BenchmarkDirectWriteShapingResultAllocations":                         "0ef03d21a4dd498f2d109acf060dfb5562a30f50e14c779820e826629a8f2050",
	"internal/fontglyph/directwrite_allocation_windows_test.go:TestDirectWriteOneResultAllocationEliminatesConversionHeapObject":     "1a5da02da687e7ee99083db652aa0de6652d2dd3af87ca7645a8763e027dbdd5",
	"internal/fontglyph/directwrite_allocation_windows_test.go:TestDirectWriteOneResultAllocationMatchesBaselineOutput":              "526eaa17664d36c349481ef56a3ef123a29a9a62d1c484b37883fc20a6a33b08",
	"internal/fontglyph/platform/abi_contracts_source_windows_test.go:TestDirectWriteABIStructsAtSourceAndRuntime":                   "d5629de02e20dfe33a0c01ea997cb5b38da228b2b8298e2e8d4de218920cbd29",
	"internal/fontglyph/platform/abi_contracts_source_windows_test.go:TestDirectWriteNativeMethodSignaturesAtSource":                 "87e17a8e09b071ed0761122a9296e8ddd0cc89551a45834c36dc556ba5213093",
	"internal/fontglyph/platform/contracts_source_test.go:TestPlatformConcreteContractsAtSourceAndRuntime":                           "2cd26c3842edbf2eaa2eb23b1a74e78cd978d424743b7759a40ddcb84a9e92c3",
	"internal/fontglyph/platform/contracts_source_test.go:TestPlatformFunctionSignaturesCompile":                                     "f354bc0aab3587cc246e438686c1f536e9a9921f10f5b761d293bbd8785833a0",
	"internal/fontglyph/platform/api_windows.go:ShapeInto":                                                                           "391453e8ea07c5b800c8c588463855d648230d9806a416fa18f78fc2edbc6e22",
	"internal/fontglyph/platform/directwrite_bridge_windows.go:shapeTextInto":                                                        "21d6cd8019ff0c80c64c6acebb101fac6cd7ac0ccbb8ad75f823ebb0db9ec450",
	"internal/fontglyph/platform/directwrite_bridge_windows.go:iWriteFactory.createFontFaceFromPathIndex":                            "8289cb96019728873ec4a1e9e46ee916d1a0c4f78d4912f73010cffc8540dd42",
	"internal/fontglyph/platform/directwrite_integration_windows_test.go:TestDirectWriteCreatesNonzeroCollectionFace":                "3119e79710498d57271b6fa8408d8696c10ed87c1a79899707c5a8d508e1ba1c",
	"internal/fontglyph/platform/directwrite_integration_windows_test.go:TestDirectWriteRasterConstructorValidatesBeforeNativeCalls": "e4719c317997370bd008d23abd919b10bd5d8196459a6164172d9db429969f14",
	"internal/fontglyph/platform/directwrite_integration_windows_test.go:TestDirectWriteRasterFaceIndexCollectionBounds":             "c187e34f39be757263dd2e8aa08efbf5b185fa555a48c033c6d2fdfcbfccb35a",
	"internal/fontglyph/platform/directwrite_raster_windows.go:iWriteFactory.openFontFile":                                           "af1087f51962383386d430684d37998db9b376285d1f032607f6091e8fd30b96",
	"internal/fontglyph/platform/directwrite_raster_windows.go:newDWriteRasterizerWithFactory":                                       "bc3cb03f442745267980450bdba932253b5b3ebd264ccd15babcb6e57b184253",
	"internal/fontglyph/platform/directwrite_raster_windows.go:validateDWriteFaceIndex":                                              "4e26c7d1bffe4c7fea54696da8d4f158de41fec80e7c78332be4c745ae70c496",
	"internal/fontglyph/platform/directwrite_raster_windows.go:validateDWriteRasterRequest":                                          "359957669b392a232c104bf8573ff446e9127891fa889f5319d685b80c54ef9b",
	"internal/fontglyph/raster/contracts_source_test.go:TestRasterConcreteContractsAtSourceAndRuntime":                               "63788bdcdbb4437fd5a4ec87399e68c9759993824f33194220c8f829f9627df7",
	"internal/fontglyph/raster/contracts_source_test.go:TestRasterMethodAndFunctionSignatures":                                       "f4de76007a825cc2cfd4cfacd8e61b35644e032a06bb92b6ab7ae8f281dbc190",
	"internal/fontglyph/root_contracts_source_test.go:TestRootConcreteABI":                                                           "7274f8d0a8a068e67eddf0f89d68930af7039674309309d722c3608fb3a49ad1",
	"internal/fontglyph/root_contracts_source_test.go:TestRootExportedMethodSets":                                                    "1366bfd291c8f92dc48da4945f58c847dc12a662562238158879dc460326663d",
	"internal/fontglyph/shaper_contracts_source_test.go:TestRootShapingContractsAtSourceAndRuntime":                                  "bca014ef33064c10b7b74ec5a0fb1eedb32d1b5869e9c7618e4d1a2ffb0408d8",
}

var stagePaths = map[string][]string{
	commitT: {
		"internal/fontglyph/raster_platform_characterization_test.go",
		"internal/fontglyph/raster_platform_characterization_windows_test.go",
	},
	commitA: {
		"internal/fontglyph/platform/contracts.go", "internal/fontglyph/platform/contracts_test.go",
		"internal/fontglyph/raster/contracts.go", "internal/fontglyph/raster/contracts_test.go",
	},
	commitM: {
		"internal/fontglyph/platform/available_windows.go", "internal/fontglyph/platform/directwrite_abi_windows_test.go",
		"internal/fontglyph/platform/directwrite_analysis_windows.go", "internal/fontglyph/platform/directwrite_analysis_windows_test.go",
		"internal/fontglyph/platform/directwrite_bridge_windows.go", "internal/fontglyph/platform/directwrite_bridge_windows_test.go",
		"internal/fontglyph/platform/directwrite_raster_windows.go", "internal/fontglyph/raster/bitmap_cbdt.go",
		"internal/fontglyph/raster/bitmap_cbdt_test.go", "internal/fontglyph/raster/bitmap_sbix.go",
		"internal/fontglyph/raster/bitmap_sbix_test.go", "internal/fontglyph/raster/color_colr.go",
		"internal/fontglyph/raster/color_colr_composite.go", "internal/fontglyph/raster/color_colr_composite_test.go",
		"internal/fontglyph/raster/color_colr_gradient.go", "internal/fontglyph/raster/color_colr_test.go",
		"internal/fontglyph/raster/color_colr_transform.go", "internal/fontglyph/raster/color_colr_types.go",
		"internal/fontglyph/raster/color_colr_variation.go", "internal/fontglyph/raster/color_colr_variation_test.go",
		"internal/fontglyph/raster/color_colr_varstore.go", "internal/fontglyph/raster/color_svg.go",
		"internal/fontglyph/raster/color_svg_gradient.go", "internal/fontglyph/raster/color_svg_path.go",
		"internal/fontglyph/raster/color_svg_raster.go", "internal/fontglyph/raster/color_svg_test.go",
		"internal/fontglyph/raster/color_svg_text.go", "internal/fontglyph/raster/color_tables.go",
		"internal/fontglyph/raster/color_tables_test.go", "internal/fontglyph/raster/fixture_test.go",
		"internal/fontglyph/raster/rgba.go", "internal/fontglyph/raster/sfnt_tables.go",
		"internal/fontglyph/raster/testdata/NotoEmoji-LICENSE.txt", "internal/fontglyph/raster/testdata/README.md",
		"internal/fontglyph/raster/testdata/colr-composite-multiply-table.bin", "internal/fontglyph/raster/testdata/colr-var-scale-table.bin",
		"internal/fontglyph/raster/testdata/cpal-red-green.bin", "internal/fontglyph/raster/testdata/noto-color-emoji-smoke.provenance.txt",
		"internal/fontglyph/raster/testdata/noto-color-emoji-smoke.ttf", "internal/fontglyph/raster/testdata/svg-gradient-table.bin",
		"internal/fontglyph/raster/testdata/svg-text-table.bin",
	},
	commitW: {
		"internal/fontglyph/backend.go", "internal/fontglyph/backend_svg.go", "internal/fontglyph/color_colr_render.go",
		"internal/fontglyph/color_tables.go", "internal/fontglyph/directwrite_allocation_windows_test.go",
		"internal/fontglyph/directwrite_raster_windows.go", "internal/fontglyph/directwrite_shaper_windows.go",
		"internal/fontglyph/font_cache.go", "internal/fontglyph/font_cache_test.go", "internal/fontglyph/font_install.go",
		"internal/fontglyph/platform/abi_windows.go", "internal/fontglyph/platform/api_stub.go",
		"internal/fontglyph/platform/api_windows.go", "internal/fontglyph/platform/contracts.go",
		"internal/fontglyph/platform/directwrite_bridge_windows.go", "internal/fontglyph/platform/directwrite_integration_windows_test.go",
		"internal/fontglyph/platform/directwrite_raster_windows.go", "internal/fontglyph/raster/api.go",
		"internal/fontglyph/raster/bitmap_sbix.go", "internal/fontglyph/raster/color_colr.go",
		"internal/fontglyph/raster/colr_raster_test.go", "internal/fontglyph/raster/composite.go",
		"internal/fontglyph/raster/contracts.go", "internal/fontglyph/raster/fixture_test.go",
		"internal/fontglyph/raster/fuzz_test.go", "internal/fontglyph/raster/portable.go",
		"internal/fontglyph/raster/render.go", "internal/fontglyph/raster/rgba.go",
		"internal/fontglyph/raster/testdata/fuzz/FuzzPortableRasterInputs/16b3cccf44f67d06",
		"internal/fontglyph/raster_platform_characterization_test.go",
		"internal/fontglyph/raster_platform_characterization_windows_test.go",
	},
}

// Filled only with paths owned by the final G commit.
var stageGPaths = strings.Fields(`
.gitattributes
docs/architecture-maturity/implementation-plan.md
docs/architecture.md
docs/validation/architecture-maturity-slice-5.5c.md
docs/validation/architecture-maturity-slice-5.5c/benchmark-binaries.txt
docs/validation/architecture-maturity-slice-5.5c/benchmark-summary.txt
docs/validation/architecture-maturity-slice-5.5c/benchmarks-base.txt
docs/validation/architecture-maturity-slice-5.5c/benchmarks-candidate.txt
docs/validation/architecture-maturity-slice-5.5c/benchmarks-interleaved.txt
docs/validation/architecture-maturity-slice-5.5c/directwrite-allocation-benchmark.txt
docs/validation/architecture-maturity-slice-5.5c/gates.txt
docs/validation/architecture-maturity-slice-5.5c/platform-gates.txt
docs/validation/architecture-maturity-slice-5.5c/scope-and-commits.txt
docs/validation/architecture-maturity-slice-5.5c/source-manifest-base.txt
docs/validation/architecture-maturity-slice-5.5c/source-manifest-candidate.txt
internal/fontglyph/backend.go
internal/fontglyph/bitmap_cbdt.go
internal/fontglyph/bitmap_cbdt_test.go
internal/fontglyph/bitmap_sbix.go
internal/fontglyph/bitmap_sbix_test.go
internal/fontglyph/color_colr.go
internal/fontglyph/color_colr_composite.go
internal/fontglyph/color_colr_composite_test.go
internal/fontglyph/color_colr_gradient.go
internal/fontglyph/color_colr_render.go
internal/fontglyph/color_colr_test.go
internal/fontglyph/color_colr_types.go
internal/fontglyph/color_colr_variation.go
internal/fontglyph/color_colr_variation_test.go
internal/fontglyph/color_colr_varstore.go
internal/fontglyph/color_svg.go
internal/fontglyph/color_svg_gradient.go
internal/fontglyph/color_svg_path.go
internal/fontglyph/color_svg_raster.go
internal/fontglyph/color_svg_test.go
internal/fontglyph/color_svg_text.go
internal/fontglyph/colr_raster_test.go
internal/fontglyph/directwrite_abi_windows_test.go
internal/fontglyph/directwrite_analysis_windows.go
internal/fontglyph/directwrite_analysis_windows_test.go
internal/fontglyph/directwrite_bridge_windows.go
internal/fontglyph/directwrite_bridge_windows_test.go
internal/fontglyph/directwrite_collection_windows_test.go
internal/fontglyph/directwrite_fontface_windows_test.go
internal/fontglyph/directwrite_raster_facade_windows.go
internal/fontglyph/directwrite_raster_windows.go
internal/fontglyph/directwrite_shape_windows_test.go
internal/fontglyph/directwrite_shaper_windows.go
internal/fontglyph/fixture_helpers_test.go
internal/fontglyph/fixture_test.go
internal/fontglyph/font_cache.go
internal/fontglyph/font_install.go
internal/fontglyph/platform/abi_contracts_source_windows_test.go
internal/fontglyph/platform/contracts_source_test.go
internal/fontglyph/raster/contracts_source_test.go
internal/fontglyph/raster/testdata/NotoEmoji-LICENSE.txt
internal/fontglyph/raster/testdata/README.md
internal/fontglyph/raster/testdata/colr-composite-multiply-table.bin
internal/fontglyph/raster/testdata/colr-var-scale-table.bin
internal/fontglyph/raster/testdata/cpal-red-green.bin
internal/fontglyph/raster/testdata/noto-color-emoji-smoke.provenance.txt
internal/fontglyph/raster/testdata/noto-color-emoji-smoke.ttf
internal/fontglyph/raster/testdata/svg-gradient-table.bin
internal/fontglyph/raster/testdata/svg-text-table.bin
internal/fontglyph/raster_compat.go
internal/fontglyph/root_contracts_source_test.go
internal/fontglyph/sfnt_tables.go
internal/fontglyph/shaper.go
internal/fontglyph/shaper_contracts_source_test.go
internal/fontglyph/shaper_default_windows.go
internal/fontglyph/subpixel.go
scripts/check-maturity-gates.go
scripts/check-slice55a-evidence.go
scripts/check-slice55b-evidence.go
scripts/check-slice55c-evidence.go
scripts/run-slice55c-linux-wsl.ps1
`)

var cleanupPaths = []string{
	"internal/fontglyph",
}

var stageSubjects = map[string]string{
	commitT: "test(fontglyph): characterize raster color and platform extraction",
	commitA: "refactor(fontglyph): add raster and platform seams",
	commitM: "refactor(fontglyph): copy raster color and native implementations",
	commitW: "refactor(fontglyph): wire raster and platform packages",
}

var obsoleteRootFiles = []string{
	"bitmap_cbdt.go", "bitmap_sbix.go", "color_colr.go", "color_colr_composite.go",
	"color_colr_variation.go", "color_colr_varstore.go", "color_svg.go", "color_svg_gradient.go",
	"color_svg_path.go", "color_svg_raster.go", "color_svg_text.go", "directwrite_analysis_windows.go",
	"directwrite_bridge_windows.go", "directwrite_raster_windows.go", "sfnt_tables.go", "rgba.go",
}

var forbiddenRootIdentifiers = map[string]bool{
	"sbixExtractor": true, "cbdtExtractor": true, "colrParser": true, "svgExtractor": true,
	"svgDocumentRecord": true, "dwriteScriptAnalysis": true, "dwriteGlyphOffset": true,
	"dwriteFeatureArguments": true, "iWriteFactory": true, "dwriteGlyphRunAnalysis": true,
	"subpixelFIRKernel": true, "premul": true,
}

var authorityDeclarations = map[string]string{
	"func:newCBDTExtractor":               "internal/fontglyph/raster/bitmap_cbdt.go",
	"func:newSbixExtractor":               "internal/fontglyph/raster/bitmap_sbix.go",
	"func:newCOLRParser":                  "internal/fontglyph/raster/color_colr.go",
	"func:newSVGExtractor":                "internal/fontglyph/raster/color_svg.go",
	"func:rasterizeSVGDocument":           "internal/fontglyph/raster/color_svg_raster.go",
	"func:compositeCOLRPixel":             "internal/fontglyph/raster/composite.go",
	"func:newDirectWriteFactory":          "internal/fontglyph/platform/directwrite_bridge_windows.go",
	"func:newDirectWriteFeatureArguments": "internal/fontglyph/platform/directwrite_bridge_windows.go",
	"type:dwriteScriptAnalysis":           "internal/fontglyph/platform/directwrite_bridge_windows.go",
	"type:dwriteGlyphOffset":              "internal/fontglyph/platform/directwrite_bridge_windows.go",
}

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "-print-body-hashes" || os.Args[1] == "-print-all-body-hashes") {
		hashes := collectBodyHashes()
		keys := sortedKeys(bodyPins)
		if os.Args[1] == "-print-all-body-hashes" {
			keys = sortedKeys(hashes)
		}
		for _, key := range keys {
			fmt.Printf("%s=%s\n", key, hashes[key])
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "-print-cleanup-hash" {
		fmt.Println(gitDiffDigest(commitW, ""))
		return
	}
	if len(os.Args) == 2 && (os.Args[1] == "-write-candidate-manifest" || os.Args[1] == "-write-base-manifest") {
		var entries map[string]string
		var failures []string
		name := "source-manifest-candidate.txt"
		if os.Args[1] == "-write-base-manifest" {
			var err error
			entries, err = manifestAtCommit(commitT)
			if err != nil {
				failures = append(failures, err.Error())
			}
			name = "source-manifest-base.txt"
		} else {
			entries, failures = presentManifest()
		}
		if len(failures) != 0 {
			for _, failure := range failures {
				fmt.Fprintln(os.Stderr, failure)
			}
			os.Exit(1)
		}
		var lines []string
		for _, path := range sortedKeys(entries) {
			lines = append(lines, entries[path]+"  "+path)
		}
		path := filepath.Join(evidenceDir, name)
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	var failures []string
	failures = append(failures, checkEvidenceFormat()...)
	failures = append(failures, checkManifests()...)
	if len(failures) != 0 {
		for _, failure := range failures {
			fmt.Fprintln(os.Stderr, "slice55c:", failure)
		}
		os.Exit(1)
	}
	failures = nil
	failures = append(failures, checkArtifacts()...)
	failures = append(failures, checkBenchmarkMetadata()...)
	failures = append(failures, checkRawBenchmarks()...)
	failures = append(failures, checkDirectWriteAllocationEvidence()...)
	failures = append(failures, checkPlatformEvidence()...)
	failures = append(failures, checkDAG()...)
	failures = append(failures, checkRootOwnership()...)
	failures = append(failures, checkConcreteRootAPI()...)
	failures = append(failures, checkBodyPins()...)
	failures = append(failures, checkABIAndBuildTags()...)
	failures = append(failures, checkFixtures()...)
	failures = append(failures, checkGuardSelfTests()...)
	failures = append(failures, checkDiffCheck()...)
	failures = append(failures, checkHistory()...)
	if len(failures) != 0 {
		for _, failure := range failures {
			fmt.Fprintln(os.Stderr, "slice55c:", failure)
		}
		os.Exit(1)
	}
	fmt.Println("slice 5.5c evidence ok")
}

func checkEvidenceFormat() []string {
	paths := []string{"docs/validation/architecture-maturity-slice-5.5c.md"}
	for name := range artifactHashes {
		paths = append(paths, filepath.Join(evidenceDir, name))
	}
	var failures []string
	for _, path := range sorted(paths) {
		data, err := os.ReadFile(path)
		if err != nil {
			failures = append(failures, path+": "+err.Error())
			continue
		}
		if bytes.ContainsRune(data, '\r') {
			failures = append(failures, path+" contains non-LF line endings")
		}
		if len(data) == 0 || !bytes.HasSuffix(data, []byte("\n")) || bytes.HasSuffix(data, []byte("\n\n")) {
			failures = append(failures, path+" must end in exactly one LF")
		}
		for index, line := range bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n")) {
			if bytes.HasSuffix(line, []byte(" ")) || bytes.HasSuffix(line, []byte("\t")) {
				failures = append(failures, fmt.Sprintf("%s trailing whitespace line %d", path, index+1))
			}
		}
	}
	return failures
}

func checkArtifacts() []string {
	var failures []string
	for name, want := range artifactHashes {
		data, err := readLF(filepath.Join(evidenceDir, name))
		if err != nil {
			failures = append(failures, name+": "+err.Error())
			continue
		}
		if got := digest(data); got != want {
			failures = append(failures, fmt.Sprintf("%s hash=%s want=%s", name, got, want))
		}
	}
	for _, path := range []string{"docs/validation/architecture-maturity-slice-5.5c.md", "docs/architecture.md", "docs/architecture-maturity/implementation-plan.md"} {
		data, err := readLF(path)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		for _, required := range []string{"Slice 5.5c", "internal/fontglyph/raster", "internal/fontglyph/platform"} {
			if path != "docs/architecture-maturity/implementation-plan.md" && !bytes.Contains(bytes.ToLower(data), bytes.ToLower([]byte(required))) {
				failures = append(failures, path+" missing "+required)
			}
		}
	}
	return failures
}

func checkBenchmarkMetadata() []string {
	data, err := readLF(filepath.Join(evidenceDir, "benchmark-binaries.txt"))
	if err != nil {
		return []string{err.Error()}
	}
	text := string(data)
	var failures []string
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for key, hash := range binaryHashes {
		side, name, _ := strings.Cut(key, "/")
		needle := "compile side=" + side + " name=" + name + " "
		var matching []string
		for _, line := range lines {
			if strings.HasPrefix(line, needle) {
				matching = append(matching, line)
			}
		}
		if len(matching) != 1 || !strings.HasSuffix(matching[0], " sha256="+hash) {
			failures = append(failures, "binary pin mismatch "+key)
		}
	}
	for _, required := range []string{
		"base_production_commit=" + evidenceBaseCommit, "base_characterization_commit=" + evidenceCommitT,
		"candidate_lineage_W=" + evidenceCommitW, "environment=GOMAXPROCS=1", "warmup=none", "samples=10",
		"physical_order=odd:AB,even:BA", "physical_witness=per-run-strict-monotonic-Stopwatch-ticks-plus-unique-GUID-nonce",
		"threshold_median_ns_percent=3", "allocation_rule=no-increase-in-worst-B/op-or-allocs/op",
		"run name=fontglyph benchmark=BenchmarkL402RasterColorGlyph", "run name=core benchmark=BenchmarkPhase15TerminalStartupMemory",
		"run name=render benchmark=BenchmarkPhase13TextOnlySnapshot", "run name=glfwgl benchmark=BenchmarkPhase13DisabledDraw",
	} {
		if strings.Count(text, required) != 1 {
			failures = append(failures, "benchmark metadata missing/duplicate "+required)
		}
	}
	if strings.Count(text, "compile side=") != 8 || strings.Count(text, "\nrun name=") != 4 {
		failures = append(failures, "compile/run record cardinality changed")
	}
	return failures
}

func checkPlatformEvidence() []string {
	data, err := readLF(filepath.Join(evidenceDir, "platform-gates.txt"))
	if err != nil {
		return []string{err.Error()}
	}
	text := string(data)
	commands := []string{
		`platform=linux/amd64 mode=execution host="WSL2 Ubuntu-24.04" suite=packages command="powershell.exe -NoProfile -File scripts/run-slice55c-linux-wsl.ps1 -Suite packages"`,
		`platform=linux/amd64 mode=execution host="WSL2 Ubuntu-24.04" suite=root command="powershell.exe -NoProfile -File scripts/run-slice55c-linux-wsl.ps1 -Suite root"`,
	}
	result := ` result=UNAVAILABLE exit=1 detail="Go toolchain not found in Ubuntu-24.04"`
	if runtime.GOOS == "windows" && exec.Command("wsl.exe", "-d", "Ubuntu-24.04", "--", "bash", "-lc", "command -v go >/dev/null 2>&1").Run() == nil {
		result = " result=PASS"
	}
	var failures []string
	for _, command := range commands {
		if strings.Count(text, command+result) != 1 {
			failures = append(failures, "platform evidence does not match executed Windows-host command: "+command)
		}
	}
	script, err := readLF("scripts/run-slice55c-linux-wsl.ps1")
	if err != nil {
		return append(failures, err.Error())
	}
	for _, required := range []string{`wsl.exe -d Ubuntu-24.04 -- bash -lc`, `repo=$(wslpath -a -u -- "$1")`, `cd -- "$repo"`, `command -v go`} {
		if strings.Count(string(script), required) != 1 {
			failures = append(failures, "Windows-host WSL runner missing/duplicate "+required)
		}
	}
	return failures
}

type benchSpec struct{ pkg, benchmark, importPath string }
type benchValue struct {
	ns            float64
	bytes, allocs int64
}
type benchBlock struct {
	sample          int
	side, pkg, text string
	value           benchValue
}
type benchWitness struct {
	sequence, sample int
	side, pkg        string
	monotonicTicks   uint64
	nonce            string
}
type benchCapture struct {
	blocks    []benchBlock
	byKey     map[string]benchBlock
	orders    []string
	witnesses []benchWitness
}

var benchSpecs = []benchSpec{
	{"fontglyph", "BenchmarkL402RasterColorGlyph", "cervterm/internal/fontglyph"},
	{"core", "BenchmarkPhase15TerminalStartupMemory", "cervterm/internal/core"},
	{"render", "BenchmarkPhase13TextOnlySnapshot", "cervterm/internal/render"},
	{"glfwgl", "BenchmarkPhase13DisabledDraw", "cervterm/internal/frontend/glfwgl"},
}
var blockHeader = regexp.MustCompile(`^### sample=([0-9]+) side=(base|candidate) package=(fontglyph|core|render|glfwgl) benchmark=(Benchmark\S+) command=(.+)$`)
var orderHeader = regexp.MustCompile(`^### sample=([0-9]+) order=(AB|BA)$`)
var benchLine = regexp.MustCompile(`^(Benchmark\S+?)(?:-\d+)?\s+\d+\s+([0-9.]+) ns/op\s+([0-9]+) B/op\s+([0-9]+) allocs/op$`)
var witnessHeader = regexp.MustCompile(`^### witness sequence=([0-9]{3}) sample=([0-9]+) side=(base|candidate) package=(fontglyph|core|render|glfwgl) monotonic_ticks=([0-9]+) nonce=([0-9a-f]{32})$`)

func checkRawBenchmarks() []string {
	base, bf := parseBench(filepath.Join(evidenceDir, "benchmarks-base.txt"))
	candidate, cf := parseBench(filepath.Join(evidenceDir, "benchmarks-candidate.txt"))
	interleaved, inf := parseBench(filepath.Join(evidenceDir, "benchmarks-interleaved.txt"))
	failures := append(append(bf, cf...), inf...)
	failures = append(failures, validateBenchSequence(base, "base", false)...)
	failures = append(failures, validateBenchSequence(candidate, "candidate", false)...)
	failures = append(failures, validateBenchSequence(interleaved, "", true)...)
	for key, block := range base.byKey {
		if got, ok := interleaved.byKey[key]; !ok || got.text != block.text {
			failures = append(failures, "interleaved base projection mismatch "+key)
		}
	}
	for key, block := range candidate.byKey {
		if got, ok := interleaved.byKey[key]; !ok || got.text != block.text {
			failures = append(failures, "interleaved candidate projection mismatch "+key)
		}
	}
	for _, spec := range benchSpecs {
		baseValues := valuesFor(base, spec.pkg)
		candidateValues := valuesFor(candidate, spec.pkg)
		if len(baseValues) != 10 || len(candidateValues) != 10 {
			failures = append(failures, fmt.Sprintf("%s physical samples=%d/%d want=10/10", spec.benchmark, len(baseValues), len(candidateValues)))
			continue
		}
		bm, cm := median(baseValues), median(candidateValues)
		if cm > bm*1.03 {
			failures = append(failures, fmt.Sprintf("%s median drift %.6f%%", spec.benchmark, (cm/bm-1)*100))
		}
		bb, ba := worstAlloc(baseValues)
		cb, ca := worstAlloc(candidateValues)
		if cb > bb || ca > ba {
			failures = append(failures, fmt.Sprintf("%s allocation drift %d/%d -> %d/%d", spec.benchmark, bb, ba, cb, ca))
		}
	}
	return failures
}

func checkDirectWriteAllocationEvidence() []string {
	path := filepath.Join(evidenceDir, "directwrite-allocation-benchmark.txt")
	data, err := readLF(path)
	if err != nil {
		return []string{err.Error()}
	}
	text := string(data)
	command := `command="go test ./internal/fontglyph -run '^$' -bench '^BenchmarkDirectWriteShapingResultAllocations$' -benchmem -benchtime=1s -count=5 -cpu=1"`
	var failures []string
	if strings.Count(text, command) != 1 || strings.Count(text, "\nPASS\n") != 1 {
		failures = append(failures, "DirectWrite allocation benchmark command/result identity changed")
	}
	row := regexp.MustCompile(`^BenchmarkDirectWriteShapingResultAllocations/(two-result-baseline|one-result-root-concrete)\s+\d+\s+[0-9.]+ ns/op\s+([0-9]+) B/op\s+([0-9]+) allocs/op$`)
	counts := map[string]int{}
	worstBytes := map[string]int64{}
	worstAllocs := map[string]int64{}
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		match := row.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		counts[match[1]]++
		bytesValue, _ := strconv.ParseInt(match[2], 10, 64)
		allocValue, _ := strconv.ParseInt(match[3], 10, 64)
		if bytesValue > worstBytes[match[1]] {
			worstBytes[match[1]] = bytesValue
		}
		if allocValue > worstAllocs[match[1]] {
			worstAllocs[match[1]] = allocValue
		}
	}
	if counts["two-result-baseline"] != 5 || counts["one-result-root-concrete"] != 5 {
		failures = append(failures, fmt.Sprintf("DirectWrite allocation samples=%v want five per result path", counts))
	}
	if worstBytes["one-result-root-concrete"] >= worstBytes["two-result-baseline"] || worstAllocs["one-result-root-concrete"] >= worstAllocs["two-result-baseline"] {
		failures = append(failures, fmt.Sprintf("DirectWrite one-result allocation evidence did not improve bytes/allocs: %v/%v", worstBytes, worstAllocs))
	}
	return failures
}

func parseBench(path string) (benchCapture, []string) {
	data, err := readLF(path)
	capture := benchCapture{byKey: make(map[string]benchBlock)}
	if err != nil {
		return capture, []string{err.Error()}
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	var failures []string
	for i := 0; i < len(lines); {
		if strings.TrimSpace(lines[i]) == "" {
			i++
			continue
		}
		if match := orderHeader.FindStringSubmatch(lines[i]); match != nil {
			capture.orders = append(capture.orders, lines[i])
			i++
			continue
		}
		if match := witnessHeader.FindStringSubmatch(lines[i]); match != nil {
			sequence, _ := strconv.Atoi(match[1])
			sample, _ := strconv.Atoi(match[2])
			ticks, _ := strconv.ParseUint(match[5], 10, 64)
			capture.witnesses = append(capture.witnesses, benchWitness{sequence: sequence, sample: sample, side: match[3], pkg: match[4], monotonicTicks: ticks, nonce: match[6]})
			i++
			continue
		}
		match := blockHeader.FindStringSubmatch(lines[i])
		if match == nil {
			failures = append(failures, fmt.Sprintf("%s unbound line %d=%q", path, i+1, lines[i]))
			i++
			continue
		}
		start := i
		sample, _ := strconv.Atoi(match[1])
		block := benchBlock{sample: sample, side: match[2], pkg: match[3]}
		i++
		bodyStart := i
		for i < len(lines) && !strings.HasPrefix(lines[i], "### ") {
			i++
		}
		body := append([]string(nil), lines[bodyStart:i]...)
		for len(body) != 0 && strings.TrimSpace(body[len(body)-1]) == "" {
			body = body[:len(body)-1]
		}
		block.text = strings.Join(append([]string{lines[start]}, body...), "\n")
		failures = append(failures, validateBenchBlock(path, &block, match[4], match[5], body)...)
		key := benchKey(block.side, block.sample, block.pkg)
		if _, exists := capture.byKey[key]; exists {
			failures = append(failures, path+" duplicate block "+key)
		} else {
			capture.byKey[key] = block
			capture.blocks = append(capture.blocks, block)
		}
	}
	return capture, failures
}

func validateBenchBlock(path string, block *benchBlock, benchmark, command string, body []string) []string {
	var failures []string
	spec, ok := specFor(block.pkg)
	if !ok || benchmark != spec.benchmark {
		failures = append(failures, path+" benchmark/package binding changed")
	}
	wantCommand := fmt.Sprintf("D:/Temp/%s55c-%s.test.exe -test.run=^$ -test.bench=^%s$ -test.benchmem -test.count=1 -test.cpu=1 -test.benchtime=1s", block.side, block.pkg, spec.benchmark)
	if command != wantCommand {
		failures = append(failures, fmt.Sprintf("%s command=%q want=%q", path, command, wantCommand))
	}
	if len(body) != 6 {
		return append(failures, fmt.Sprintf("%s block %s body lines=%d want=6", path, benchKey(block.side, block.sample, block.pkg), len(body)))
	}
	if body[0] != "goos: windows" || body[1] != "goarch: amd64" || body[2] != "pkg: "+spec.importPath || !strings.HasPrefix(body[3], "cpu: ") || body[5] != "PASS" {
		failures = append(failures, path+" physical identity block changed "+benchKey(block.side, block.sample, block.pkg))
	}
	match := benchLine.FindStringSubmatch(body[4])
	if match == nil || match[1] != spec.benchmark {
		return append(failures, path+" malformed benchmark row "+body[4])
	}
	ns, e1 := strconv.ParseFloat(match[2], 64)
	b, e2 := strconv.ParseInt(match[3], 10, 64)
	a, e3 := strconv.ParseInt(match[4], 10, 64)
	if e1 != nil || e2 != nil || e3 != nil || ns <= 0 || b < 0 || a < 0 {
		failures = append(failures, path+" invalid benchmark values")
	}
	block.value = benchValue{ns: ns, bytes: b, allocs: a}
	return failures
}

func validateBenchSequence(c benchCapture, side string, interleaved bool) []string {
	wantBlocks := 40
	if interleaved {
		wantBlocks = 80
	}
	if len(c.blocks) != wantBlocks {
		return []string{fmt.Sprintf("physical block count=%d want=%d", len(c.blocks), wantBlocks)}
	}
	var failures []string
	if interleaved && len(c.witnesses) != wantBlocks {
		failures = append(failures, fmt.Sprintf("physical witness count=%d want=%d", len(c.witnesses), wantBlocks))
	}
	if !interleaved && len(c.witnesses) != 0 {
		failures = append(failures, "projection capture unexpectedly contains physical witnesses")
	}
	index := 0
	witnessIndex := 0
	var previousTicks uint64
	nonces := make(map[string]bool)
	for sample := 1; sample <= 10; sample++ {
		order := []string{side}
		label := "AB"
		if interleaved {
			order = []string{"base", "candidate"}
			if sample%2 == 0 {
				order, label = []string{"candidate", "base"}, "BA"
			}
		}
		if interleaved {
			want := fmt.Sprintf("### sample=%d order=%s", sample, label)
			if sample-1 >= len(c.orders) || c.orders[sample-1] != want {
				failures = append(failures, "physical order header missing/reordered "+want)
			}
		}
		for _, blockSide := range order {
			for _, spec := range benchSpecs {
				block := c.blocks[index]
				index++
				if block.sample != sample || block.side != blockSide || block.pkg != spec.pkg {
					failures = append(failures, fmt.Sprintf("physical block %d reordered", index-1))
				}
				if interleaved && witnessIndex < len(c.witnesses) {
					witness := c.witnesses[witnessIndex]
					witnessIndex++
					if witness.sequence != witnessIndex || witness.sample != sample || witness.side != blockSide || witness.pkg != spec.pkg {
						failures = append(failures, fmt.Sprintf("physical witness %d does not bind its exact run", witnessIndex))
					}
					if witness.monotonicTicks <= previousTicks {
						failures = append(failures, fmt.Sprintf("physical witness %d monotonic timestamp did not increase", witnessIndex))
					}
					previousTicks = witness.monotonicTicks
					if nonces[witness.nonce] {
						failures = append(failures, fmt.Sprintf("physical witness %d reused nonce", witnessIndex))
					}
					nonces[witness.nonce] = true
				}
			}
		}
	}
	if interleaved && len(c.orders) != 10 {
		failures = append(failures, fmt.Sprintf("order headers=%d want=10", len(c.orders)))
	}
	return failures
}

func valuesFor(c benchCapture, pkg string) []benchValue {
	var out []benchValue
	for _, b := range c.blocks {
		if b.pkg == pkg {
			out = append(out, b.value)
		}
	}
	return out
}
func specFor(pkg string) (benchSpec, bool) {
	for _, s := range benchSpecs {
		if s.pkg == pkg {
			return s, true
		}
	}
	return benchSpec{}, false
}
func benchKey(side string, sample int, pkg string) string {
	return fmt.Sprintf("%s/%02d/%s", side, sample, pkg)
}
func median(values []benchValue) float64 {
	items := make([]float64, len(values))
	for i, v := range values {
		items[i] = v.ns
	}
	sort.Float64s(items)
	return (items[4] + items[5]) / 2
}
func worstAlloc(values []benchValue) (int64, int64) {
	var b, a int64
	for _, v := range values {
		if v.bytes > b {
			b = v.bytes
		}
		if v.allocs > a {
			a = v.allocs
		}
	}
	return b, a
}

func checkManifests() []string {
	basePath := filepath.Join(evidenceDir, "source-manifest-base.txt")
	candidatePath := filepath.Join(evidenceDir, "source-manifest-candidate.txt")
	baseData, be := readLF(basePath)
	candidateData, ce := readLF(candidatePath)
	baseEntries, bf := parseManifest(basePath, baseData)
	candidateEntries, cf := parseManifest(candidatePath, candidateData)
	failures := append(bf, cf...)
	if be != nil {
		failures = append(failures, be.Error())
	}
	if ce != nil {
		failures = append(failures, ce.Error())
	}
	if gitObjectExists(commitT) {
		expected, err := manifestAtCommit(commitT)
		if err != nil {
			failures = append(failures, err.Error())
		} else {
			failures = append(failures, compareManifest(basePath, baseEntries, expected)...)
		}
	}
	present, pf := presentManifest()
	failures = append(failures, pf...)
	if exactCandidateManifestRequired() {
		failures = append(failures, compareManifest(candidatePath, candidateEntries, present)...)
	} else {
		failures = append(failures, comparePinnedManifest(candidatePath, candidateEntries, present)...)
	}
	return failures
}

func exactCandidateManifestRequired() bool {
	head := git("rev-parse", "HEAD")
	return head == commitW || head == commitG
}

func parseManifest(path string, data []byte) (map[string]string, []string) {
	entries := make(map[string]string)
	var failures []string
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 || len(parts[0]) != 64 || parts[1] == "" || entries[parts[1]] != "" {
			failures = append(failures, path+" malformed/duplicate line "+line)
			continue
		}
		if _, err := hex.DecodeString(parts[0]); err != nil {
			failures = append(failures, path+" invalid digest "+parts[0])
			continue
		}
		entries[filepath.ToSlash(parts[1])] = parts[0]
	}
	return entries, failures
}
func rasterFixturePath(path string) bool {
	path = filepath.ToSlash(path)
	return strings.HasPrefix(path, "internal/fontglyph/testdata/") || strings.HasPrefix(path, "internal/fontglyph/raster/testdata/")
}

func textRasterFixturePath(path string) bool {
	if !rasterFixturePath(path) {
		return false
	}
	name := filepath.Base(filepath.ToSlash(path))
	return name == "README.md" || name == "NotoEmoji-LICENSE.txt" || name == "noto-color-emoji-smoke.provenance.txt"
}

func sourcePath(path string) bool {
	path = filepath.ToSlash(path)
	return strings.HasSuffix(path, ".go") || path == "go.mod" || path == "go.sum" || path == "scripts/run-slice55c-linux-wsl.ps1" || rasterFixturePath(path)
}

func manifestDigest(path string, data []byte) string {
	if rasterFixturePath(path) && !textRasterFixturePath(path) {
		return digest(data)
	}
	return digest(normalizeLF(data))
}
func manifestAtCommit(commit string) (map[string]string, error) {
	out := make(map[string]string)
	paths := gitLines("ls-tree", "-r", "--name-only", commit)
	for _, p := range paths {
		if !sourcePath(p) {
			continue
		}
		data, err := exec.Command("git", "show", commit+":"+p).Output()
		if err != nil {
			return nil, err
		}
		out[p] = manifestDigest(p, data)
	}
	return out, nil
}
func presentManifest() (map[string]string, []string) {
	out := make(map[string]string)
	var failures []string
	err := filepath.WalkDir(".", func(path string, e os.DirEntry, walkErr error) error {
		if walkErr != nil {
			failures = append(failures, walkErr.Error())
			return nil
		}
		rel := filepath.ToSlash(strings.TrimPrefix(path, "."+string(filepath.Separator)))
		if e.IsDir() {
			if rel == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !sourcePath(rel) || rel == "scripts/check-slice55c-evidence.go" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			failures = append(failures, err.Error())
			return nil
		}
		out[rel] = manifestDigest(rel, data)
		return nil
	})
	if err != nil {
		failures = append(failures, err.Error())
	}
	return out, failures
}
func compareManifest(label string, actual, expected map[string]string) []string {
	var failures []string
	if len(actual) != len(expected) {
		failures = append(failures, fmt.Sprintf("%s source count=%d want=%d", label, len(actual), len(expected)))
	}
	for p, w := range expected {
		if actual[p] != w {
			failures = append(failures, label+" source drift "+p)
		}
	}
	for p := range actual {
		if expected[p] == "" {
			failures = append(failures, label+" unexpected source "+p)
		}
	}
	return failures
}

// comparePinnedManifest treats the retained Slice 5.5c manifest as historical
// evidence rather than a repository-cardinality freeze. Successors may add paths,
// but retained non-guard sources and fixtures remain exact; compatibility-maintained
// guard programs are validated by the current maturity suite and rewritten-history pins.
func comparePinnedManifest(label string, pinned, present map[string]string) []string {
	var failures []string
	for p, want := range pinned {
		if guardCompatibilityPath(p) {
			continue
		}
		if present[p] != want {
			failures = append(failures, label+" pinned source/fixture drift "+p)
		}
	}
	return failures
}

func guardCompatibilityPath(path string) bool {
	switch filepath.ToSlash(path) {
	case "scripts/check-maturity-gates.go", "scripts/check-slice55a-evidence.go", "scripts/check-slice55b-evidence.go", "scripts/check-slice55c-evidence.go":
		return true
	default:
		return false
	}
}

var slice55cDAGEdges = map[string]map[string]bool{
	".": {
		"cervterm/internal/fontdesc":                true,
		"cervterm/internal/fontglyph/cache":         true,
		"cervterm/internal/fontglyph/discovery":     true,
		"cervterm/internal/fontglyph/internal/face": true,
		"cervterm/internal/fontglyph/platform":      true,
		"cervterm/internal/fontglyph/raster":        true,
		"cervterm/internal/fontglyph/shape":         true,
		"cervterm/internal/unicodecluster":          true,
	},
	"discovery":     {"cervterm/internal/fontdesc": true},
	"cache":         {"cervterm/internal/fontglyph/internal/face": true},
	"shape":         {"cervterm/internal/fontglyph/internal/face": true, "cervterm/internal/fontdesc": true, "cervterm/internal/unicodecluster": true, "cervterm/internal/unicodeprops": true},
	"raster":        {"cervterm/internal/fontglyph/internal/face": true, "cervterm/internal/fontdesc": true, "cervterm/internal/unicodecluster": true, "cervterm/internal/unicodeprops": true},
	"platform":      {"cervterm/internal/fontglyph/internal/face": true, "cervterm/internal/fontdesc": true, "cervterm/internal/unicodecluster": true, "cervterm/internal/unicodeprops": true},
	"internal/face": {"cervterm/internal/fontdesc": true},
}

func checkDAG() []string {
	var failures []string
	_ = filepath.WalkDir("internal/fontglyph", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			failures = append(failures, err.Error())
			return nil
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			failures = append(failures, parseErr.Error())
			return nil
		}
		failures = append(failures, slice55cDAGFindings(filepath.ToSlash(path), file)...)
		return nil
	})
	if out, err := exec.Command("go", "list", "-deps", "-test", "./internal/fontglyph/...").CombinedOutput(); err != nil {
		failures = append(failures, "package cycle/dependency check: "+string(out))
	}
	return failures
}

// slice55cDAGFindings applies ADR-0021 to every production Go file below
// internal/fontglyph. The root facade and known subsystems use closed edge
// allowlists. A future package is additive-safe only when it has no local
// dependency, preventing sibling and facade back-imports.
func slice55cDAGFindings(path string, file *ast.File) []string {
	rel := strings.TrimPrefix(filepath.ToSlash(path), "internal/fontglyph/")
	if rel == path {
		return []string{path + " is outside internal/fontglyph"}
	}
	directory := filepath.ToSlash(filepath.Dir(rel))
	edges := slice55cDAGEdges[directory] // Unknown additive packages default to no local edges.
	var failures []string
	for _, imported := range file.Imports {
		name, err := strconv.Unquote(imported.Path.Value)
		if err == nil && strings.HasPrefix(name, "cervterm/internal/") && !edges[name] {
			failures = append(failures, fmt.Sprintf("%s imports forbidden local package %s", path, name))
		}
	}
	return failures
}

func obsoleteRootSource(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(path))
	if filepath.ToSlash(filepath.Dir(clean)) != "internal/fontglyph" {
		return false
	}
	name := filepath.Base(clean)
	for _, obsolete := range obsoleteRootFiles {
		if name == obsolete {
			return true
		}
	}
	return false
}

func checkRootOwnership() []string {
	var failures []string
	for _, name := range obsoleteRootFiles {
		if _, err := os.Stat(filepath.Join("internal", "fontglyph", name)); err == nil {
			failures = append(failures, "obsolete root authority remains "+name)
		}
	}
	counts := make(map[string][]string)
	for _, root := range []string{"internal/fontglyph", "internal/fontglyph/raster", "internal/fontglyph/platform"} {
		_ = filepath.WalkDir(root, func(path string, e os.DirEntry, err error) error {
			if err != nil || e.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			clean := filepath.ToSlash(path)
			if root == "internal/fontglyph" && strings.Count(strings.TrimPrefix(clean, "internal/fontglyph/"), "/") > 0 {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return nil
			}
			for _, decl := range file.Decls {
				switch d := decl.(type) {
				case *ast.FuncDecl:
					if d.Recv == nil {
						counts["func:"+d.Name.Name] = append(counts["func:"+d.Name.Name], clean)
					}
				case *ast.GenDecl:
					if d.Tok == token.TYPE {
						for _, s := range d.Specs {
							ts := s.(*ast.TypeSpec)
							counts["type:"+ts.Name.Name] = append(counts["type:"+ts.Name.Name], clean)
						}
					}
				}
			}
			return nil
		})
	}
	for key, path := range authorityDeclarations {
		got := counts[key]
		if len(got) != 1 || got[0] != path {
			failures = append(failures, fmt.Sprintf("authority %s=%v want=[%s]", key, got, path))
		}
	}
	rootFiles, _ := filepath.Glob(filepath.Join("internal", "fontglyph", "*.go"))
	rootImports := make(map[string]bool)
	for _, path := range rootFiles {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		failures = append(failures, rootRetentionFindings(filepath.ToSlash(path), file)...)
		for _, item := range file.Imports {
			importPath, err := strconv.Unquote(item.Path.Value)
			if err == nil {
				rootImports[importPath] = true
			}
		}
	}
	for _, required := range []string{"cervterm/internal/fontglyph/raster", "cervterm/internal/fontglyph/platform"} {
		if !rootImports[required] {
			failures = append(failures, "root facade missing source import "+required)
		}
	}
	return failures
}

func rootRetentionFindings(path string, file *ast.File) []string {
	var failures []string
	ast.Inspect(file, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && forbiddenRootIdentifiers[id.Name] {
			failures = append(failures, path+" recursively retains owner-private identifier "+id.Name)
		}
		return true
	})
	return uniqueSorted(failures)
}

type fieldPin struct{ name, typ string }

func checkConcreteRootAPI() []string {
	types := map[string][]fieldPin{
		"RasterizedGlyph":   {{"Image", "*image.RGBA"}, {"Width", "int"}, {"Height", "int"}, {"BearingX", "int"}, {"BearingY", "int"}, {"AdvanceX", "float64"}, {"CellSpan", "int"}, {"HasColor", "bool"}, {"Subpixel", "bool"}},
		"OpenTypeBackend":   {{"faces", "[]loadedFace"}, {"fallbackSpec", "Spec"}, {"fallbacksLoaded", "bool"}, {"closed", "bool"}, {"cellW", "int"}, {"cellH", "int"}, {"baseline", "int"}, {"ppem", "uint16"}, {"shaper", "Shaper"}, {"features", "fontdesc.FeatureSet"}, {"dwRaster", "glyphRasterizer"}, {"subpixelText", "bool"}, {"closeOnce", "sync.Once"}},
		"ColorTables":       {{"HasCBDT", "bool"}, {"HasCBLC", "bool"}, {"HasSbix", "bool"}, {"HasCOLR", "bool"}, {"HasCPAL", "bool"}, {"HasSVG", "bool"}, {"HasCOLRVersion", "bool"}, {"COLRVersion", "uint16"}},
		"COLRGlyph":         {{"GlyphID", "uint16"}, {"Layers", "[]COLRLayer"}},
		"COLRLayer":         {{"GlyphID", "uint16"}, {"PaletteIndex", "uint16"}, {"Color", "color.RGBA"}, {"Foreground", "bool"}, {"Transform", "COLRTransform"}, {"Fill", "COLRFillKind"}, {"LinearGradient", "COLRLinearGradient"}, {"RadialGradient", "COLRRadialGradient"}, {"SweepGradient", "COLRSweepGradient"}, {"CompositeMode", "int"}, {"Source", "[]COLRLayer"}, {"Backdrop", "[]COLRLayer"}},
		"DirectWriteShaper": {{"Fallback", "Shaper"}},
		"ShapedGlyph":       {{"GlyphID", "uint16"}, {"XOffset", "float64"}, {"YOffset", "float64"}, {"XAdvance", "float64"}},
	}
	methodPins := map[string][]string{
		"RasterizedGlyph": {},
		"OpenTypeBackend": {"CellMetrics func() (width int, height int, baseline int)", "Close func()", "InspectClusterGlyph func(cluster string, cellSpan int) GlyphInspection", "Rasterize func(r rune, cellSpan int) (RasterizedGlyph, bool)", "RasterizeCluster func(cluster string, cellSpan int) (RasterizedGlyph, bool)", "RasterizeRun func(run string, cellSpan int) (RasterizedGlyph, bool)", "SetShaper func(shaper Shaper)", "SupportsLigatures func() bool", "TextRasterEngine func() string"},
		"ColorTables":     {"HasAnyColor func() bool", "HasBitmapColor func() bool", "HasLayerColor func() bool", "HasRenderableLayerColor func() bool", "PreferredFormat func() string"},
		"COLRGlyph":       {}, "COLRLayer": {}, "ShapedGlyph": {},
		"DirectWriteShaper": {"Available func() bool", "FeatureCapability func() string", "Shape func(cluster string, face loadedFace, ppem uint16) ([]ShapedGlyph, bool)", "ShapeFeatures func(cluster string, face loadedFace, ppem uint16, features fontdesc.FeatureSet) ([]ShapedGlyph, bool)"},
	}
	found := make(map[string][]fieldPin)
	methods := make(map[string][]string)
	var failures []string
	files, _ := filepath.Glob(filepath.Join("internal", "fontglyph", "*.go"))
	for _, path := range files {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		for _, decl := range file.Decls {
			switch item := decl.(type) {
			case *ast.GenDecl:
				if item.Tok != token.TYPE {
					continue
				}
				for _, spec := range item.Specs {
					ts := spec.(*ast.TypeSpec)
					if _, tracked := types[ts.Name.Name]; !tracked {
						continue
					}
					if ts.Assign != token.NoPos {
						failures = append(failures, "concrete root type became alias "+ts.Name.Name)
						continue
					}
					structure, ok := ts.Type.(*ast.StructType)
					if !ok {
						failures = append(failures, "concrete root type is not struct "+ts.Name.Name)
						continue
					}
					var pins []fieldPin
					for _, field := range structure.Fields.List {
						var rendered bytes.Buffer
						_ = format.Node(&rendered, token.NewFileSet(), field.Type)
						for _, name := range field.Names {
							pins = append(pins, fieldPin{name.Name, rendered.String()})
						}
					}
					found[ts.Name.Name] = pins
				}
			case *ast.FuncDecl:
				if item.Recv == nil || len(item.Recv.List) != 1 || !ast.IsExported(item.Name.Name) {
					continue
				}
				receiver := receiverName(item.Recv.List[0].Type)
				if _, tracked := methodPins[receiver]; !tracked {
					continue
				}
				var rendered bytes.Buffer
				_ = format.Node(&rendered, token.NewFileSet(), item.Type)
				methods[receiver] = append(methods[receiver], item.Name.Name+" "+rendered.String())
			}
		}
	}
	for name, want := range types {
		if !equalFieldPins(found[name], want) {
			failures = append(failures, fmt.Sprintf("root type %s fields=%v want=%v", name, found[name], want))
		}
	}
	for name, want := range methodPins {
		got := sorted(methods[name])
		want = sorted(want)
		if !equalStrings(got, want) {
			failures = append(failures, fmt.Sprintf("root type %s methods=%v want=%v", name, got, want))
		}
	}
	return failures
}

func receiverName(expr ast.Expr) string {
	switch item := expr.(type) {
	case *ast.Ident:
		return item.Name
	case *ast.StarExpr:
		return receiverName(item.X)
	}
	return ""
}

func equalFieldPins(a, b []fieldPin) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

func checkBodyPins() []string {
	actual := collectBodyHashes()
	var failures []string
	for key, want := range bodyPins {
		if actual[key] != want {
			failures = append(failures, fmt.Sprintf("body %s=%s want=%s", key, actual[key], want))
		}
	}
	return failures
}
func collectBodyHashes() map[string]string {
	out := make(map[string]string)
	_ = filepath.WalkDir("internal/fontglyph", func(path string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return nil
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			key := filepath.ToSlash(path) + ":" + functionKey(fn)
			var buf bytes.Buffer
			if err := format.Node(&buf, token.NewFileSet(), fn); err == nil {
				out[key] = digest(buf.Bytes())
			}
		}
		return nil
	})
	return out
}
func functionKey(fn *ast.FuncDecl) string {
	if fn.Recv == nil {
		return fn.Name.Name
	}
	var buf bytes.Buffer
	_ = format.Node(&buf, token.NewFileSet(), fn.Recv.List[0].Type)
	receiver := strings.TrimPrefix(strings.TrimSpace(buf.String()), "*")
	return receiver + "." + fn.Name.Name
}

func checkABIAndBuildTags() []string {
	var failures []string
	_ = filepath.WalkDir("internal/fontglyph/platform", func(path string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		name := filepath.Base(path)
		data, _ := readLF(path)
		first := strings.SplitN(string(data), "\n", 2)[0]
		if strings.Contains(name, "_windows") && first != "//go:build windows" {
			failures = append(failures, filepath.ToSlash(path)+" build tag="+first)
		}
		if name == "api_stub.go" && first != "//go:build !windows" {
			failures = append(failures, "platform stub build tag="+first)
		}
		return nil
	})
	for _, path := range []string{"internal/fontglyph/directwrite_raster_facade_windows.go", "internal/fontglyph/directwrite_shaper_windows.go", "internal/fontglyph/shaper_default_windows.go", "internal/fontglyph/raster_platform_characterization_windows_test.go"} {
		data, err := readLF(path)
		if err != nil || !bytes.HasPrefix(data, []byte("//go:build windows\n")) {
			failures = append(failures, path+" missing exact windows build tag")
		}
	}
	abi, err := readLF("internal/fontglyph/platform/abi_windows.go")
	if err != nil {
		failures = append(failures, err.Error())
	} else {
		for _, needle := range []string{"unsafe.Sizeof(dwriteScriptAnalysis{})", "unsafe.Offsetof(dwriteScriptAnalysis{}.Script)", "unsafe.Offsetof(dwriteScriptAnalysis{}.Shapes)", "unsafe.Sizeof(dwriteGlyphOffset{})"} {
			if strings.Count(string(abi), needle) != 1 {
				failures = append(failures, "ABI projection missing/duplicate "+needle)
			}
		}
	}
	if runtime.GOOS == "windows" {
		if out, err := exec.Command("go", "test", "./internal/fontglyph/platform", "./internal/fontglyph", "-run", "Test(DirectWrite|L402DirectWrite)", "-count=1").CombinedOutput(); err != nil {
			failures = append(failures, "DirectWrite host ABI/tag tests: "+string(out))
		}
	}
	return failures
}

func checkFixtures() []string {
	shared := []string{"NotoEmoji-LICENSE.txt", "README.md", "colr-composite-multiply-table.bin", "colr-var-scale-table.bin", "cpal-red-green.bin", "noto-color-emoji-smoke.provenance.txt", "noto-color-emoji-smoke.ttf", "svg-gradient-table.bin", "svg-text-table.bin"}
	expectedPaths := make([]string, 0, len(shared)+1)
	var failures []string
	for _, name := range shared {
		root := filepath.Join("internal", "fontglyph", "testdata", name)
		expectedPaths = append(expectedPaths, filepath.ToSlash(root))
		if info, err := os.Stat(root); err != nil || info.Size() == 0 {
			failures = append(failures, "missing shared raster fixture "+name)
		}
		if _, err := os.Stat(filepath.Join("internal", "fontglyph", "raster", "testdata", name)); err == nil {
			failures = append(failures, "duplicated raster fixture "+name)
		}
	}
	expectedPaths = append(expectedPaths, "internal/fontglyph/raster/testdata/fuzz/FuzzPortableRasterInputs/16b3cccf44f67d06")
	var actualPaths []string
	for _, root := range []string{"internal/fontglyph/testdata", "internal/fontglyph/raster/testdata"} {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err == nil && !entry.IsDir() {
				actualPaths = append(actualPaths, filepath.ToSlash(path))
			}
			return nil
		})
	}
	if !equalStrings(sorted(actualPaths), sorted(expectedPaths)) {
		failures = append(failures, fmt.Sprintf("raster fixture/testdata inventory=%v want=%v", sorted(actualPaths), sorted(expectedPaths)))
	}
	rasterTest, _ := readLF("internal/fontglyph/raster/fixture_test.go")
	if !bytes.Contains(rasterTest, []byte(`filepath.Join("..", "testdata", name)`)) {
		failures = append(failures, "raster fixture tests do not use sole shared fixture authority")
	}
	for _, path := range []string{"internal/fontglyph/raster/bitmap_cbdt_test.go", "internal/fontglyph/raster/bitmap_sbix_test.go", "internal/fontglyph/raster/color_colr_test.go", "internal/fontglyph/raster/color_colr_variation_test.go", "internal/fontglyph/raster/color_colr_composite_test.go", "internal/fontglyph/raster/color_svg_test.go", "internal/fontglyph/raster/fixture_test.go", "internal/fontglyph/raster/colr_raster_test.go", "internal/fontglyph/raster/fuzz_test.go"} {
		if _, err := os.Stat(path); err != nil {
			failures = append(failures, "missing raster coverage "+path)
		}
	}
	for _, path := range []string{"internal/fontglyph/platform/directwrite_abi_windows_test.go", "internal/fontglyph/platform/directwrite_analysis_windows_test.go", "internal/fontglyph/platform/directwrite_bridge_windows_test.go", "internal/fontglyph/platform/directwrite_integration_windows_test.go", "internal/fontglyph/platform/abi_contracts_source_windows_test.go", "internal/fontglyph/directwrite_allocation_windows_test.go"} {
		if _, err := os.Stat(path); err != nil {
			failures = append(failures, "missing platform coverage "+path)
		}
	}
	return failures
}

func checkGuardSelfTests() []string {
	var failures []string
	fixtures := []string{
		`package fontglyph; type hidden = colrParser`,
		`package fontglyph; type box[T any] struct{ value T }; type hidden = box[svgExtractor]`,
		`package fontglyph; func f(){ _ = struct{ value sbixExtractor }{} }`,
		`package fontglyph; var f = func(x dwriteScriptAnalysis) dwriteGlyphOffset { return dwriteGlyphOffset{} }`,
		`package fontglyph; func f(){ x := cbdtExtractor{}; _ = func(){ _ = x } }`,
	}
	for i, source := range fixtures {
		file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
		if err != nil || len(rootRetentionFindings("fixture.go", file)) == 0 {
			failures = append(failures, fmt.Sprintf("recursive AST anti-retention fixture %d escaped", i))
		}
	}

	textLF := []byte("license line\nprovenance line\n")
	textCRLF := bytes.ReplaceAll(textLF, []byte("\n"), []byte("\r\n"))
	textMutation := append([]byte(nil), textLF...)
	textMutation[0] ^= 1
	for _, path := range []string{
		"internal/fontglyph/testdata/README.md",
		"internal/fontglyph/testdata/NotoEmoji-LICENSE.txt",
		"internal/fontglyph/testdata/noto-color-emoji-smoke.provenance.txt",
	} {
		if manifestDigest(path, textLF) != manifestDigest(path, textCRLF) {
			failures = append(failures, "LF/CRLF textual fixture portability rejected for "+path)
		}
		if manifestDigest(path, textLF) == manifestDigest(path, textMutation) {
			failures = append(failures, "one-byte semantic textual fixture mutation escaped for "+path)
		}
	}
	binaryPath := "internal/fontglyph/testdata/cpal-red-green.bin"
	binaryFixture := []byte{0x00, 0x0a, 0x0d, 0xff}
	binaryMutation := append([]byte(nil), binaryFixture...)
	binaryMutation[0] ^= 1
	if manifestDigest(binaryPath, binaryFixture) == manifestDigest(binaryPath, binaryMutation) {
		failures = append(failures, "one-byte binary fixture mutation escaped byte-exact manifest guard")
	}
	if manifestDigest(binaryPath, []byte("line\n")) == manifestDigest(binaryPath, []byte("line\r\n")) {
		failures = append(failures, "binary fixture line-ending bytes were normalized")
	}
	valid := historyFacts{wExists: true, head: commitW, cleanup: cleanupHash, dirty: sorted(stageGPaths)}
	if got := validateHistoryFacts(valid); len(got) != 0 {
		failures = append(failures, "full-history pre-G fixture rejected: "+strings.Join(got, "; "))
	}
	invalid := valid
	invalid.dirty = []string{"unexpected"}
	if len(validateHistoryFacts(invalid)) == 0 {
		failures = append(failures, "invalid pre-G history fixture escaped")
	}
	fullHistoryDescendant := historyFacts{
		wExists: true, head: strings.Repeat("5", 40), cleanup: cleanupHash, gAncestor: true,
		g: []commitFact{{hash: commitG, parent: commitW, subject: gSubject, paths: sorted(stageGPaths)}},
	}
	if got := validateHistoryFacts(fullHistoryDescendant); len(got) != 0 {
		failures = append(failures, "full-history successor/tag fixture rejected: "+strings.Join(got, "; "))
	}
	shallowFixtures := []struct {
		depth int
		facts historyFacts
	}{
		{1, historyFacts{shallow: true, head: commitG, headSubject: gSubject, headParents: []string{commitW}}},
		{3, historyFacts{shallow: true, wExists: true, head: commitG, cleanup: cleanupHash, gAncestor: true, g: []commitFact{{hash: commitG, parent: commitW, subject: gSubject, paths: sorted(stageGPaths)}}}},
		{50, historyFacts{shallow: true, wExists: true, head: strings.Repeat("5", 40), cleanup: cleanupHash, gAncestor: true, g: []commitFact{{hash: commitG, parent: commitW, subject: gSubject, paths: sorted(stageGPaths)}}}},
		{200, historyFacts{shallow: true, wExists: true, head: strings.Repeat("6", 40), cleanup: cleanupHash, gAncestor: true, g: []commitFact{{hash: commitG, parent: commitW, subject: gSubject, paths: sorted(stageGPaths)}}}},
	}
	for _, fixture := range shallowFixtures {
		got := validateHistoryFacts(fixture.facts)
		if len(got) == 0 || !strings.Contains(strings.Join(got, "; "), "fetch-depth: 0") {
			failures = append(failures, fmt.Sprintf("depth-%d shallow fixture did not require fetch-depth: 0", fixture.depth))
		}
	}
	pinnedFixture := map[string]string{
		"internal/fontglyph/backend.go":                         strings.Repeat("a", 64),
		"internal/fontglyph/raster/testdata/cpal-red-green.bin": strings.Repeat("b", 64),
	}
	presentFixture := map[string]string{
		"internal/fontglyph/backend.go":                         strings.Repeat("a", 64),
		"internal/fontglyph/raster/testdata/cpal-red-green.bin": strings.Repeat("b", 64),
		"internal/fontglyph/future/additive.go":                 strings.Repeat("c", 64),
	}
	if got := comparePinnedManifest("fixture", pinnedFixture, presentFixture); len(got) != 0 {
		failures = append(failures, "safe additive package fixture rejected: "+strings.Join(got, "; "))
	}
	for _, path := range []string{"internal/fontglyph/backend.go", "internal/fontglyph/raster/testdata/cpal-red-green.bin"} {
		mutated := make(map[string]string, len(presentFixture))
		for name, hash := range presentFixture {
			mutated[name] = hash
		}
		mutated[path] = strings.Repeat("d", 64)
		if len(comparePinnedManifest("fixture", pinnedFixture, mutated)) == 0 {
			failures = append(failures, "altered pinned source/fixture escaped: "+path)
		}
	}
	dagFixtures := []struct {
		name, path, source string
		wantFailure        bool
	}{
		{"safe additive package", "internal/fontglyph/future/additive.go", `package future; import "fmt"; var _ = fmt.Sprintf`, false},
		{"root stable leaf", "internal/fontglyph/additive.go", `package fontglyph; import fontdesc "cervterm/internal/fontdesc"; var _ fontdesc.FeatureSet`, false},
		{"root forbidden action", "internal/fontglyph/additive.go", `package fontglyph; import "cervterm/internal/action"`, true},
		{"root forbidden alias import", "internal/fontglyph/additive.go", `package fontglyph; import forbidden "cervterm/internal/action"`, true},
		{"root forbidden dot import", "internal/fontglyph/additive.go", `package fontglyph; import . "cervterm/internal/action"`, true},
		{"root forbidden blank import", "internal/fontglyph/additive.go", `package fontglyph; import _ "cervterm/internal/action"`, true},
		{"additive root back-import", "internal/fontglyph/future/root.go", `package future; import _ "cervterm/internal/fontglyph"`, true},
		{"public subsystem sibling import", "internal/fontglyph/raster/shape.go", `package raster; import sibling "cervterm/internal/fontglyph/shape"`, true},
		{"public subsystem facade back-import", "internal/fontglyph/shape/root.go", `package shape; import _ "cervterm/internal/fontglyph"`, true},
		{"raster platform back-import", "internal/fontglyph/raster/platform.go", `package raster; import . "cervterm/internal/fontglyph/platform"`, true},
		{"platform raster back-import", "internal/fontglyph/platform/raster.go", `package platform; import _ "cervterm/internal/fontglyph/raster"`, true},
	}
	for _, fixture := range dagFixtures {
		file, err := parser.ParseFile(token.NewFileSet(), fixture.path, fixture.source, parser.ImportsOnly)
		got := []string(nil)
		if err == nil {
			got = slice55cDAGFindings(fixture.path, file)
		}
		if err != nil || (len(got) != 0) != fixture.wantFailure {
			failures = append(failures, "DAG fixture did not enforce "+fixture.name)
		}
	}
	if !obsoleteRootSource("internal/fontglyph/bitmap_cbdt.go") {
		failures = append(failures, "resurrected cleanup source fixture escaped")
	}
	return failures
}

type commitFact struct {
	hash, parent, subject string
	paths                 []string
}

type historyFacts struct {
	shallow                    bool
	wExists                    bool
	head, headSubject, cleanup string
	headParents                []string
	dirty                      []string
	g                          []commitFact
	gAncestor                  bool
}

func checkHistory() []string {
	head := git("rev-parse", "HEAD")
	facts := historyFacts{
		shallow: git("rev-parse", "--is-shallow-repository") == "true",
		wExists: gitObjectExists(commitW) && (head == commitW || exec.Command("git", "merge-base", "--is-ancestor", commitW, head).Run() == nil),
		head:    head, headSubject: git("show", "-s", "--format=%s", "HEAD"), headParents: commitObjectParents(head), dirty: dirtyPaths(),
	}
	if facts.shallow || !facts.wExists {
		return validateHistoryFacts(facts)
	}
	var failures []string
	parents := map[string]string{commitT: baseCommit, commitA: commitT, commitM: commitA, commitW: commitM}
	for commit, parent := range parents {
		if got := git("rev-parse", commit+"^"); got != parent {
			failures = append(failures, fmt.Sprintf("commit %s parent=%s want=%s", commit, got, parent))
		}
		if got := git("show", "-s", "--format=%s", commit); got != stageSubjects[commit] {
			failures = append(failures, fmt.Sprintf("commit %s subject=%q", commit, got))
		}
		if got := commitPaths(commit); !equalStrings(got, sorted(stagePaths[commit])) {
			failures = append(failures, fmt.Sprintf("commit %s paths=%v want=%v", commit, got, sorted(stagePaths[commit])))
		}
	}
	if head == commitW {
		facts.cleanup = gitDiffDigest(commitW, "")
	} else {
		for _, line := range gitLines("log", "HEAD", "--format=%H%x09%P%x09%s") {
			parts := strings.SplitN(line, "\t", 3)
			if len(parts) == 3 && parts[1] == commitW && parts[2] == gSubject {
				facts.g = append(facts.g, commitFact{hash: parts[0], parent: parts[1], subject: parts[2], paths: commitPaths(parts[0])})
			}
		}
		if len(facts.g) == 1 {
			facts.gAncestor = exec.Command("git", "merge-base", "--is-ancestor", facts.g[0].hash, head).Run() == nil
			facts.cleanup = gitDiffDigest(commitW, facts.g[0].hash)
		}
	}
	return append(failures, validateHistoryFacts(facts)...)
}

func validateHistoryFacts(f historyFacts) []string {
	if f.shallow {
		return []string{"shallow repository is unsupported; use actions/checkout with fetch-depth: 0 or fetch full history before running maturity gates"}
	}
	var failures []string
	if !f.wExists {
		if len(f.dirty) != 0 {
			failures = append(failures, fmt.Sprintf("detached/descendant worktree dirty=%v", f.dirty))
		}
		return append(failures, "required W/G ancestry unavailable; use actions/checkout with fetch-depth: 0 or fetch full history before running maturity gates")
	}
	if f.cleanup != cleanupHash {
		failures = append(failures, fmt.Sprintf("G cleanup hash=%s want=%s", f.cleanup, cleanupHash))
	}
	if f.head == commitW {
		if !equalStrings(f.dirty, sorted(stageGPaths)) {
			failures = append(failures, fmt.Sprintf("dirty pre-G paths=%v want=%v", f.dirty, sorted(stageGPaths)))
		}
		return failures
	}
	if len(f.g) != 1 {
		return append(failures, fmt.Sprintf("G commit cardinality=%d want=1", len(f.g)))
	}
	g := f.g[0]
	if g.hash != commitG || g.parent != commitW || g.subject != gSubject {
		failures = append(failures, "G hash/parent/subject identity changed")
	}
	if !equalStrings(g.paths, sorted(stageGPaths)) {
		failures = append(failures, fmt.Sprintf("G paths=%v want=%v", g.paths, sorted(stageGPaths)))
	}
	if f.head != g.hash && !f.gAncestor {
		failures = append(failures, "unique G is not ancestor of HEAD")
	}
	if len(f.dirty) != 0 {
		failures = append(failures, fmt.Sprintf("clean G/merged worktree dirty=%v", f.dirty))
	}
	return failures
}

func checkDiffCheck() []string {
	gates, err := readLF(filepath.Join(evidenceDir, "gates.txt"))
	if err != nil {
		return []string{err.Error()}
	}
	retainedCommand := "git diff --check " + evidenceBaseCommit + "..HEAD"
	if strings.Count(string(gates), `command="`+retainedCommand+`" result=PASS`) != 1 {
		return []string{"gates must retain exact " + retainedCommand + " semantics"}
	}
	if !gitObjectExists(baseCommit) {
		return nil
	}
	command := "git diff --check " + baseCommit + "..HEAD"
	out, err := exec.Command("git", "diff", "--check", baseCommit+"..HEAD").CombinedOutput()
	if err != nil {
		return []string{command + ": " + strings.TrimSpace(string(out))}
	}
	return nil
}

func gitDiffDigest(from, to string) string {
	args := []string{"diff", "--no-ext-diff", "--binary", from}
	if to != "" {
		args = append(args, to)
	}
	args = append(args, "--")
	args = append(args, cleanupPaths...)
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return ""
	}
	return digest(normalizeLF(out))
}
func dirtyPaths() []string {
	set := map[string]bool{}
	for _, args := range [][]string{{"diff", "--no-renames", "--name-only"}, {"diff", "--cached", "--no-renames", "--name-only"}, {"ls-files", "--others", "--exclude-standard"}} {
		for _, p := range gitLines(args...) {
			set[filepath.ToSlash(p)] = true
		}
	}
	var out []string
	for p := range set {
		out = append(out, p)
	}
	return sorted(out)
}
func commitObjectParents(commit string) []string {
	out, err := exec.Command("git", "cat-file", "-p", commit).Output()
	if err != nil {
		return nil
	}
	var parents []string
	for _, line := range strings.Split(string(normalizeLF(out)), "\n") {
		if strings.HasPrefix(line, "parent ") {
			parents = append(parents, strings.TrimPrefix(line, "parent "))
		}
		if line == "" {
			break
		}
	}
	return parents
}

func commitPaths(commit string) []string {
	return sorted(gitLines("diff-tree", "--no-commit-id", "--name-only", "-r", commit))
}
func gitObjectExists(commit string) bool {
	return exec.Command("git", "cat-file", "-e", commit+"^{commit}").Run() == nil
}
func git(args ...string) string {
	out, _ := exec.Command("git", args...).Output()
	return strings.TrimSpace(string(out))
}
func gitLines(args ...string) []string {
	text := git(args...)
	if text == "" {
		return nil
	}
	return strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
}
func gitOutput(name string, args ...string) string {
	out, _ := exec.Command(name, args...).Output()
	return string(normalizeLF(out))
}
func readLF(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return normalizeLF(data), nil
}
func normalizeLF(data []byte) []byte {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))
}
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func sorted(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}
func sortedKeys[V any](values map[string]V) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	values = sorted(values)
	out := values[:1]
	for _, v := range values[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
