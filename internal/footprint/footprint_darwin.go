//go:build darwin

package footprint

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/ebitengine/purego"
)

const rusageInfoV2Flavor = 2

// rusageInfoV2 mirrors Apple's named struct rusage_info_v2 in sys/resource.h.
// Keep the declaration field-for-field; numeric byte offsets are intentionally
// not used. Apple currently defines this structure as 160 bytes.
type rusageInfoV2 struct {
	UUID                [16]byte
	UserTime            uint64
	SystemTime          uint64
	PkgIdleWkups        uint64
	InterruptWkups      uint64
	Pageins             uint64
	WiredSize           uint64
	ResidentSize        uint64
	PhysFootprint       uint64
	ProcStartAbstime    uint64
	ProcExitAbstime     uint64
	ChildUserTime       uint64
	ChildSystemTime     uint64
	ChildPkgIdleWkups   uint64
	ChildInterruptWkups uint64
	ChildPageins        uint64
	ChildElapsedAbstime uint64
	DiskIOBytesRead     uint64
	DiskIOBytesWritten  uint64
}

var (
	loadOnce      sync.Once
	loadErr       error
	procPidRusage func(int32, int32, unsafe.Pointer) int32
	errnoLocation func() *int32
)

func load() error {
	loadOnce.Do(func() {
		lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			loadErr = fmt.Errorf("load libSystem: %w", err)
			return
		}
		// libSystem remains loaded for process lifetime so the function pointer is valid.
		purego.RegisterLibFunc(&procPidRusage, lib, "proc_pid_rusage")
		purego.RegisterLibFunc(&errnoLocation, lib, "__error")
	})
	return loadErr
}

func Physical(pid int32) (uint64, error) {
	if err := load(); err != nil {
		return 0, err
	}
	var usage rusageInfoV2
	// errno is thread-local. Pin the call and read to the same OS thread, and
	// resolve its address before the native call so symbol lookup cannot replace it.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	errno := errnoLocation()
	if rc := procPidRusage(pid, rusageInfoV2Flavor, unsafe.Pointer(&usage)); rc != 0 {
		return 0, fmt.Errorf("proc_pid_rusage pid %d: %w", pid, syscall.Errno(*errno))
	}
	return usage.PhysFootprint, nil
}

func StructSize() uintptr { return unsafe.Sizeof(rusageInfoV2{}) }
