//go:build ignore

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	baseCommit        = "10857a0a53815e375894bebebc7326c119fda350"
	commitT           = "09da2261e2965e2bbe0d56f49f1b20eab7ca4e13"
	commitA           = "b8d603295e82cc581b86711f06208af04afe911e"
	commitM           = "2b3686a029a611a74be621dc8b09dc8bd28e5826"
	commitW           = "b98ee43fe500f40fbd6c16799d1b58551e85cfb7"
	commitG           = "92fa34a2d18455581a8916ef1a2bcb5af195c881"
	successorW        = "b6d724c363bd7aa28119fd4ed208ab0b0d11f650"
	evidenceBase      = "973660df6ea31112a6984a39421ad60783e706d4"
	evidenceT         = "d0bfd354bc088093621348a817ed005cc1e1df70"
	evidenceW         = "d6e1a81dfedb4f8910b9340b6da0d3c2b8b8291c"
	gSubject          = "refactor(fontglyph): guard resolution and shaping extraction"
	evidence          = "docs/validation/architecture-maturity-slice-5.5b"
	cleanupDiffHash   = "ed6becd4ae19c55f89083d817fdd3049536208a919e2baf945fd222af6d53e8c"
	validationDocHash = "82bc83c45298d174d0362a693ba26ad4aa4b7d5fe8395a74b7182c9b47bd6b62"
)

var artifactHashes = map[string]string{
	"benchmark-binaries.txt":        "51e3b8332d15566048f99febf9d0e9a6cf7e23ec03f67aca93346131d895351e",
	"benchmarks-base.txt":           "9248acb0a46c6a2e45cd409bf5bb384b5e355ca4d0a865a4e39628043f456761",
	"benchmarks-candidate.txt":      "3c49404a19ab113b8a68353ab69a2eba1ee42f2e7abc6f6a3a3eedd7b4a64038",
	"benchmarks-interleaved.txt":    "2fd35cd67b5bcde0edcfd0892584bac52b7611958fce6bd1f8aa47b1f8c6e5dc",
	"platform-gates.txt":            "179b950bbfc577f21f1bf63e8061c58ff4755c794fbc0e4d93418fb162acf36a",
	"scope-and-commits.txt":         "75f9e2bb0ac4423a5259ca6c10d9d9e30390d3241830a586ee685f9606dd681e",
	"source-manifest-base.txt":      "ab85a007fa45960054d5783caa641137ca5641efffd8d19512f2cc037324ab01",
	"source-manifest-candidate.txt": "7aa058a776c6f57cf686b7ac675da430caf0c94f8a37abf1a813a1393bd8bee9",
}

var binaryHashes = map[string]string{
	"base-fontglyph.test.exe":      "be32a9a7bbc38f315dfd09c29dbcb808f83a9540f02aee913fde348499568df0",
	"base-core.test.exe":           "215100d246e3184c0bec4b685a642c2c303e4bf6befe0d0c7a1298a5b06d9de6",
	"base-render.test.exe":         "3535a8e405b1c4212dd5ee7f728d661a6e885eb35facd099922b703009a2c96e",
	"base-glfwgl.test.exe":         "fcc6a5b9cc862a50b2028f80d12be512097965074c218ceb5acea2962092ab8e",
	"candidate-fontglyph.test.exe": "c457bba3eb3cd48ced097bdd758914d366e6944849bec656e0cfff82d276963b",
	"candidate-core.test.exe":      "215100d246e3184c0bec4b685a642c2c303e4bf6befe0d0c7a1298a5b06d9de6",
	"candidate-render.test.exe":    "3535a8e405b1c4212dd5ee7f728d661a6e885eb35facd099922b703009a2c96e",
	"candidate-glfwgl.test.exe":    "e40b5863f3fc2e42792211b6c2dd1f72e5ff783a05a39ed8d110d59f5c7ea299",
}

var functionHashes = map[string]string{
	"internal/fontglyph/shape/resolver.go:Resolver.DescriptorPlans":                                                      "bc0868f17e63f25e7d453654e9b09f55bb080d3b4dbb155ca43072a849e2e98c",
	"internal/fontglyph/shape/policy.go:NewPolicy":                                                                       "6c97e129fd558d669270abbddb2b82bd75679d68a74db2416153d97524739cdb",
	"internal/fontglyph/shape/policy.go:Policy.Resolve":                                                                  "748749ab3796bc97ce35321e4a874c957fff697a813fbd0407157e53643a314d",
	"internal/fontglyph/shape/policy.go:Policy.tryPlans":                                                                 "d13c8b5b97545f8048e532d765b0f2e0be9f662aad93951141ad9e8f255dbdef",
	"internal/fontglyph/shape/policy.go:Policy.recordFailure":                                                            "71451bb5f57926935878195a33abe0b3ce287550b09e60a464009ef4c35a96cf",
	"internal/fontglyph/shape/policy.go:Policy.rememberLocked":                                                           "7c1aaed91c83fa6a67db9c9a7cb7c2a4d83e9fc0636a72ab18916bad70262378",
	"internal/fontglyph/shape/policy.go:Policy.Close":                                                                    "ba6edfa188fa12b64c4bb2bd3a34d18cac7f6cbffdaed596e4d9a360047a341b",
	"internal/fontglyph/shape/shaping.go:Simple.Shape":                                                                   "6d7ca30eaf5f2b884793a0ddc00ed2a8f5e4948b3f1d821f9514f51642ca41ad",
	"internal/fontglyph/shape/shaping.go:ShapeOneRune":                                                                   "ccf17472ceae9e275a2b7c15f7cc06014b911d86fe1b4884a2ae2bf1eac97643",
	"internal/fontglyph/shape/shaping.go:CenterInCells":                                                                  "fa4aea54fc8f3f0adec5787d772ad47c5dacbf69d32f636dfe6134f44001284c",
	"internal/fontglyph/shape/shaping.go:RunSubstituted":                                                                 "063f60c50689e4179912753ef4492f82576ca532b174a60d4085d210f40696e4",
	"internal/fontglyph/shape/implementation_test.go:TestPolicyCachesUseHardBudget":                                      "ed8dd06b344981f68715a85d7ae494ab1e2fc1b25408810d0460b3668e31a1c2",
	"internal/fontglyph/shape/implementation_test.go:TestPolicyPositiveCacheEvictsInDeterministicFIFOOrder":              "49c272d58375b96152e3a4baf15419284f0c3df489cc698df067ee677d916f89",
	"internal/fontglyph/shape/implementation_test.go:TestPolicyResolveCloseLifecycleRetainsInputsUntilInflightCompletes": "b21f78c902e36798afd896b5e1b1511184990869ddfe6b1798eb4329e45911e0",
	"internal/fontglyph/shape/implementation_test.go:TestNewPolicyValidatesAllAuthoredBudgets":                           "9d13f7008a7c1212d1820588b805638dea846d32ee2ba9c3f305d9d2e2ad3b03",
	"internal/fontglyph/backend.go:normalizeClusterToSingleRune":                                                         "56110eb0d472bd95980fa49b427a75ba53dec918b6bfabd693971bc49d98a4d6",
	"internal/fontglyph/shape_facade.go:shapeResolver":                                                                   "a767cf26f8844e938b2876defc77937bd3baa5ee66a4178139fac2e00070e5fe",
	"internal/fontglyph/shape_facade.go:resolvedPlanFromShape":                                                           "d36dfc1c5f9537dffd94eb0a786c642713db50c0d542e43f4b3feedf7fc53c47",
	"internal/fontglyph/shape_facade.go:resolvedPlanToShape":                                                             "390faeebbc88edb6d911896533f77bf01702a9418c4dd65ab3c2b88ae8b5caf9",
	"internal/fontglyph/shape_facade.go:resolvedPlansFromShape":                                                          "c09dc339a2c3f740bccb22ffeefc17bc636961d4d100d04e946155c6eb447027",
	"internal/fontglyph/shape_facade.go:resolveDescriptorFacePlans":                                                      "20bae7f021d676dea0f525197f1887435bfe8096c7457f88f12aff21c2d9eacb",
	"internal/fontglyph/shape_facade.go:classifySyntheticFallback":                                                       "1434fb88880bde6f4a3c78c341f9fe1b3b5b01df9e7a00f2ce5d0079c8b197a6",
	"internal/fontglyph/shape_facade.go:newResolvedFacePlan":                                                             "a8a7d3f8cd6ff951f6c0a89becd4426362ea9df13cb3941dbae65d92e0ce21f2",
	"internal/fontglyph/shape_facade.go:resolveEmbeddedFallbackPlan":                                                     "c8e20126e56c1060b40cb62de9a1e6ea7d95165373e221de1441e09cd39565fd",
	"internal/fontglyph/shape_facade.go:resolveFaceCandidates":                                                           "dd2062731c4a214d3d6f2510510e06604829447eaa2ced6d1307feeb23f0f8fb",
	"internal/fontglyph/shape_facade.go:shapingFaceRef":                                                                  "950a4076b006a5ef42845dc706e165bbe47f09f773d5bd08c44b794767d2b560",
	"internal/fontglyph/shape_facade.go:shapedGlyphsFromShape":                                                           "386bcd0afc79875859f9e5880b26f6b17683ad7c7cb749d4fd82834cbda1695a",
	"internal/fontglyph/shape_facade.go:shapedGlyphsToShape":                                                             "a774b3eb7f1dfa78ac01bcb8db55a177a017470cd52373df73453575a7e412c3",
	"internal/fontglyph/shape_facade.go:rootToShapeShaper.Shape":                                                         "ba739fc7e50f41498e8121d943895ab9a7603601255fd4b780e5ccb8d6f1c94e",
	"internal/fontglyph/shape_facade.go:rootToShapeShaper.ShapeFeatures":                                                 "1b60fc24c86587852894540fc294f2f82d67c9ca5ff9972bfeff651d8a0149c7",
	"internal/fontglyph/shape_facade.go:loadedFaceFromShapingRef":                                                        "b36c4ac85139ae53003a26c058d9a4bcd42a5f8c528cf2fd2cc048ff72909637",
	"internal/fontglyph/shape_facade.go:shapeToRootShaper.Shape":                                                         "2c75eef427cd3452736ef8d280bf3a42d485232abd3cbb747780d4403c9cc8e6",
	"internal/fontglyph/shape_facade.go:shapeToRootShaper.ShapeFeatures":                                                 "6c984c3b702aab4d3d0c872b7c5caa8b9bfec265ad5dd8a41734ea6b95b0bfef",
	"internal/fontglyph/shape_facade.go:shapeToRootShaper.FeatureCapability":                                             "a2c9225bf5f9be1d52b4e5d992614ba91a33a92c197080df0d09382c2dfbc095",
	"internal/fontglyph/shaper_default.go:newDefaultShaper":                                                              "eee8aeaaccd83a5298076e6d88a8bcf04f31c2732cee9a782039405b601fba4b",
	"internal/fontglyph/shaper_default_windows.go:newDefaultShaper":                                                      "a638a6a0a06c08efa8a41efc9a43695f7ab37f73fc0580736abe981bde641133",
	"internal/fontglyph/simple_shaper.go:SimpleShaper.FeatureCapability":                                                 "c80070e4faedd366e8da9c0bf0db1f08f624d44e34ac5f16e5ac5e17c4e96b89",
	"internal/fontglyph/shape_facade.go:fallbackBackend.installShapePolicy":                                              "5b9307d70c45c1a6606980d890ae2eec990ed7c56ddb8f5f52b5d4cff53b3255",
	"internal/fontglyph/shape_facade.go:fallbackBackend.loadShapePlan":                                                   "64b63d815000dc0d50fab25bbd50a39004dd5ab7c3777e9d84b37dc5624bb9da",
	"internal/fontglyph/shape_facade.go:fallbackBackend.removeLoadedOrder":                                               "5d2c5d20cb117834b54f294304ba7ef8327cd079cc88fdf2092d365aa7cd5bf2",
	"internal/fontglyph/shape_facade.go:fallbackBackend.resolveContent":                                                  "0e1fbdd53db382501b564019b6fe10602e04583d7291929c0cf3814c5db0ff32",
	"internal/fontglyph/shape_facade.go:resolvePrimaryFacePlan":                                                          "66b04178fd13771d7e5be448266aae376caf890a224d1f8e5da8117afee9ba57",
	"internal/fontglyph/shape_facade.go:rootShaperFromShape":                                                             "d4c73f2e68d5a87977f16215c548c704a0c848c3d19a2892f2ce34f6a318ce70",
	"internal/fontglyph/fallback_resolver.go:fallbackBackend.Close":                                                      "1fec88bc21bc7ae1efb4b22f06c51aa8f54954a67be2ef0536af1210dc5d5258",
	"internal/fontglyph/fallback_resolver_test.go:TestFallbackBackendReleasesLosingCandidatePins":                        "0f59a5349e3c22846a148bc5e0eca02f13240bc0e4baf343c8eebcce5f5486f3",
	"internal/fontglyph/fallback_resolver_test.go:TestFallbackBackendClosesRetainedBackendsInReverseAcquisitionOrder":    "8842a5b6823d4a0d8ff18921a538c4ab31c38fc50c024c9e12bb37c9a3f5c4ea",
	"internal/fontglyph/runshape.go:runSubstituted":                                                                      "1b3d5d76f50934d9f771092aafea0af089385643c3917228f971ead1ccf0e6bc",
	"internal/fontglyph/shaper.go:centerShapedGlyphsInCells":                                                             "9433b4fd4b2c42eab446a5721ee1e5bf7e55d40f5aa92a9741cb0958a25c4bb5",
	"internal/fontglyph/shaper.go:isPortableShaper":                                                                      "83689c321131f315ac4305758a52eb717eb662499a9d019e7d1670322f77061f",
	"internal/fontglyph/simple_shaper.go:SimpleShaper.ShapeFeatures":                                                     "4e3cf1dae9c10f3f62e58baa5e8bd2a02877c0ff1e872233c10e5002e743bddc",
	"internal/fontglyph/simple_shaper.go:SimpleShaper.Shape":                                                             "43e876e0ac19353bef12e3fefdf3863bced3679c030e7adc07b87bcc9142c557",
	"internal/fontglyph/simple_shaper.go:shapeOneRune":                                                                   "3a4ceeaa6f53eda77b92035ff981d237f64b18e314256e6c6ab4a37da9b4d128",
	"internal/fontglyph/simple_shaper.go:isSimpleShapeableCluster":                                                       "2fea76b220ee17017ad46e5f124be6cb93620ca6ebf2c4a439e21c593781aca1",
	"internal/fontglyph/simple_shaper.go:isComplexShapingRune":                                                           "a2e69beda9f480b31b5b9ae7fbc13be6d780024e7abbe9281320a00a5b064c06",
}

