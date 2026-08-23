package api

import (
	"encoding/binary"
	"syscall"

	"golang.org/x/sys/unix"
)

// processRSS on the dev Mac: no /proc, so the peak from getrusage — in
// bytes here, where Linux reports kilobytes; both branches of
// `adminstats._rss` say which, and `kind` rides along so the page can label
// a peak as a peak.
func processRSS() (int64, string) {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0, "peak"
	}
	return ru.Maxrss, "peak"
}

// machineMemory: total from the kernel, no MemAvailable — absent rather
// than approximated, as Python leaves it off any box without /proc.
func machineMemory() (total, available *int64) {
	size, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return nil, nil
	}
	v := int64(size) // #nosec G115 -- physical memory fits in nine exabytes
	return &v, nil
}

// diskUsage is `shutil.disk_usage`'s statvfs arithmetic; darwin's statfs
// carries no fragment size, so the block size stands in, as Python's
// statvfs shim on this platform effectively does.
func diskUsage(path string) (total, used, free int64) {
	var fs unix.Statfs_t
	if err := unix.Statfs(path, &fs); err != nil {
		return 0, 0, 0
	}
	bsize := int64(fs.Bsize)
	total = bsize * int64(fs.Blocks)     // #nosec G115 -- block counts fit int64
	used = total - bsize*int64(fs.Bfree) // #nosec G115 -- as above
	free = bsize * int64(fs.Bavail)      // #nosec G115 -- as above
	return total, used, free
}

// loadAverages via `vm.loadavg`: three fixed-point longs and their scale.
func loadAverages() []float64 {
	raw, err := unix.SysctlRaw("vm.loadavg")
	if err != nil || len(raw) < 24 {
		return []float64{}
	}
	// struct loadavg { fixpt_t ldavg[3]; long fscale; } — three 32-bit
	// fixpoints, four bytes of padding, then the scale as a 64-bit long:
	// twenty-four bytes on this architecture, measured when a 32-byte guard
	// silently emptied the deployed golden's `load` list.
	scale := float64(binary.LittleEndian.Uint64(raw[len(raw)-8:]))
	if scale == 0 {
		return []float64{}
	}
	out := make([]float64, 0, 3)
	for i := 0; i < 3; i++ {
		fix := binary.LittleEndian.Uint32(raw[i*4 : i*4+4])
		out = append(out, float64(fix)/scale)
	}
	return out
}
