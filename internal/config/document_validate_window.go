package config

import (
	"math"
	"strconv"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

func validateInteger(source, path string, value lua.LValue) error {
	number, ok := value.(lua.LNumber)
	if !ok {
		return typeError(source, path, KindInteger, value)
	}
	parsed := float64(number)
	upper := math.Ldexp(1, strconv.IntSize-1)
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) || math.Trunc(parsed) != parsed || parsed < -upper || parsed >= upper {
		return documentError(source, path, "must be an integer in [%g, %g)", -upper, upper)
	}
	return nil
}

func isWindowSidePaddingPath(path string) bool {
	for _, candidate := range []string{"window.padding_left", "window.padding_right", "window.padding_top", "window.padding_bottom"} {
		if path == candidate || strings.HasSuffix(path, "."+candidate) {
			return true
		}
	}
	return false
}