var stagePaths = map[string][]string{
	commitT: {"internal/fontglyph/shape_characterization_test.go"},
	commitA: {
		"internal/fontglyph/internal/face/ref.go", "internal/fontglyph/shape/contracts.go", "internal/fontglyph/shape/contracts_test.go",
		"internal/fontglyph/shape/policy_contracts.go", "internal/fontglyph/shape/resolver_contracts.go",
	},
	commitM: {
		"internal/fontglyph/shape/implementation_test.go", "internal/fontglyph/shape/policy.go",
		"internal/fontglyph/shape/resolver.go", "internal/fontglyph/shape/shaping.go",
	},
	commitW: {
		"internal/fontdesc/features.go", "internal/fontglyph/backend.go", "internal/fontglyph/face_resolver.go",
		"internal/fontglyph/fallback_resolver.go", "internal/fontglyph/fallback_resolver_test.go", "internal/fontglyph/internal/face/ref.go",
		"internal/fontglyph/runshape.go", "internal/fontglyph/shape/implementation_test.go", "internal/fontglyph/shape/policy.go",
		"internal/fontglyph/shape/policy_contracts.go", "internal/fontglyph/shape/resolver.go", "internal/fontglyph/shape/shaping.go",
		"internal/fontglyph/shape_characterization_test.go", "internal/fontglyph/shape_facade.go", "internal/fontglyph/shaper.go",
		"internal/fontglyph/shaper_default.go", "internal/fontglyph/shaper_default_windows.go", "internal/fontglyph/simple_shaper.go",
	},
}

// Filled after all G files exist. Dirty pre-G and clean post-G both require this exact set.
var stageGPaths = []string{
	"docs/architecture-maturity/implementation-plan.md",
	"docs/architecture.md",
	"docs/validation/architecture-maturity-slice-5.5b.md",
	"docs/validation/architecture-maturity-slice-5.5b/benchmark-binaries.txt",
	"docs/validation/architecture-maturity-slice-5.5b/benchmarks-base.txt",
	"docs/validation/architecture-maturity-slice-5.5b/benchmarks-candidate.txt",
	"docs/validation/architecture-maturity-slice-5.5b/benchmarks-interleaved.txt",
	"docs/validation/architecture-maturity-slice-5.5b/platform-gates.txt",
	"docs/validation/architecture-maturity-slice-5.5b/scope-and-commits.txt",
	"docs/validation/architecture-maturity-slice-5.5b/source-manifest-base.txt",
	"docs/validation/architecture-maturity-slice-5.5b/source-manifest-candidate.txt",
	"internal/fontglyph/backend.go",
	"internal/fontglyph/face_resolver.go",
	"internal/fontglyph/fallback_resolver.go",
	"internal/fontglyph/fallback_resolver_test.go",
	"internal/fontglyph/shape_characterization_test.go",
	"internal/fontglyph/shape_facade.go",
	"internal/fontglyph/simple_shaper.go",
	"scripts/check-maturity-gates.go",
	"scripts/check-slice55b-evidence.go",
}

var cleanupPaths = []string{
	"internal/fontglyph/backend.go",
	"internal/fontglyph/face_resolver.go",
	"internal/fontglyph/fallback_resolver.go",
	"internal/fontglyph/fallback_resolver_test.go",
	"internal/fontglyph/shape_characterization_test.go",
	"internal/fontglyph/shape_facade.go",
	"internal/fontglyph/simple_shaper.go",
}

var stageSubjects = map[string]string{
	commitT: "test(fontglyph): characterize resolution and shaping extraction",
	commitA: "refactor(fontglyph): add resolution and shaping seams",
	commitM: "refactor(fontglyph): copy resolution and shaping implementations",
	commitW: "refactor(fontglyph): wire resolution and shaping package",
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "-print-hashes" {
		for key, value := range collectFunctionHashes() {
			fmt.Printf("%s=%s\n", key, value)
		}
		return
	}
	var failures []string
	failures = append(failures, checkArtifacts()...)
	failures = append(failures, checkBenchmarkMetadata()...)
	failures = append(failures, checkRawSamples()...)
	failures = append(failures, checkManifests()...)
	failures = append(failures, checkDAG()...)
	failures = append(failures, checkAPIAndRetention()...)
	failures = append(failures, checkFunctionBodies()...)
	failures = append(failures, checkGuardSelfTests()...)
	failures = append(failures, checkCommitsAndPaths()...)
	if len(failures) != 0 {
		for _, failure := range failures {
			fmt.Fprintln(os.Stderr, "slice55b:", failure)
		}
		os.Exit(1)
	}
	fmt.Println("slice 5.5b evidence ok")
}

func readLF(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	data = bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))
	return data, nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func checkArtifacts() []string {
	var failures []string
	for name, want := range artifactHashes {
		data, err := readLF(filepath.Join(evidence, name))
		if err != nil {
			failures = append(failures, name+": "+err.Error())
			continue
		}
		if got := digest(data); got != want {
			failures = append(failures, fmt.Sprintf("%s hash=%s want=%s", name, got, want))
		}
	}
	doc, err := readLF("docs/validation/architecture-maturity-slice-5.5b.md")
	if err != nil {
		failures = append(failures, err.Error())
	} else {
		if got := digest(doc); got != validationDocHash {
			failures = append(failures, fmt.Sprintf("validation narrative hash=%s want=%s", got, validationDocHash))
		}
		for _, required := range []string{evidenceW, "exactly ten physical samples", "No push or merge", "internal/fontglyph/shape"} {
			if !bytes.Contains(doc, []byte(required)) {
				failures = append(failures, "validation doc missing "+required)
			}
		}
	}
	return failures
}

