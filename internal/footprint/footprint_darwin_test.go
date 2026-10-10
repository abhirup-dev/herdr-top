//go:build darwin

package footprint

import (
	"os"
	"testing"
	"unsafe"
)

func TestRusageInfoV2LayoutAndCurrentProcess(t *testing.T) {
	if got := unsafe.Sizeof(rusageInfoV2{}); got != 160 {
		t.Fatalf("rusage_info_v2 size=%d, want 160", got)
	}
	if got := unsafe.Offsetof(rusageInfoV2{}.PhysFootprint); got != 72 {
		t.Fatalf("PhysFootprint offset=%d, want 72", got)
	}
	v, err := Physical(int32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	if v == 0 {
		t.Fatal("current process returned zero physical footprint")
	}
}
