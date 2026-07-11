package buildinfo

import (
	"fmt"
	"runtime"
)

var Version = "0.1.0-dev"

func Info() string {
	return fmt.Sprintf("CervTerm %s (%s/%s, %s)", Version, runtime.GOOS, runtime.GOARCH, runtime.Version())
}