func checkBenchmarkMetadata() []string {
	data, err := readLF(filepath.Join(evidence, "benchmark-binaries.txt"))
	if err != nil {
		return []string{err.Error()}
	}
	return validateBenchmarkMetadata(string(data))
}

func validateBenchmarkMetadata(text string) []string {
	var failures []string
	for name, hash := range binaryHashes {
		if strings.Count(text, name) != 1 || !strings.Contains(text, "sha256="+hash) {
			failures = append(failures, "binary metadata mismatch for "+name)
		}
	}
	for _, required := range []string{
		"base-production=" + evidenceBase, "base-harness=" + evidenceT, "candidate-production=present-tree-w-semantic",
		"candidate-lineage-w=" + evidenceW, "candidate-harness=present-tree", "candidate-source-state=dirty-pre-G",
		"source-manifest-base-sha256=" + artifactHashes["source-manifest-base.txt"],
		"source-manifest-candidate-sha256=" + artifactHashes["source-manifest-candidate.txt"],
		"warmup=none", "samples=10", "physical-order=odd:AB,even:BA",
		"threshold-median-ns-percent=3", "allocation-rule=no-increase-in-worst-B/op-or-allocs/op",
	} {
		if strings.Count(text, required) != 1 {
			failures = append(failures, "benchmark metadata missing/duplicate "+required)
		}
	}
	if strings.Count(text, "compile side=") != 8 || strings.Count(text, "\nrun package=") != 4 {
		failures = append(failures, "compile/run record cardinality changed")
	}
	for _, required := range []string{
		`compile side=base package=./internal/fontglyph GOOS=windows GOARCH=amd64 tags=none command="go test -c -trimpath -o base-fontglyph.test.exe ./internal/fontglyph" sha256=` + binaryHashes["base-fontglyph.test.exe"],
		`compile side=base package=./internal/core GOOS=windows GOARCH=amd64 tags=none command="go test -c -trimpath -o base-core.test.exe ./internal/core" sha256=` + binaryHashes["base-core.test.exe"],
		`compile side=base package=./internal/render GOOS=windows GOARCH=amd64 tags=none command="go test -c -trimpath -o base-render.test.exe ./internal/render" sha256=` + binaryHashes["base-render.test.exe"],
		`compile side=base package=./internal/frontend/glfwgl GOOS=windows GOARCH=amd64 tags=glfw command="go test -c -trimpath -tags glfw -o base-glfwgl.test.exe ./internal/frontend/glfwgl" sha256=` + binaryHashes["base-glfwgl.test.exe"],
		`compile side=candidate package=./internal/fontglyph GOOS=windows GOARCH=amd64 tags=none command="go test -c -trimpath -o candidate-fontglyph.test.exe ./internal/fontglyph" sha256=` + binaryHashes["candidate-fontglyph.test.exe"],
		`compile side=candidate package=./internal/core GOOS=windows GOARCH=amd64 tags=none command="go test -c -trimpath -o candidate-core.test.exe ./internal/core" sha256=` + binaryHashes["candidate-core.test.exe"],
		`compile side=candidate package=./internal/render GOOS=windows GOARCH=amd64 tags=none command="go test -c -trimpath -o candidate-render.test.exe ./internal/render" sha256=` + binaryHashes["candidate-render.test.exe"],
		`compile side=candidate package=./internal/frontend/glfwgl GOOS=windows GOARCH=amd64 tags=glfw command="go test -c -trimpath -tags glfw -o candidate-glfwgl.test.exe ./internal/frontend/glfwgl" sha256=` + binaryHashes["candidate-glfwgl.test.exe"],
		`run package=fontglyph GOMAXPROCS=1 cpu=1 run="^$" bench="BenchmarkL402(ResolvePrimaryPlans|SimpleShapeASCII|FallbackResolutionHit)$" benchmem=true benchtime=1s count=1`,
		`run package=core GOMAXPROCS=1 cpu=1 run="^$" bench="BenchmarkPhase15TerminalStartupMemory$" benchmem=true benchtime=1s count=1`,
		`run package=render GOMAXPROCS=1 cpu=1 run="^$" bench="BenchmarkPhase13TextOnlySnapshot$" benchmem=true benchtime=1s count=1`,
		`run package=glfwgl GOMAXPROCS=1 cpu=1 run="^$" bench="BenchmarkPhase13DisabledDraw$" benchmem=true benchtime=1s count=1`,
	} {
		if strings.Count(text, required) != 1 {
			failures = append(failures, "exact compile/run record missing or duplicated: "+required)
		}
	}
	return failures
}

type benchValue struct {
	ns     float64
	bytes  int64
	allocs int64
}

type benchRow struct {
	name  string
	value benchValue
}

type benchBlock struct {
	header, side, order, pkg, text string
	sample                         int
	rows                           []benchRow
}

type benchCapture struct {
	blocks  []benchBlock
	byKey   map[string]benchBlock
	values  map[string][]benchValue
	headers []string
}

var benchLine = regexp.MustCompile(`^(Benchmark\S+?)(?:-\d+)?\s+\d+\s+([0-9.]+) ns/op\s+([0-9]+) B/op\s+([0-9]+) allocs/op$`)
var sampleHeader = regexp.MustCompile(`^=== sample=([0-9]{2}) order=(AB|BA) side=(base|candidate) package=(fontglyph|core|render|glfwgl) ===$`)

var benchmarkNames = map[string][]string{
	"fontglyph": {"BenchmarkL402ResolvePrimaryPlans", "BenchmarkL402SimpleShapeASCII", "BenchmarkL402FallbackResolutionHit"},
	"core":      {"BenchmarkPhase15TerminalStartupMemory"},
	"render":    {"BenchmarkPhase13TextOnlySnapshot"},
	"glfwgl":    {"BenchmarkPhase13DisabledDraw"},
}

var benchmarkPatterns = map[string]string{
	"fontglyph": "BenchmarkL402(ResolvePrimaryPlans|SimpleShapeASCII|FallbackResolutionHit)$",
	"core":      "BenchmarkPhase15TerminalStartupMemory$",
	"render":    "BenchmarkPhase13TextOnlySnapshot$",
	"glfwgl":    "BenchmarkPhase13DisabledDraw$",
}

var benchmarkPackages = map[string]string{
	"fontglyph": "cervterm/internal/fontglyph",
	"core":      "cervterm/internal/core",
	"render":    "cervterm/internal/render",
	"glfwgl":    "cervterm/internal/frontend/glfwgl",
}

func parseBench(path string) (benchCapture, []string) {
	data, err := readLF(path)
	if err != nil {
		return benchCapture{}, []string{err.Error()}
	}
	return parseBenchData(path, data)
}

func parseBenchData(path string, data []byte) (benchCapture, []string) {
	capture := benchCapture{byKey: make(map[string]benchBlock), values: make(map[string][]benchValue)}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	var failures []string
	for index := 0; index < len(lines); {
		if strings.TrimSpace(lines[index]) == "" {
			index++
			continue
		}
		match := sampleHeader.FindStringSubmatch(lines[index])
		if match == nil {
			failures = append(failures, fmt.Sprintf("%s unbound physical line %d=%q", path, index+1, lines[index]))
			index++
			continue
		}
		start := index
		sample, _ := strconv.Atoi(match[1])
		block := benchBlock{header: lines[index], side: match[3], order: match[2], pkg: match[4], sample: sample}
		index++
		for index < len(lines) && !strings.HasPrefix(lines[index], "=== sample=") {
			index++
		}
		body := append([]string(nil), lines[start+1:index]...)
		for len(body) != 0 && strings.TrimSpace(body[len(body)-1]) == "" {
			body = body[:len(body)-1]
		}
		blockLines := append([]string{block.header}, body...)
		block.text = strings.Join(blockLines, "\n")
		capture.headers = append(capture.headers, block.header)
		failures = append(failures, validateBenchBlock(path, &block, body)...)
		key := benchBlockKey(block.side, block.sample, block.pkg)
		if _, duplicate := capture.byKey[key]; duplicate {
			failures = append(failures, path+" duplicate physical block "+key)
		} else {
			capture.byKey[key] = block
			capture.blocks = append(capture.blocks, block)
			for _, row := range block.rows {
				capture.values[row.name] = append(capture.values[row.name], row.value)
			}
		}
	}
	return capture, failures
}

func validateBenchBlock(path string, block *benchBlock, body []string) []string {
	var failures []string
	wantRows := benchmarkNames[block.pkg]
	wantLength := 6 + len(wantRows)
	if len(body) != wantLength {
		failures = append(failures, fmt.Sprintf("%s block %s body lines=%d want=%d", path, benchBlockKey(block.side, block.sample, block.pkg), len(body), wantLength))
		return failures
	}
	wantCommand := fmt.Sprintf(`command: GOMAXPROCS=1 C:\temp\cerv55b-bin\%s-%s.test.exe -test.run=^$ -test.bench=%s -test.benchmem -test.benchtime=1s -test.count=1 -test.cpu=1`, block.side, block.pkg, benchmarkPatterns[block.pkg])
	exact := []string{wantCommand, "goos: windows", "goarch: amd64", "pkg: " + benchmarkPackages[block.pkg]}
	for index, want := range exact {
		if body[index] != want {
			failures = append(failures, fmt.Sprintf("%s block %s identity row %d=%q want=%q", path, benchBlockKey(block.side, block.sample, block.pkg), index, body[index], want))
		}
	}
	if !strings.HasPrefix(body[4], "cpu: ") || strings.TrimSpace(strings.TrimPrefix(body[4], "cpu: ")) == "" {
		failures = append(failures, path+" block "+benchBlockKey(block.side, block.sample, block.pkg)+" missing CPU identity")
	}
	for index, wantName := range wantRows {
		line := body[5+index]
		match := benchLine.FindStringSubmatch(line)
		if match == nil {
			failures = append(failures, fmt.Sprintf("%s block %s malformed benchmark row %q", path, benchBlockKey(block.side, block.sample, block.pkg), line))
			continue
		}
		if match[1] != wantName {
			failures = append(failures, fmt.Sprintf("%s block %s benchmark row %d=%s want=%s", path, benchBlockKey(block.side, block.sample, block.pkg), index, match[1], wantName))
		}
		ns, nsErr := strconv.ParseFloat(match[2], 64)
		byteCount, byteErr := strconv.ParseInt(match[3], 10, 64)
		allocs, allocErr := strconv.ParseInt(match[4], 10, 64)
		if nsErr != nil || byteErr != nil || allocErr != nil || ns <= 0 || byteCount < 0 || allocs < 0 {
			failures = append(failures, fmt.Sprintf("%s block %s invalid benchmark value %q", path, benchBlockKey(block.side, block.sample, block.pkg), line))
			continue
		}
		block.rows = append(block.rows, benchRow{name: match[1], value: benchValue{ns: ns, bytes: byteCount, allocs: allocs}})
	}
	if body[len(body)-1] != "PASS" {
		failures = append(failures, fmt.Sprintf("%s block %s terminal row=%q want PASS", path, benchBlockKey(block.side, block.sample, block.pkg), body[len(body)-1]))
	}
	return failures
}

