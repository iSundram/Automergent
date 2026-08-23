package render

import (
	"os"
	"runtime/pprof"
	"testing"
)

func TestProfileRasterize(t *testing.T) {
	os.MkdirAll("/tmp/opencode", 0o755)
	f, err := os.Create("/tmp/opencode/cpu.prof")
	if err != nil {
		t.Skip(err)
	}
	pprof.StartCPUProfile(f)
	defer pprof.StopCPUProfile()
	rasterize(1024)
}
