package analyzer

import "strings"

// detectOtelCollector looks for a running OpenTelemetry Collector process.
func detectOtelCollector() (bool, string) {
	// First try exact process name matches for standard distributions.
	for _, bin := range []string{"otelcol", "otelcol-contrib"} {
		ok, pidStr := runCmd("pgrep", "-x", bin)
		if ok {
			configPath := otelConfigFromPID(strings.TrimSpace(pidStr))
			return true, configPath
		}
	}
	// Fall back to full command-line search to catch custom builds
	// like dynatrace-otel-collector, opentelemetry-collector, etc.
	for _, pattern := range []string{"otel-collector", "otelcol"} {
		ok, pidStr := runCmd("pgrep", "-f", pattern)
		if ok {
			// pgrep may return multiple PIDs; use the first one.
			pid := strings.TrimSpace(strings.SplitN(pidStr, "\n", 2)[0])
			configPath := otelConfigFromPID(pid)
			return true, configPath
		}
	}
	return false, ""
}

// otelConfigFromPID returns the --config= path from a process's command line.
func otelConfigFromPID(pid string) string {
	if pid == "" {
		return ""
	}
	ok, cmdline := runCmd("ps", "-p", pid, "-o", "args=")
	if !ok {
		return ""
	}
	return extractOtelConfigPath(cmdline)
}

// extractOtelConfigPath parses an otelcol cmdline to find --config=<path>.
func extractOtelConfigPath(cmdline string) string {
	for _, part := range strings.Fields(cmdline) {
		if strings.HasPrefix(part, "--config=") {
			return strings.TrimPrefix(part, "--config=")
		}
	}
	return ""
}