func benchBlockKey(side string, sample int, pkg string) string {
	return fmt.Sprintf("%s/%02d/%s", side, sample, pkg)
}

func compareBenchCaptures(base, candidate, interleaved benchCapture) []string {
	var failures []string
	for _, sideCapture := range []benchCapture{base, candidate} {
		for key, block := range sideCapture.byKey {
			if got, ok := interleaved.byKey[key]; !ok || got.text != block.text {
				failures = append(failures, "interleaved exact block projection mismatch "+key)
			}
		}
	}
	if len(interleaved.byKey) != len(base.byKey)+len(candidate.byKey) {
		failures = append(failures, fmt.Sprintf("interleaved block set=%d want exact %d", len(interleaved.byKey), len(base.byKey)+len(candidate.byKey)))
	}
	return failures
}

func checkRawSamples() []string {
	base, baseFailures := parseBench(filepath.Join(evidence, "benchmarks-base.txt"))
	candidate, candidateFailures := parseBench(filepath.Join(evidence, "benchmarks-candidate.txt"))
	interleaved, interleavedFailures := parseBench(filepath.Join(evidence, "benchmarks-interleaved.txt"))
	failures := append(append(baseFailures, candidateFailures...), interleavedFailures...)
	failures = append(failures, validateSideHeaders(base.headers, "base")...)
	failures = append(failures, validateSideHeaders(candidate.headers, "candidate")...)
	failures = append(failures, validateInterleavedHeaders(interleaved.headers)...)
	failures = append(failures, compareBenchCaptures(base, candidate, interleaved)...)
	for name, baseRows := range base.values {
		candidateRows := candidate.values[name]
		if len(baseRows) != 10 || len(candidateRows) != 10 {
			failures = append(failures, fmt.Sprintf("%s rows base/candidate=%d/%d want=10/10", name, len(baseRows), len(candidateRows)))
			continue
		}
		baseMedian, candidateMedian := median(baseRows), median(candidateRows)
		if candidateMedian > baseMedian*1.03 {
			failures = append(failures, fmt.Sprintf("%s median drift %.6f%%", name, (candidateMedian/baseMedian-1)*100))
		}
		baseB, baseA := worstAlloc(baseRows)
		candidateB, candidateA := worstAlloc(candidateRows)
		if candidateB > baseB || candidateA > baseA {
			failures = append(failures, fmt.Sprintf("%s allocation drift %d/%d -> %d/%d", name, baseB, baseA, candidateB, candidateA))
		}
	}
	if len(base.values) != 6 || len(candidate.values) != 6 {
		failures = append(failures, fmt.Sprintf("benchmark names base/candidate=%d/%d want=6/6", len(base.values), len(candidate.values)))
	}
	return failures
}

func validateSideHeaders(headers []string, side string) []string {
	var failures []string
	if len(headers) != 40 {
		return []string{fmt.Sprintf("%s physical headers=%d want=40", side, len(headers))}
	}
	for i, header := range headers {
		match := sampleHeader.FindStringSubmatch(header)
		if match == nil || match[3] != side {
			failures = append(failures, fmt.Sprintf("%s malformed header %d=%q", side, i, header))
			continue
		}
		sample := i/4 + 1
		label := "AB"
		if sample%2 == 0 {
			label = "BA"
		}
		packages := []string{"fontglyph", "core", "render", "glfwgl"}
		if match[1] != fmt.Sprintf("%02d", sample) || match[2] != label || match[4] != packages[i%len(packages)] {
			failures = append(failures, fmt.Sprintf("%s reordered header %d=%q", side, i, header))
		}
	}
	return failures
}

func validateInterleavedHeaders(headers []string) []string {
	if len(headers) != 80 {
		return []string{fmt.Sprintf("interleaved physical headers=%d want=80", len(headers))}
	}
	var failures []string
	index := 0
	packages := []string{"fontglyph", "core", "render", "glfwgl"}
	for sample := 1; sample <= 10; sample++ {
		order := []string{"base", "candidate"}
		label := "AB"
		if sample%2 == 0 {
			order, label = []string{"candidate", "base"}, "BA"
		}
		for _, side := range order {
			for _, pkg := range packages {
				want := fmt.Sprintf("=== sample=%02d order=%s side=%s package=%s ===", sample, label, side, pkg)
				if headers[index] != want {
					failures = append(failures, fmt.Sprintf("interleaved header %d=%q want=%q", index, headers[index], want))
				}
				index++
			}
		}
	}
	return failures
}

func median(values []benchValue) float64 {
	items := make([]float64, len(values))
	for i, value := range values {
		items[i] = value.ns
	}
	sort.Float64s(items)
	return (items[4] + items[5]) / 2
}

func worstAlloc(values []benchValue) (int64, int64) {
	var b, a int64
	for _, value := range values {
		if value.bytes > b {
			b = value.bytes
		}
		if value.allocs > a {
			a = value.allocs
		}
	}
	return b, a
}

func checkManifests() []string {
	basePath := filepath.Join(evidence, "source-manifest-base.txt")
	candidatePath := filepath.Join(evidence, "source-manifest-candidate.txt")
	baseData, baseErr := readLF(basePath)
	candidateData, candidateErr := readLF(candidatePath)
	var failures []string
	if baseErr != nil {
		failures = append(failures, baseErr.Error())
	}
	if candidateErr != nil {
		failures = append(failures, candidateErr.Error())
	}
	baseEntries, baseParseFailures := parseManifest(basePath, baseData)
	candidateEntries, candidateParseFailures := parseManifest(candidatePath, candidateData)
	failures = append(failures, baseParseFailures...)
	failures = append(failures, candidateParseFailures...)
	if gitObjectExists(baseCommit) && gitObjectExists(commitT) {
		wantBase, err := manifestEntriesAtCommit(baseCommit)
		if err != nil {
			failures = append(failures, "immutable base manifest: "+err.Error())
		} else {
			overlay, overlayErr := exec.Command("git", "show", commitT+":internal/fontglyph/shape_characterization_test.go").Output()
			if overlayErr != nil {
				failures = append(failures, "immutable base harness overlay: "+overlayErr.Error())
			} else {
				wantBase["internal/fontglyph/shape_characterization_test.go"] = digest(normalizeLF(overlay))
				failures = append(failures, compareManifestEntries(basePath, baseEntries, wantBase)...)
			}
		}
	}
	head := git("rev-parse", "HEAD")
	if inSuccessorMode(head) {
		if gitObjectExists(commitG) {
			wantCandidate, err := manifestEntriesAtCommit(commitG)
			if err != nil {
				failures = append(failures, "historical candidate manifest: "+err.Error())
			} else {
				delete(wantCandidate, "scripts/check-slice55b-evidence.go")
				failures = append(failures, comparePinnedManifestEntries(candidatePath, candidateEntries, wantCandidate)...)
			}
		}
	} else {
		wantCandidate, presentFailures := presentTreeManifestEntries()
		failures = append(failures, presentFailures...)
		failures = append(failures, compareManifestEntries(candidatePath, candidateEntries, wantCandidate)...)
	}
	return failures
}

func normalizeLF(data []byte) []byte {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))
}

func sourceManifestPath(path string) bool {
	return strings.HasSuffix(path, ".go") || path == "go.mod" || path == "go.sum"
}

func parseManifest(path string, data []byte) (map[string]string, []string) {
	entries := make(map[string]string)
	if data == nil {
		return entries, nil
	}
	var failures []string
	previous := ""
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 || len(parts[0]) != 64 || parts[1] == "" || filepath.ToSlash(parts[1]) != parts[1] || previous >= parts[1] || entries[parts[1]] != "" {
			failures = append(failures, path+" malformed, duplicate, or unsorted manifest line "+line)
			continue
		}
		if _, err := hex.DecodeString(parts[0]); err != nil {
			failures = append(failures, path+" invalid SHA-256 "+parts[0])
			continue
		}
		entries[parts[1]] = parts[0]
		previous = parts[1]
	}
	if len(entries) == 0 {
		failures = append(failures, path+" empty manifest")
	}
	for _, required := range []string{"go.mod", "go.sum", "internal/fontglyph/shape_characterization_test.go"} {
		if entries[required] == "" {
			failures = append(failures, path+" missing "+required)
		}
	}
	return entries, failures
}

func manifestEntriesAtCommit(commit string) (map[string]string, error) {
	listing, err := exec.Command("git", "ls-tree", "-r", "--name-only", commit).Output()
	if err != nil {
		return nil, err
	}
	entries := make(map[string]string)
	for _, rawPath := range strings.Split(strings.TrimSpace(string(listing)), "\n") {
		path := filepath.ToSlash(strings.TrimSpace(rawPath))
		if !sourceManifestPath(path) {
			continue
		}
		content, showErr := exec.Command("git", "show", commit+":"+path).Output()
		if showErr != nil {
			return nil, showErr
		}
		entries[path] = digest(normalizeLF(content))
	}
	return entries, nil
}

