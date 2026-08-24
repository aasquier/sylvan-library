package api

import (
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// diskUsage is `shutil.disk_usage`: statvfs arithmetic — total from the
// fragment size, used as total minus free-for-root, free as free-for-us.
// Frsize is already an int64 on this platform; the block counts are uint64
// and fit comfortably (nine exabytes of volume would overflow first).
func diskUsage(path string) (total, used, free int64) {
	var fs unix.Statfs_t
	if err := unix.Statfs(path, &fs); err != nil {
		return 0, 0, 0
	}
	frsize := fs.Frsize
	total = frsize * int64(fs.Blocks)     // #nosec G115 -- see above
	used = total - frsize*int64(fs.Bfree) // #nosec G115 -- see above
	free = frsize * int64(fs.Bavail)      // #nosec G115 -- see above
	return total, used, free
}

// processRSS reads `/proc/self/status` VmRSS — the *current* resident size,
// kind "current", exactly the branch `adminstats._rss` takes on the
// deployment. The fallback (peak, from getrusage) lives in the darwin file;
// Linux without /proc is not a machine this app meets.
func processRSS() (int64, string) {
	raw, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, "current"
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if rest, found := strings.CutPrefix(line, "VmRSS:"); found {
			fields := strings.Fields(rest)
			if len(fields) > 0 {
				kb, _ := strconv.ParseInt(fields[0], 10, 64)
				return kb * 1024, "current"
			}
		}
	}
	return 0, "current"
}

// machineMemory is `_machine_memory`: total physical memory, and — where the
// kernel says — what could be allocated before swapping. `MemAvailable` is
// Linux's own answer; there is no portable equivalent, so elsewhere it is
// absent rather than approximated.
func machineMemory() (total, available *int64) {
	raw, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return nil, nil
	}
	read := func(prefix string) *int64 {
		for _, line := range strings.Split(string(raw), "\n") {
			if rest, found := strings.CutPrefix(line, prefix); found {
				fields := strings.Fields(rest)
				if len(fields) > 0 {
					kb, err := strconv.ParseInt(fields[0], 10, 64)
					if err == nil {
						v := kb * 1024
						return &v
					}
				}
			}
		}
		return nil
	}
	return read("MemTotal:"), read("MemAvailable:")
}

// loadAverages reads `/proc/loadavg` — the kernel's own figures.
// An unreadable file is an empty list rather than an error.
func loadAverages() []float64 {
	raw, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return []float64{}
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 3 {
		return []float64{}
	}
	out := make([]float64, 0, 3)
	for _, f := range fields[:3] {
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return []float64{}
		}
		out = append(out, v)
	}
	return out
}
