//go:build !darwin

package footprint

import (
	"fmt"
	"runtime"
)

func Physical(_ int32) (uint64, error) {
	return 0, fmt.Errorf("physical footprint is unavailable on %s", runtime.GOOS)
}

func StructSize() uintptr { return 0 }