func presentTreeManifestEntries() (map[string]string, []string) {
	entries := make(map[string]string)
	var failures []string
	err := filepath.WalkDir(".", func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			failures = append(failures, path+": "+walkErr.Error())
			return nil
		}
		rel := filepath.ToSlash(strings.TrimPrefix(path, "."+string(filepath.Separator)))
		if entry.IsDir() {
			if rel == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !sourceManifestPath(rel) || rel == "scripts/check-slice55b-evidence.go" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			failures = append(failures, rel+": "+readErr.Error())
			return nil
		}
		entries[rel] = digest(normalizeLF(data))
		return nil
	})
	if err != nil {
		failures = append(failures, err.Error())
	}
	return entries, failures
}

func compareManifestEntries(path string, actual, expected map[string]string) []string {
	var failures []string
	if len(actual) != len(expected) {
		failures = append(failures, fmt.Sprintf("%s source path count=%d want=%d", path, len(actual), len(expected)))
	}
	for sourcePath, want := range expected {
		if actual[sourcePath] != want {
			failures = append(failures, path+" present-tree source drift "+sourcePath)
		}
	}
	for sourcePath := range actual {
		if expected[sourcePath] == "" {
			failures = append(failures, path+" unexpected source path "+sourcePath)
		}
	}
	return failures
}

// comparePinnedManifestEntries preserves the historical manifest as an immutable
// ledger while allowing later slices to add source paths of their own.
func comparePinnedManifestEntries(path string, actual, pinned map[string]string) []string {
	var failures []string
	for sourcePath, want := range pinned {
		if actual[sourcePath] != want {
			failures = append(failures, path+" pinned manifest entry drift "+sourcePath)
		}
	}
	return failures
}

func checkDAG() []string {
	allowed := map[string]map[string]bool{
		"shape":         {"cervterm/internal/fontdesc": true, "cervterm/internal/fontglyph/internal/face": true, "cervterm/internal/unicodecluster": true, "cervterm/internal/unicodeprops": true},
		"raster":        {"cervterm/internal/fontdesc": true, "cervterm/internal/fontglyph/internal/face": true, "cervterm/internal/unicodecluster": true, "cervterm/internal/unicodeprops": true},
		"platform":      {"cervterm/internal/fontdesc": true, "cervterm/internal/fontglyph/internal/face": true, "cervterm/internal/unicodecluster": true, "cervterm/internal/unicodeprops": true},
		"cache":         {"cervterm/internal/fontglyph/internal/face": true},
		"discovery":     {"cervterm/internal/fontdesc": true},
		"internal/face": {"cervterm/internal/fontdesc": true},
	}
	var failures []string
	_ = filepath.WalkDir("internal/fontglyph", func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel("internal/fontglyph", filepath.Dir(path))
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		switch rel {
		case "shape", "raster", "platform", "cache", "discovery", "internal/face":
		default:
			return nil
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			failures = append(failures, path+": "+parseErr.Error())
			return nil
		}
		for _, forbidden := range forbiddenLocalImports(rel, file, allowed) {
			failures = append(failures, fmt.Sprintf("%s forbidden local import %s", path, forbidden))
		}
		return nil
	})
	command := exec.Command("go", "list", "-deps", "-test", "./internal/fontglyph/...")
	if output, err := command.CombinedOutput(); err != nil {
		failures = append(failures, "package cycle/dependency gate: "+string(output))
	}
	return failures
}

func forbiddenLocalImports(subsystem string, file *ast.File, allowed map[string]map[string]bool) []string {
	var forbidden []string
	for _, item := range file.Imports {
		importPath, _ := strconv.Unquote(item.Path.Value)
		if strings.HasPrefix(importPath, "cervterm/") && !allowed[subsystem][importPath] {
			forbidden = append(forbidden, importPath)
		}
	}
	return forbidden
}

func checkAPIAndRetention() []string {
	var failures []string
	checks := []struct {
		path, name string
		fields     []string
	}{
		{"internal/fontglyph/shaper.go", "ShapedGlyph", []string{"GlyphID", "XOffset", "YOffset", "XAdvance"}},
		{"internal/fontglyph/simple_shaper.go", "SimpleShaper", nil},
		{"internal/fontglyph/descriptor_backend.go", "FaceDiagnostic", []string{"Metadata", "Tier", "AuthoredIndex", "Synthetic"}},
		{"internal/fontglyph/fallback_resolver.go", "fallbackBackend", []string{"primary", "spec", "environment", "index", "features", "loaded", "loadedOrder", "load", "covers", "policy", "closed", "closeOnce"}},
		{"internal/fontglyph/shape_facade.go", "rootToShapeShaper", []string{"root"}},
		{"internal/fontglyph/shape_facade.go", "shapeToRootShaper", []string{"inner"}},
		{"internal/fontglyph/face_resolver.go", "resolvedFacePlan", []string{"descriptor", "target", "selected", "tier", "authoredIndex", "synthetic", "canonicalFaceID", "resolvedKey"}},
	}
	for _, check := range checks {
		file, err := parser.ParseFile(token.NewFileSet(), check.path, nil, 0)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		fields, alias, found := structFields(file, check.name)
		if !validConcreteStruct(fields, alias, found, check.fields) {
			failures = append(failures, fmt.Sprintf("%s concrete fields=%v alias=%v want=%v", check.name, fields, alias, check.fields))
		}
	}
	methods := exportedMethods("internal/fontglyph/simple_shaper.go", "SimpleShaper")
	if !equalStrings(methods, []string{"FeatureCapability", "Shape", "ShapeFeatures"}) {
		failures = append(failures, "SimpleShaper method inventory="+fmt.Sprint(methods))
	}
	for receiver, want := range map[string][]string{
		"rootToShapeShaper": {"Shape", "ShapeFeatures"},
		"shapeToRootShaper": {"FeatureCapability", "Shape", "ShapeFeatures"},
	} {
		if got := exportedMethods("internal/fontglyph/shape_facade.go", receiver); !equalStrings(got, want) {
			failures = append(failures, receiver+" method inventory="+fmt.Sprint(got))
		}
	}
	for _, path := range []string{"internal/fontglyph/face_resolver.go", "internal/fontglyph/fallback_resolver.go", "internal/fontglyph/shape_facade.go", "internal/fontglyph/simple_shaper.go"} {
		data, _ := readLF(path)
		for _, forbidden := range []string{"legacyResolve", "legacySimple", "contentResolutionKey", "loadFailedRing", "resolvedRing"} {
			if bytes.Contains(data, []byte(forbidden)) {
				failures = append(failures, path+" retains obsolete authority "+forbidden)
			}
		}
	}
	failures = append(failures, checkRootAntiRetention()...)
	failures = append(failures, checkRootShapeFacadeInventory()...)
	return failures
}

func validConcreteStruct(fields []string, alias, found bool, want []string) bool {
	return found && !alias && equalStrings(fields, want)
}

func structFields(file *ast.File, name string) ([]string, bool, bool) {
	for _, declaration := range file.Decls {
		gen, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != name {
				continue
			}
			structure, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				return nil, typeSpec.Assign.IsValid(), true
			}
			var fields []string
			for _, field := range structure.Fields.List {
				if len(field.Names) == 0 {
					fields = append(fields, "<embedded>")
					continue
				}
				for _, fieldName := range field.Names {
					fields = append(fields, fieldName.Name)
				}
			}
			return fields, typeSpec.Assign.IsValid(), true
		}
	}
	return nil, false, false
}

func exportedMethods(path, receiver string) []string {
	file, _ := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	var methods []string
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Recv != nil && recvName(function.Recv.List[0].Type) == receiver && ast.IsExported(function.Name.Name) {
			methods = append(methods, function.Name.Name)
		}
	}
	sort.Strings(methods)
	return methods
}

func checkFunctionBodies() []string {
	return validateFunctionHashes(collectFunctionHashes(), functionHashes)
}

func validateFunctionHashes(actual, want map[string]string) []string {
	var failures []string
	for key, expected := range want {
		if got := actual[key]; got != expected {
			failures = append(failures, fmt.Sprintf("body %s=%s want=%s", key, got, expected))
		}
	}
	return failures
}

func collectFunctionHashes() map[string]string {
	paths := map[string]bool{}
	for key := range functionHashes {
		paths[strings.SplitN(key, ":", 2)[0]] = true
	}
	out := map[string]string{}
	for path := range paths {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			continue
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			name := function.Name.Name
			if function.Recv != nil {
				name = recvName(function.Recv.List[0].Type) + "." + name
			}
			key := path + ":" + name
			if _, wanted := functionHashes[key]; !wanted {
				continue
			}
			var buffer bytes.Buffer
			_ = format.Node(&buffer, token.NewFileSet(), function.Type)
			_ = format.Node(&buffer, token.NewFileSet(), function.Body)
			out[key] = digest(buffer.Bytes())
		}
	}
	return out
}

func recvName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return recvName(value.X)
	case *ast.IndexExpr:
		return recvName(value.X)
	case *ast.IndexListExpr:
		return recvName(value.X)
	}
	return "?"
}

