// Package sysinfo reads basic host stats from /proc and statfs (Linux only).
package sysinfo

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Stats struct {
	Hostname   string  `json:"hostname"`
	UptimeSec  float64 `json:"uptime_sec"`
	CPUPercent float64 `json:"cpu_percent"`
	CPUCores   int     `json:"cpu_cores"`
	Load1      float64 `json:"load1"`
	MemTotal   uint64  `json:"mem_total"`
	MemUsed    uint64  `json:"mem_used"`
	DiskTotal  uint64  `json:"disk_total"`
	DiskUsed   uint64  `json:"disk_used"`
	DiskPath   string  `json:"disk_path"`
}

// Collect samples CPU over a short interval and reads memory/disk for diskPath.
func Collect(diskPath string) Stats {
	s := Stats{CPUCores: runtime.NumCPU(), DiskPath: diskPath}
	s.Hostname, _ = os.Hostname()

	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		if f := strings.Fields(string(b)); len(f) > 0 {
			s.UptimeSec, _ = strconv.ParseFloat(f[0], 64)
		}
	}
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		if f := strings.Fields(string(b)); len(f) > 0 {
			s.Load1, _ = strconv.ParseFloat(f[0], 64)
		}
	}

	idle1, total1 := cpuTimes()
	time.Sleep(250 * time.Millisecond)
	idle2, total2 := cpuTimes()
	if dt := total2 - total1; dt > 0 {
		s.CPUPercent = 100 * float64(dt-(idle2-idle1)) / float64(dt)
	}

	mem := meminfo()
	s.MemTotal = mem["MemTotal"]
	s.MemUsed = mem["MemTotal"] - mem["MemAvailable"]

	var fs syscall.Statfs_t
	if err := syscall.Statfs(diskPath, &fs); err == nil {
		s.DiskTotal = fs.Blocks * uint64(fs.Bsize)
		s.DiskUsed = (fs.Blocks - fs.Bfree) * uint64(fs.Bsize)
	}
	return s
}

func cpuTimes() (idle, total uint64) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0, 0
	}
	fields := strings.Fields(sc.Text())
	for i, v := range fields[1:] {
		n, _ := strconv.ParseUint(v, 10, 64)
		total += n
		if i == 3 || i == 4 { // idle + iowait
			idle += n
		}
	}
	return idle, total
}

func meminfo() map[string]uint64 {
	out := map[string]uint64{}
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		fs := strings.Fields(v)
		if len(fs) == 0 {
			continue
		}
		n, _ := strconv.ParseUint(fs[0], 10, 64)
		out[k] = n * 1024 // values are in kB
	}
	return out
}
