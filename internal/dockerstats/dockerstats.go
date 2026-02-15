package dockerstats

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Stats holds Docker resource usage for display.
type Stats struct {
	RAMUsedBytes  int64   // memory in use
	RAMLimitBytes int64   // memory limit (0 if unknown)
	CPUPercent    float64 // aggregate CPU %
	DiskUsedBytes int64   // disk used by Docker
	DiskLimitBytes int64  // disk limit (0 if unknown, e.g. Docker Desktop virtual disk)
	Error         string
}

// Fetch runs docker CLI commands and returns resource stats.
func Fetch(ctx context.Context) Stats {
	out := Stats{}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	// Memory limit from docker info (Total Memory or Memory for Desktop)
	infoOut, err := output(ctx, "docker", "info")
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out.RAMLimitBytes = parseMemFromInfo(infoOut)

	// Memory and CPU from docker stats --no-stream (aggregate across containers)
	statsOut, _ := output(ctx, "docker", "stats", "--no-stream", "--format", "{{.MemUsage}}\t{{.CPUPerc}}")
	out.RAMUsedBytes, out.CPUPercent = parseStats(statsOut)

	// Disk from docker system df
	dfOut, err := output(ctx, "docker", "system", "df")
	if err != nil {
		if out.Error != "" {
			out.Error += "; " + err.Error()
		} else {
			out.Error = err.Error()
		}
		return out
	}
	out.DiskUsedBytes, out.DiskLimitBytes = parseSystemDF(dfOut)

	// If we got no RAM limit from info but we have usage, try to infer from stats (sum of container limits or use 0)
	if out.RAMLimitBytes == 0 && out.RAMUsedBytes > 0 {
		// Leave limit 0; TUI will show "used" only or "N/A" for limit
	}
	return out
}

func output(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	b, err := cmd.Output()
	return strings.TrimSpace(string(b)), err
}

// parseMemFromInfo extracts memory limit from "Total Memory: 7.663GiB" or "Memory: 4GiB" (Docker Desktop).
var reMemInfo = regexp.MustCompile(`(?i)(?:Total\s+)?Memory:\s*([\d.]+)\s*([KMG]?i?B)`)

func parseMemFromInfo(s string) int64 {
	matches := reMemInfo.FindStringSubmatch(s)
	if len(matches) < 3 {
		return 0
	}
	return parseSize(matches[1], matches[2])
}

// parseStats parses "X.XXGiB / Y.YYGiB\tZ.ZZ%" lines and returns total used bytes and sum of CPU %.
var reMemUsage = regexp.MustCompile(`([\d.]+)\s*([KMG]?i?B)\s*/\s*[\d.]+\s*[KMG]?i?B`)
var reCPU = regexp.MustCompile(`([\d.]+)%`)

func parseStats(s string) (totalMem int64, totalCPU float64) {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) >= 1 {
			if m := reMemUsage.FindStringSubmatch(parts[0]); len(m) >= 3 {
				totalMem += parseSize(m[1], m[2])
			}
		}
		if len(parts) >= 2 {
			if m := reCPU.FindStringSubmatch(parts[1]); len(m) >= 2 {
				if p, err := strconv.ParseFloat(m[1], 64); err == nil {
					totalCPU += p
				}
			}
		}
	}
	return totalMem, totalCPU
}

// parseSystemDF parses "docker system df" output. Columns are TYPE, TOTAL, ACTIVE, SIZE, RECLAIMABLE.
// We sum the SIZE column (4th field) per line.
func parseSystemDF(s string) (used int64, limit int64) {
	reSize := regexp.MustCompile(`([\d.]+)\s*([KMG]?i?B)`)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if i == 0 && strings.HasPrefix(strings.TrimSpace(line), "TYPE") {
			continue
		}
		// Split on whitespace; SIZE is typically 4th column (index 3)
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		m := reSize.FindStringSubmatch(fields[3])
		if len(m) >= 3 {
			used += parseSize(m[1], m[2])
		}
	}
	return used, 0
}

func parseSize(numStr, unit string) int64 {
	n, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0
	}
	unit = strings.ToUpper(strings.TrimSpace(unit))
	var mult int64 = 1
	switch {
	case strings.HasPrefix(unit, "K"):
		mult = 1024
	case strings.HasPrefix(unit, "M"):
		mult = 1024 * 1024
	case strings.HasPrefix(unit, "G"):
		mult = 1024 * 1024 * 1024
	}
	return int64(n * float64(mult))
}

// FormatSize returns human-readable size (e.g. "1.2 GB").
func FormatSize(b int64) string {
	if b <= 0 {
		return "0 B"
	}
	const unit = 1024
	if b < unit {
		return strconv.FormatInt(b, 10) + " B"
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB"}
	if exp >= len(units) {
		exp = len(units) - 1
		div = 1
		for i := 0; i < exp; i++ {
			div *= unit
		}
	}
	return strconv.FormatFloat(float64(b)/float64(div), 'f', 2, 64) + " " + units[exp]
}