func checkGuardSelfTests() []string {
	var failures []string
	retentionFixtures := []string{
		`package fontglyph; type hidden = contentResolutionKey`,
		`package fontglyph; type box[T any] struct{ value T }; type hidden = box[contentResolutionKey]`,
		`package fontglyph; type hidden struct{ contentResolutionKey }`,
		`package fontglyph; func f(x contentResolutionKey) { y := x; _ = []any{y} }`,
		`package fontglyph; var f = func(x contentResolutionKey) contentResolutionKey { return x }`,
		`package fontglyph; func f(x contentResolutionKey) { escape := func() any { return x }; _ = escape }`,
		`package fontglyph; func f() { _ = struct{ value contentResolutionKey }{} }`,
	}
	for index, source := range retentionFixtures {
		file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
		if err != nil || !retainsForbidden(file, rootForbiddenAuthority()) {
			failures = append(failures, fmt.Sprintf("anti-retention fixture %d was not rejected", index))
		}
	}
	production, err := readLF("internal/fontglyph/fallback_resolver.go")
	if err != nil {
		failures = append(failures, "real-source anti-retention fixture: "+err.Error())
	} else {
		mutations := []string{
			`type retainedAlias = contentResolutionKey`,
			`type retainedBox[T any] struct{ value T }; type retainedGeneric = retainedBox[contentResolutionKey]`,
			`type retainedEmbedding struct{ contentResolutionKey }`,
			`var retainedLiteral = func(value contentResolutionKey) contentResolutionKey { return value }`,
			`func retainedCapture(value contentResolutionKey) { inferred := value; escape := func() any { return inferred }; _ = escape }`,
			`func retainedComposite() { value := struct{ nested []contentResolutionKey }{}; _ = value }`,
		}
		for index, mutation := range mutations {
			source := append(append([]byte(nil), production...), []byte("\n"+mutation+"\n")...)
			file, parseErr := parser.ParseFile(token.NewFileSet(), "internal/fontglyph/fallback_resolver.go", source, 0)
			if parseErr != nil || !retainsForbidden(file, rootForbiddenAuthority()) {
				failures = append(failures, fmt.Sprintf("real-source anti-retention mutation %d escaped", index))
			}
		}
		mutatedSource := append(append([]byte(nil), production...), []byte("\ntype escapedShapeAuthority = shapepkg.Policy\n")...)
		mutatedFile, parseErr := parser.ParseFile(token.NewFileSet(), "internal/fontglyph/fallback_resolver.go", mutatedSource, 0)
		if parseErr != nil {
			failures = append(failures, "real-source shape facade mutation did not parse")
		} else if keys := rootShapeFacadeDeclarationKeys("internal/fontglyph/fallback_resolver.go", mutatedFile); keys["internal/fontglyph/fallback_resolver.go:type:escapedShapeAuthority"] == 0 {
			failures = append(failures, "real-source shape authority alias escaped canonical facade inventory")
		}
	}

	apiFixtures := []struct {
		name   string
		source string
		want   []string
	}{
		{"SimpleShaper alias", `package fontglyph; type SimpleShaper = struct{}`, nil},
		{"fallbackBackend embedding", `package fontglyph; type hidden struct{}; type fallbackBackend struct { primary, spec, environment, index, features, loaded, loadedOrder, load, covers, policy, closed, closeOnce int; hidden }`, []string{"primary", "spec", "environment", "index", "features", "loaded", "loadedOrder", "load", "covers", "policy", "closed", "closeOnce"}},
	}
	for _, fixture := range apiFixtures {
		file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", fixture.source, 0)
		if err != nil {
			failures = append(failures, fixture.name+": "+err.Error())
			continue
		}
		fields, alias, found := structFields(file, strings.Fields(fixture.name)[0])
		if validConcreteStruct(fields, alias, found, fixture.want) {
			failures = append(failures, fixture.name+" escaped exact concrete API guard")
		}
	}

	allowed := map[string]map[string]bool{
		"shape":         {"cervterm/internal/fontdesc": true, "cervterm/internal/fontglyph/internal/face": true, "cervterm/internal/unicodecluster": true, "cervterm/internal/unicodeprops": true},
		"cache":         {"cervterm/internal/fontglyph/internal/face": true},
		"discovery":     {"cervterm/internal/fontdesc": true},
		"internal/face": {"cervterm/internal/fontdesc": true},
	}
	importFixtures := []struct{ subsystem, source string }{
		{"shape", `package shape; import _ "cervterm/internal/fontglyph"`},
		{"cache", `package cache; import _ "cervterm/internal/fontglyph/shape"`},
		{"discovery", `package discovery; import _ "cervterm/internal/fontglyph/cache"`},
		{"internal/face", `package face; import _ "cervterm/internal/unicodecluster"`},
	}
	for index, fixture := range importFixtures {
		file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", fixture.source, parser.ImportsOnly)
		if err != nil || len(forbiddenLocalImports(fixture.subsystem, file, allowed)) == 0 {
			failures = append(failures, fmt.Sprintf("ADR-0021 import fixture %d was not rejected", index))
		}
	}

	baseCapture, _ := parseBench(filepath.Join(evidence, "benchmarks-base.txt"))
	baseHeaders := baseCapture.headers
	headerMutations := [][]string{
		append([]string(nil), baseHeaders[:len(baseHeaders)-1]...),
		append(append([]string(nil), baseHeaders...), baseHeaders[0]),
		append([]string(nil), baseHeaders...),
	}
	headerMutations[2][0], headerMutations[2][1] = headerMutations[2][1], headerMutations[2][0]
	for index, headers := range headerMutations {
		if len(validateSideHeaders(headers, "base")) == 0 {
			failures = append(failures, fmt.Sprintf("raw side-order fixture %d escaped", index))
		}
	}
	interleavedCapture, _ := parseBench(filepath.Join(evidence, "benchmarks-interleaved.txt"))
	interleavedHeaders := interleavedCapture.headers
	interleavedMutations := [][]string{
		append([]string(nil), interleavedHeaders[:len(interleavedHeaders)-1]...),
		append(append([]string(nil), interleavedHeaders...), interleavedHeaders[0]),
		append([]string(nil), interleavedHeaders...),
	}
	interleavedMutations[2][0], interleavedMutations[2][1] = interleavedMutations[2][1], interleavedMutations[2][0]
	for index, headers := range interleavedMutations {
		if len(validateInterleavedHeaders(headers)) == 0 {
			failures = append(failures, fmt.Sprintf("raw physical AB/BA fixture %d escaped", index))
		}
	}
	candidateCapture, _ := parseBench(filepath.Join(evidence, "benchmarks-candidate.txt"))
	interleavedData, interleavedErr := readLF(filepath.Join(evidence, "benchmarks-interleaved.txt"))
	if interleavedErr == nil {
		first := interleavedCapture.byKey[benchBlockKey("base", 1, "fontglyph")]
		second := interleavedCapture.byKey[benchBlockKey("base", 2, "fontglyph")]
		firstBody := strings.TrimPrefix(first.text, first.header+"\n")
		secondBody := strings.TrimPrefix(second.text, second.header+"\n")
		mutated := strings.Replace(string(interleavedData), first.text, first.header+"\n"+secondBody, 1)
		mutated = strings.Replace(mutated, second.text, second.header+"\n"+firstBody, 1)
		mutatedCapture, parseFailures := parseBenchData("row-swap-fixture", []byte(mutated))
		if len(parseFailures) != 0 || len(compareBenchCaptures(baseCapture, candidateCapture, mutatedCapture)) == 0 {
			failures = append(failures, "interleaved row swap with intact headers escaped exact projection guard")
		}
	}

	presentManifest, presentManifestFailures := presentTreeManifestEntries()
	if len(presentManifestFailures) == 0 {
		mutatedManifest := make(map[string]string, len(presentManifest))
		for path, hash := range presentManifest {
			mutatedManifest[path] = hash
		}
		mutatedManifest["internal/fontglyph/shape/policy.go"] = strings.Repeat("0", 64)
		if len(compareManifestEntries("synthetic-merge-source-mutation", mutatedManifest, presentManifest)) == 0 {
			failures = append(failures, "altered source with exact synthetic merge identities escaped present-tree manifest guard")
		}
	}

	pinnedManifest := map[string]string{"go.mod": strings.Repeat("1", 64), "internal/fontglyph/shape/policy.go": strings.Repeat("2", 64)}
	descendantManifest := map[string]string{"go.mod": pinnedManifest["go.mod"], "internal/fontglyph/shape/policy.go": pinnedManifest["internal/fontglyph/shape/policy.go"], "scripts/check-slice55c-evidence.go": strings.Repeat("3", 64)}
	if got := comparePinnedManifestEntries("descendant-addition", descendantManifest, pinnedManifest); len(got) != 0 {
		failures = append(failures, "legitimate descendant manifest addition rejected: "+strings.Join(got, "; "))
	}
	mutatedManifest := make(map[string]string, len(descendantManifest))
	for path, hash := range descendantManifest {
		mutatedManifest[path] = hash
	}
	mutatedManifest["internal/fontglyph/shape/policy.go"] = strings.Repeat("0", 64)
	if len(comparePinnedManifestEntries("manifest-entry-mutation", mutatedManifest, pinnedManifest)) == 0 {
		failures = append(failures, "historical manifest entry mutation escaped descendant guard")
	}
	delete(mutatedManifest, "go.mod")
	if len(comparePinnedManifestEntries("manifest-entry-deletion", mutatedManifest, pinnedManifest)) == 0 {
		failures = append(failures, "historical manifest entry deletion escaped descendant guard")
	}

	metadata, err := readLF(filepath.Join(evidence, "benchmark-binaries.txt"))
	if err == nil {
		mutated := strings.Replace(string(metadata), "samples=10", "samples=9", 1)
		if len(validateBenchmarkMetadata(mutated)) == 0 {
			failures = append(failures, "benchmark command/identity fixture escaped")
		}
	}

	actual := collectFunctionHashes()
	for key := range functionHashes {
		mutated := make(map[string]string, len(actual))
		for name, value := range actual {
			mutated[name] = value
		}
		mutated[key] = strings.Repeat("0", 64)
		if len(validateFunctionHashes(mutated, functionHashes)) == 0 {
			failures = append(failures, "exact function-body fixture escaped for "+key)
		}
		break
	}
	if isGCommitCandidate([]string{"stash-index", commitW, "index on branch"}) || isGCommitCandidate([]string{"old-g", commitT, gSubject}) || !isGCommitCandidate([]string{"g", commitW, gSubject}) {
		failures = append(failures, "G identity candidate fixture escaped")
	}

	cleanG := historyModeFacts{wExists: true, head: "g", cleanupHash: cleanupDiffHash, g: []commitFact{{hash: "g", parent: commitW, subject: gSubject, paths: sorted(stageGPaths)}}, gAncestor: true}
	syntheticSubject := "Merge " + commitW + " into " + baseCommit
	validModes := []historyModeFacts{
		{wExists: true, head: commitW, cleanupHash: cleanupDiffHash, dirty: sorted(stageGPaths)},
		cleanG,
		{wExists: true, head: "merge", cleanupHash: cleanupDiffHash, g: cleanG.g, gAncestor: true},
		{wExists: false, head: "detached-g", headSubject: gSubject},
		{wExists: false, head: "detached-merge", headSubject: syntheticSubject, expectedG: commitW, expectedBase: baseCommit},
	}
	for index, facts := range validModes {
		if got := validateHistoryMode(facts); len(got) != 0 {
			failures = append(failures, fmt.Sprintf("valid history mode fixture %d rejected: %v", index, got))
		}
	}
	invalidModes := []historyModeFacts{
		{wExists: true, head: commitW},
		{wExists: true, head: "g", g: []commitFact{{hash: "g", parent: commitT, subject: gSubject, paths: sorted(stageGPaths)}}, gAncestor: true},
		{wExists: true, head: "g", g: append(cleanG.g, cleanG.g...), gAncestor: true},
		{wExists: true, head: "merge", g: cleanG.g, gAncestor: false},
		{wExists: false, head: "detached-g", headSubject: "wrong"},
		{wExists: false, head: "detached-g", headSubject: gSubject, dirty: []string{"dirty"}},
		{wExists: false, head: "detached-merge", headSubject: "Merge " + commitT + " into " + baseCommit, expectedG: commitW, expectedBase: baseCommit},
		{wExists: false, head: "detached-merge", headSubject: "Merge " + commitW + " into " + commitT, expectedG: commitW, expectedBase: baseCommit},
		{wExists: false, head: "detached-merge", headSubject: syntheticSubject, expectedG: commitT, expectedBase: baseCommit},
		{wExists: false, head: "detached-merge", headSubject: syntheticSubject, expectedG: commitW, expectedBase: commitT},
		{wExists: false, head: "detached-merge", headSubject: syntheticSubject},
		{wExists: false, head: "detached-merge", headSubject: syntheticSubject, expectedG: commitW, expectedBase: baseCommit, identityError: "fixture event error"},
	}
	for index, facts := range invalidModes {
		if got := validateHistoryMode(facts); len(got) == 0 {
			failures = append(failures, fmt.Sprintf("invalid history mode fixture %d escaped", index))
		}
	}
	validEvent := []byte(fmt.Sprintf(`{"pull_request":{"head":{"sha":%q},"base":{"sha":%q}}}`, commitW, baseCommit))
	if head, base, err := parsePullRequestIdentities(validEvent); err != nil || head != commitW || base != baseCommit {
		failures = append(failures, fmt.Sprintf("valid pull_request event fixture rejected: %s/%s/%v", head, base, err))
	}
	for index, data := range [][]byte{
		[]byte(`{"pull_request":{"head":{"sha":"short"},"base":{"sha":"short"}}}`),
		[]byte(`{"pull_request":{}}`),
		[]byte(`not-json`),
	} {
		if _, _, err := parsePullRequestIdentities(data); err == nil {
			failures = append(failures, fmt.Sprintf("invalid pull_request event fixture %d escaped", index))
		}
	}
	return failures
}

