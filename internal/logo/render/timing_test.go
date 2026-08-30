package render

import (
	"os"
	"runtime/pprof"
	"testing"
)

func TestProfileRasterize(t *testing.T) {
	os.MkdirAll("/tmp/automergent", 0o755)
	f, err := os.Create("/tmp/automergent/cpu.prof")
	if err != nil {
		t.Skip(err)
	}
	pprof.StartCPUProfile(f)
	defer pprof.StopCPUProfile()
	rasterize(1024)
}