func rootForbiddenAuthority() map[string]bool {
	return map[string]bool{
		"contentResolutionKey": true, "rootGlyphSequence": true,
		"legacyResolvePrimaryFacePlan": true, "legacyResolveDescriptorFacePlans": true, "legacyClassifySyntheticFallback": true,
		"legacyNewResolvedFacePlan": true, "legacyResolveEmbeddedFallbackPlan": true, "legacyResolveFaceCandidates": true,
		"legacySimpleShape": true, "legacyShapeOneRune": true, "legacyIsSimpleShapeableCluster": true, "legacyIsComplexShapingRune": true,
		"rememberResolution": true, "rememberLoadFailure": true, "loadFinalPlan": true, "tryPlans": true,
	}
}

func checkRootAntiRetention() []string {
	forbidden := rootForbiddenAuthority()
	entries, err := os.ReadDir("internal/fontglyph")
	if err != nil {
		return []string{err.Error()}
	}
	var failures []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		path := filepath.ToSlash(filepath.Join("internal/fontglyph", entry.Name()))
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			failures = append(failures, path+": "+parseErr.Error())
			continue
		}
		if retainsForbidden(file, forbidden) {
			failures = append(failures, path+" recursively retains obsolete root resolution/shaping authority")
		}
	}
	return failures
}

func checkRootShapeFacadeInventory() []string {
	expected := map[string]bool{
		"internal/fontglyph/backend.go:normalizeClusterToSingleRune":             true,
		"internal/fontglyph/fallback_resolver.go:type:fallbackBackend":           true,
		"internal/fontglyph/runshape.go:runSubstituted":                          true,
		"internal/fontglyph/shaper.go:centerShapedGlyphsInCells":                 true,
		"internal/fontglyph/shaper.go:isPortableShaper":                          true,
		"internal/fontglyph/shaper_default.go:newDefaultShaper":                  true,
		"internal/fontglyph/shaper_default_windows.go:newDefaultShaper":          true,
		"internal/fontglyph/simple_shaper.go:SimpleShaper.FeatureCapability":     true,
		"internal/fontglyph/simple_shaper.go:SimpleShaper.ShapeFeatures":         true,
		"internal/fontglyph/simple_shaper.go:SimpleShaper.Shape":                 true,
		"internal/fontglyph/simple_shaper.go:shapeOneRune":                       true,
		"internal/fontglyph/simple_shaper.go:isSimpleShapeableCluster":           true,
		"internal/fontglyph/simple_shaper.go:isComplexShapingRune":               true,
		"internal/fontglyph/shape_facade.go:shapeResolver":                       true,
		"internal/fontglyph/shape_facade.go:resolvedPlanFromShape":               true,
		"internal/fontglyph/shape_facade.go:resolvedPlanToShape":                 true,
		"internal/fontglyph/shape_facade.go:resolvedPlansFromShape":              true,
		"internal/fontglyph/shape_facade.go:classifySyntheticFallback":           true,
		"internal/fontglyph/shape_facade.go:newResolvedFacePlan":                 true,
		"internal/fontglyph/shape_facade.go:resolveEmbeddedFallbackPlan":         true,
		"internal/fontglyph/shape_facade.go:fallbackBackend.installShapePolicy":  true,
		"internal/fontglyph/shape_facade.go:fallbackBackend.loadShapePlan":       true,
		"internal/fontglyph/shape_facade.go:shapedGlyphsFromShape":               true,
		"internal/fontglyph/shape_facade.go:shapedGlyphsToShape":                 true,
		"internal/fontglyph/shape_facade.go:rootToShapeShaper.Shape":             true,
		"internal/fontglyph/shape_facade.go:rootToShapeShaper.ShapeFeatures":     true,
		"internal/fontglyph/shape_facade.go:type:shapeToRootShaper":              true,
		"internal/fontglyph/shape_facade.go:shapeToRootShaper.ShapeFeatures":     true,
		"internal/fontglyph/shape_facade.go:shapeToRootShaper.FeatureCapability": true,
		"internal/fontglyph/shape_facade.go:rootShaperFromShape":                 true,
	}
	actual := make(map[string]int)
	entries, err := os.ReadDir("internal/fontglyph")
	if err != nil {
		return []string{err.Error()}
	}
	var failures []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.ToSlash(filepath.Join("internal/fontglyph", entry.Name()))
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			failures = append(failures, path+": "+parseErr.Error())
			continue
		}
		for key, count := range rootShapeFacadeDeclarationKeys(path, file) {
			actual[key] += count
		}
	}
	for key := range expected {
		if actual[key] != 1 {
			failures = append(failures, fmt.Sprintf("canonical root shape facade declaration %s count=%d want=1", key, actual[key]))
		}
	}
	for key := range actual {
		if !expected[key] {
			failures = append(failures, "non-canonical root shape authority/adapter declaration "+key)
		}
	}
	return failures
}

func rootShapeFacadeDeclarationKeys(path string, file *ast.File) map[string]int {
	aliases := make(map[string]bool)
	for _, imported := range file.Imports {
		value, _ := strconv.Unquote(imported.Path.Value)
		if value != "cervterm/internal/fontglyph/shape" {
			continue
		}
		name := "shape"
		if imported.Name != nil {
			name = imported.Name.Name
		}
		aliases[name] = true
	}
	keys := make(map[string]int)
	if len(aliases) == 0 {
		return keys
	}
	containsShapeSelector := func(node ast.Node) bool {
		found := false
		ast.Inspect(node, func(inner ast.Node) bool {
			selector, ok := inner.(*ast.SelectorExpr)
			identifier, identOK := selectorXIdent(selector)
			if ok && identOK && aliases[identifier.Name] {
				found = true
				return false
			}
			return !found
		})
		return found
	}
	for _, declaration := range file.Decls {
		if !containsShapeSelector(declaration) {
			continue
		}
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			name := declaration.Name.Name
			if declaration.Recv != nil {
				name = recvName(declaration.Recv.List[0].Type) + "." + name
			}
			keys[path+":"+name]++
		case *ast.GenDecl:
			for _, raw := range declaration.Specs {
				switch spec := raw.(type) {
				case *ast.TypeSpec:
					if containsShapeSelector(spec) {
						keys[path+":type:"+spec.Name.Name]++
					}
				case *ast.ValueSpec:
					if !containsShapeSelector(spec) {
						continue
					}
					for _, name := range spec.Names {
						keys[path+":value:"+name.Name]++
					}
				}
			}
		}
	}
	return keys
}

func selectorXIdent(selector *ast.SelectorExpr) (*ast.Ident, bool) {
	if selector == nil {
		return nil, false
	}
	identifier, ok := selector.X.(*ast.Ident)
	return identifier, ok
}

func retainsForbidden(file *ast.File, forbidden map[string]bool) bool {
	retainedNames := make(map[string]bool, len(forbidden))
	for name := range forbidden {
		retainedNames[name] = true
	}
	for changed := true; changed; {
		changed = false
		for _, declaration := range file.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, raw := range generic.Specs {
				typeSpec, ok := raw.(*ast.TypeSpec)
				if !ok || retainedNames[typeSpec.Name.Name] {
					continue
				}
				retains := exprContainsForbidden(typeSpec.Type, retainedNames)
				if !retains && typeSpec.TypeParams != nil {
					retains = exprContainsForbidden(typeSpec.TypeParams, retainedNames)
				}
				if retains {
					retainedNames[typeSpec.Name.Name] = true
					changed = true
				}
			}
		}
	}
	retained := false
	ast.Inspect(file, func(node ast.Node) bool {
		if retained || node == nil {
			return false
		}
		if identifier, ok := node.(*ast.Ident); ok && retainedNames[identifier.Name] {
			retained = true
			return false
		}
		return true
	})
	return retained
}

func exprContainsForbidden(node ast.Node, forbidden map[string]bool) bool {
	if node == nil {
		return false
	}
	found := false
	ast.Inspect(node, func(inner ast.Node) bool {
		if found || inner == nil {
			return false
		}
		if identifier, ok := inner.(*ast.Ident); ok && forbidden[identifier.Name] {
			found = true
			return false
		}
		return true
	})
	return found
}

type commitFact struct {
	hash, parent, subject string
	paths                 []string
}

type historyModeFacts struct {
	wExists                        bool
	head, headSubject, cleanupHash string
	g                              []commitFact
	dirty                          []string
	gAncestor                      bool
	expectedG, expectedBase        string
	identityError                  string
}

var syntheticMergeSubject = regexp.MustCompile(`^Merge ([0-9a-f]{40}) into ([0-9a-f]{40})$`)
var fullCommitIdentity = regexp.MustCompile(`^[0-9a-f]{40}$`)

func parsePullRequestIdentities(data []byte) (string, string, error) {
	var event struct {
		PullRequest struct {
			Head struct {
				SHA string `json:"sha"`
			} `json:"head"`
			Base struct {
				SHA string `json:"sha"`
			} `json:"base"`
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return "", "", err
	}
	head, base := event.PullRequest.Head.SHA, event.PullRequest.Base.SHA
	if !fullCommitIdentity.MatchString(head) || !fullCommitIdentity.MatchString(base) {
		return "", "", fmt.Errorf("pull_request head/base identities are not exact 40-character lowercase SHA-1 values")
	}
	return head, base, nil
}

func pullRequestIdentitiesFromEnvironment() (string, string, error) {
	path := os.Getenv("GITHUB_EVENT_PATH")
	if path == "" {
		return "", "", fmt.Errorf("GITHUB_EVENT_PATH is required for a detached synthetic merge")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	return parsePullRequestIdentities(data)
}

func checkCommitsAndPaths() []string {
	head := git("rev-parse", "HEAD")
	facts := historyModeFacts{
		wExists: gitObjectExists(commitW), head: head, headSubject: git("show", "-s", "--format=%s", "HEAD"), dirty: dirtyPaths(),
	}
	if facts.wExists {
		facts.cleanupHash = gitDiffDigest(commitW, "")
	}
	if !facts.wExists {
		if inSuccessorMode(head) {
			if len(facts.dirty) != 0 {
				return []string{fmt.Sprintf("detached successor worktree dirty=%v", facts.dirty)}
			}
			return nil
		}
		if facts.headSubject != gSubject {
			var err error
			facts.expectedG, facts.expectedBase, err = pullRequestIdentitiesFromEnvironment()
			if err != nil {
				facts.identityError = err.Error()
			}
		}
		return validateHistoryMode(facts)
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
	if head != commitW {
		for _, line := range gitLines("log", "HEAD", "--format=%H%x09%P%x09%s") {
			parts := strings.SplitN(line, "\t", 3)
			if !isGCommitCandidate(parts) {
				continue
			}
			facts.g = append(facts.g, commitFact{hash: parts[0], parent: parts[1], subject: parts[2], paths: commitPaths(parts[0])})
		}
		if len(facts.g) == 1 {
			facts.gAncestor = exec.Command("git", "merge-base", "--is-ancestor", facts.g[0].hash, head).Run() == nil
		}
		if len(facts.g) == 1 && len(facts.dirty) == 0 {
			facts.cleanupHash = gitDiffDigest(commitW, facts.g[0].hash)
		}
	}
	if inSuccessorMode(head) {
		if len(facts.g) == 1 {
			facts.cleanupHash = gitDiffDigest(commitW, facts.g[0].hash)
		}
		failures = append(failures, validateSuccessorHistory(facts)...)
		return failures
	}
	failures = append(failures, validateHistoryMode(facts)...)
	return failures
}

func isGCommitCandidate(parts []string) bool {
	return len(parts) == 3 && parts[1] == commitW && parts[2] == gSubject
}

func validateHistoryMode(facts historyModeFacts) []string {
	var failures []string
	if !facts.wExists {
		if facts.headSubject != gSubject {
			match := syntheticMergeSubject.FindStringSubmatch(facts.headSubject)
			if match == nil {
				failures = append(failures, fmt.Sprintf("detached depth-1 HEAD subject=%q is neither G nor an exact synthetic merge identity", facts.headSubject))
			} else {
				if facts.identityError != "" {
					failures = append(failures, "detached synthetic merge event identity: "+facts.identityError)
				}
				if facts.expectedG == "" || facts.expectedBase == "" {
					failures = append(failures, "detached synthetic merge lacks expected pull_request G/base identities")
				}
				if match[1] != facts.expectedG {
					failures = append(failures, fmt.Sprintf("detached synthetic merge G=%s want=%s", match[1], facts.expectedG))
				}
				if match[2] != baseCommit {
					failures = append(failures, fmt.Sprintf("detached synthetic merge base=%s want immutable base=%s", match[2], baseCommit))
				}
				if facts.expectedBase != baseCommit {
					failures = append(failures, fmt.Sprintf("pull_request base=%s want immutable base=%s", facts.expectedBase, baseCommit))
				}
				if match[2] != facts.expectedBase {
					failures = append(failures, fmt.Sprintf("detached synthetic merge base=%s want pull_request base=%s", match[2], facts.expectedBase))
				}
			}
		}
		if len(facts.dirty) != 0 {
			failures = append(failures, fmt.Sprintf("detached depth-1 worktree dirty=%v", facts.dirty))
		}
		return failures
	}
	if facts.cleanupHash != cleanupDiffHash {
		failures = append(failures, fmt.Sprintf("G cleanup diff hash=%s want=%s", facts.cleanupHash, cleanupDiffHash))
	}
	if facts.head == commitW {
		if !equalStrings(facts.dirty, sorted(stageGPaths)) {
			failures = append(failures, fmt.Sprintf("dirty pre-G paths=%v want=%v", facts.dirty, sorted(stageGPaths)))
		}
		return failures
	}
	if len(facts.g) != 1 {
		failures = append(failures, fmt.Sprintf("G commit cardinality=%d want=1", len(facts.g)))
		return failures
	}
	g := facts.g[0]
	if g.parent != commitW || g.subject != gSubject {
		failures = append(failures, fmt.Sprintf("G identity parent/subject=%s/%q", g.parent, g.subject))
	}
	if len(facts.dirty) != 0 {
		combined := unionSorted(g.paths, facts.dirty)
		if !equalStrings(combined, sorted(stageGPaths)) {
			failures = append(failures, fmt.Sprintf("dirty G-amend path union=%v want=%v", combined, sorted(stageGPaths)))
		}
	} else if !equalStrings(g.paths, sorted(stageGPaths)) {
		failures = append(failures, fmt.Sprintf("G paths=%v want=%v", g.paths, sorted(stageGPaths)))
	}
	if facts.head != g.hash && !facts.gAncestor {
		failures = append(failures, fmt.Sprintf("G commit %s is not an ancestor of HEAD %s", g.hash, facts.head))
	}
	return failures
}

func validateSuccessorHistory(facts historyModeFacts) []string {
	var failures []string
	if len(facts.g) != 1 {
		return []string{fmt.Sprintf("historical G commit cardinality=%d want=1", len(facts.g))}
	}
	g := facts.g[0]
	if g.hash != commitG || g.parent != commitW || g.subject != gSubject {
		failures = append(failures, fmt.Sprintf("historical G identity=%s parent/subject=%s/%q", g.hash, g.parent, g.subject))
	}
	if !equalStrings(g.paths, sorted(stageGPaths)) {
		failures = append(failures, fmt.Sprintf("historical G paths=%v want=%v", g.paths, sorted(stageGPaths)))
	}
	if facts.cleanupHash != cleanupDiffHash {
		failures = append(failures, fmt.Sprintf("historical G cleanup diff hash=%s want=%s", facts.cleanupHash, cleanupDiffHash))
	}
	if !facts.gAncestor {
		failures = append(failures, fmt.Sprintf("historical G commit %s is not an ancestor of successor HEAD %s", g.hash, facts.head))
	}
	return failures
}

func inSuccessorMode(head string) bool {
	if _, err := os.Stat("scripts/check-slice55c-evidence.go"); err == nil {
		return true
	}
	if !gitObjectExists(successorW) {
		return false
	}
	return head == successorW || exec.Command("git", "merge-base", "--is-ancestor", successorW, head).Run() == nil
}

func gitDiffDigest(from, to string) string {
	args := []string{"diff", "--no-ext-diff", "--binary", from}
	if to != "" {
		args = append(args, to)
	}
	args = append(args, "--")
	args = append(args, cleanupPaths...)
	output, err := exec.Command("git", args...).Output()
	if err != nil {
		return ""
	}
	return digest(output)
}

func gitObjectExists(commit string) bool {
	command := exec.Command("git", "cat-file", "-e", commit+"^{commit}")
	return command.Run() == nil
}

func git(args ...string) string {
	output, _ := exec.Command("git", args...).Output()
	return strings.TrimSpace(string(output))
}

func gitLines(args ...string) []string {
	text := git(args...)
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func commitPaths(commit string) []string {
	return sorted(gitLines("diff-tree", "--no-commit-id", "--name-only", "-r", commit))
}

func dirtyPaths() []string {
	output, _ := exec.Command("git", "status", "--porcelain=v1", "--untracked-files=all").Output()
	text := strings.TrimSuffix(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n")
	var lines []string
	if text != "" {
		lines = strings.Split(text, "\n")
	}
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if arrow := strings.LastIndex(path, " -> "); arrow >= 0 {
			path = path[arrow+4:]
		}
		paths = append(paths, filepath.ToSlash(path))
	}
	return sorted(paths)
}

func sorted(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func unionSorted(groups ...[]string) []string {
	set := make(map[string]bool)
	for _, group := range groups {
		for _, value := range group {
			set[value] = true
		}
	}
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	return sorted(values)
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
