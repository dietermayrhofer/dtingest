package installer

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"
	"time"
)

//go:embed otel.tmpl
var otelConfigTemplateText string

// otelConfigData holds the values substituted into otel.tmpl.
type otelConfigData struct {
	Endpoint   string
	AuthHeader string
}

// otelCollectorBinaryName returns the expected binary name inside the release archive.
func otelCollectorBinaryName() string {
	if runtime.GOOS == "windows" {
		return "dynatrace-otel-collector.exe"
	}
	return "dynatrace-otel-collector"
}

// otelLatestReleaseVersion queries the GitHub API and returns the latest release
// tag (e.g. "v0.44.0") for the Dynatrace OTel Collector.
func otelLatestReleaseVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/repos/Dynatrace/dynatrace-otel-collector/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub releases API returned status %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("parsing release JSON: %w", err)
	}
	if release.TagName == "" {
		return "", fmt.Errorf("release tag_name is empty")
	}
	return release.TagName, nil
}

// otelPlatformAssetName returns the versioned GitHub release asset filename for
// the current OS/architecture combination.
// Asset naming: dynatrace-otel-collector_{version}_{OS}_{arch}[.tar.gz|.zip]
// e.g. dynatrace-otel-collector_0.44.0_Darwin_arm64.tar.gz
func otelPlatformAssetName(version string) (string, error) {
	// Strip leading 'v' from tag (v0.44.0 → 0.44.0).
	ver := strings.TrimPrefix(version, "v")

	var osName, archName string
	switch runtime.GOOS {
	case "linux":
		osName = "Linux"
	case "darwin":
		osName = "Darwin"
	case "windows":
		osName = "Windows"
	default:
		return "", fmt.Errorf("unsupported OS for OTel Collector: %s", runtime.GOOS)
	}

	switch runtime.GOARCH {
	case "amd64":
		archName = "x86_64"
	case "arm64":
		archName = "arm64"
	default:
		return "", fmt.Errorf("unsupported architecture for OTel Collector: %s", runtime.GOARCH)
	}

	if runtime.GOOS == "windows" {
		return fmt.Sprintf("dynatrace-otel-collector_%s_%s_%s.zip", ver, osName, archName), nil
	}
	return fmt.Sprintf("dynatrace-otel-collector_%s_%s_%s.tar.gz", ver, osName, archName), nil
}

// otelReleaseURL returns the download URL for a specific versioned release asset.
func otelReleaseURL(version, assetName string) string {
	return fmt.Sprintf(
		"https://github.com/Dynatrace/dynatrace-otel-collector/releases/download/%s/%s",
		version, assetName,
	)
}

// downloadOtelCollector downloads and extracts the OTel Collector binary to
// the specified destination path.
func downloadOtelCollector(destDir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fmt.Printf("  Resolving latest Dynatrace OTel Collector release...\n")
	version, err := otelLatestReleaseVersion(ctx)
	if err != nil {
		return "", fmt.Errorf("resolving latest release version: %w", err)
	}

	assetName, err := otelPlatformAssetName(version)
	if err != nil {
		return "", err
	}

	downloadURL := otelReleaseURL(version, assetName)
	fmt.Printf("  Downloading Dynatrace OTel Collector %s from GitHub...\n", version)
	fmt.Printf("  URL: %s\n", downloadURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("building download request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading OTel Collector: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OTel Collector download returned status %d", resp.StatusCode)
	}

	// Save archive to temp file.
	tmpArchive, err := os.CreateTemp("", "dt-otel-collector-*")
	if err != nil {
		return "", fmt.Errorf("creating temp archive file: %w", err)
	}
	tmpArchiveName := tmpArchive.Name()
	defer os.Remove(tmpArchiveName)

	if _, err := io.Copy(tmpArchive, resp.Body); err != nil {
		tmpArchive.Close()
		return "", fmt.Errorf("writing archive to disk: %w", err)
	}
	tmpArchive.Close()

	// Extract binary from archive.
	binaryName := otelCollectorBinaryName()
	destPath := filepath.Join(destDir, binaryName)

	if strings.HasSuffix(assetName, ".zip") {
		if err := extractFromZip(tmpArchiveName, binaryName, destPath); err != nil {
			return "", fmt.Errorf("extracting from zip: %w", err)
		}
	} else {
		if err := extractFromTarGz(tmpArchiveName, binaryName, destPath); err != nil {
			return "", fmt.Errorf("extracting from tar.gz: %w", err)
		}
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(destPath, 0o755); err != nil {
			return "", fmt.Errorf("setting OTel Collector executable bit: %w", err)
		}
	}

	// On macOS, unsigned binaries downloaded from the internet are silently
	// killed by the system before they can produce any output.  Strip all
	// extended attributes (incl. quarantine) and apply an ad-hoc signature so
	// the OS allows the binary to run.
	if runtime.GOOS == "darwin" {
		if err := macOSPrepBinary(destPath); err != nil {
			return "", err
		}
	}

	return destPath, nil
}

// macOSPrepBinary removes quarantine/extended attributes and applies an ad-hoc
// code signature so macOS allows the binary to execute.
func macOSPrepBinary(binaryPath string) error {
	fmt.Println("  Preparing binary for macOS (removing quarantine, applying ad-hoc signature)...")

	if out, err := exec.Command("xattr", "-cr", binaryPath).CombinedOutput(); err != nil {
		return fmt.Errorf("xattr -cr failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}

	if _, err := exec.LookPath("codesign"); err == nil {
		if out, err := exec.Command("codesign", "--force", "--deep", "--sign", "-", binaryPath).CombinedOutput(); err != nil {
			// Non-fatal: log the warning but continue — the binary may still work.
			fmt.Printf("  Warning: ad-hoc codesign failed (may still work): %v\n%s\n",
				err, strings.TrimSpace(string(out)))
		} else {
			fmt.Println("  Ad-hoc signature applied.")
		}
	}
	return nil
}

// extractFromTarGz extracts a single file by name from a .tar.gz archive.
func extractFromTarGz(archivePath, targetName, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("reading gzip stream: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}
		if filepath.Base(hdr.Name) == targetName {
			out, err := os.Create(destPath)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			return out.Close()
		}
	}
	return fmt.Errorf("binary %q not found in archive", targetName)
}

// extractFromZip extracts a single file by name from a .zip archive.
func extractFromZip(archivePath, targetName, destPath string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("opening zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if filepath.Base(f.Name) == targetName {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			out, err := os.Create(destPath)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, rc); err != nil {
				out.Close()
				return err
			}
			return out.Close()
		}
	}
	return fmt.Errorf("binary %q not found in zip archive", targetName)
}

// sendOtelVerificationLog sends a single OTLP log record to the local collector
// (HTTP on 4318) with the given body text and returns the unique install ID
// embedded in the message so the caller can search for it.
func sendOtelVerificationLog(body string) error {
	hostname, _ := os.Hostname()

	payload := map[string]interface{}{
		"resourceLogs": []map[string]interface{}{
			{
				"resource": map[string]interface{}{
					"attributes": []map[string]interface{}{
						{"key": "service.name", "value": map[string]string{"stringValue": "dtingest"}},
						{"key": "host.name", "value": map[string]string{"stringValue": hostname}},
						{"key": "os.type", "value": map[string]string{"stringValue": runtime.GOOS}},
						{"key": "host.arch", "value": map[string]string{"stringValue": runtime.GOARCH}},
					},
				},
				"scopeLogs": []map[string]interface{}{
					{
						"scope": map[string]string{"name": "dtingest.installer"},
						"logRecords": []map[string]interface{}{
							{
								"timeUnixNano": fmt.Sprintf("%d", time.Now().UnixNano()),
								"severityText":   "INFO",
								"severityNumber": 9,
								"body":            map[string]string{"stringValue": body},
								"attributes": []map[string]interface{}{
									{"key": "dtingest.version", "value": map[string]string{"stringValue": "1.0"}},
								},
							},
						},
					},
				},
			},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling OTLP payload: %w", err)
	}

	resp, err := http.Post("http://127.0.0.1:4318/v1/logs", "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("sending OTLP log: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OTLP endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// waitForLogInDynatrace shells out to `dtctl query` until a log record
// containing searchTerm appears in Dynatrace, or until the timeout elapses.
// Authentication is handled by dtctl using the active context — no token
// needs to be passed here.
func waitForLogInDynatrace(searchTerm string, timeout time.Duration) error {
	dqlQuery := fmt.Sprintf(
		`fetch logs, from: now()-10m | filter contains(content, "%s") | limit 1`,
		searchTerm,
	)

	deadline := time.Now().Add(timeout)
	for {
		out, err := exec.Command("dtctl", "query", "--plain", dqlQuery).Output()
		if err == nil && strings.Contains(string(out), searchTerm) {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for log to appear in Dynatrace")
		}
		fmt.Print(".")
		time.Sleep(5 * time.Second)
	}
}

// buildOtelLogsUIURL constructs the Dynatrace Logs UI deep-link pre-filtered
// to show records containing searchTerm.
func buildOtelLogsUIURL(envURL, searchTerm string) string {
	base := strings.TrimRight(envURL, "/")
	fragmentJSON := fmt.Sprintf(
		`{"version":2,"dt.timeframe":{"from":"now()-30m","to":"now()"},"tableConfig":{"columns":["timestamp","status","Log message"],"columnAttributes":{"tableLineWrap":true}},"analysisMode":"logs","showDqlEditor":false,"filterFieldQuery":"content = *%s*"}`,
		searchTerm,
	)
	encoded := strings.ReplaceAll(url.QueryEscape(fragmentJSON), "+", "%20")
	return base + "/ui/apps/dynatrace.logs/#" + encoded
}

// waitForOtelCollectorReady polls TCP port 4318 until the collector accepts
// connections or the timeout elapses. crashed is closed when the process dies
// early so the probe can abort immediately.
func waitForOtelCollectorReady(timeout time.Duration, crashed <-chan error) error {
	deadline := time.Now().Add(timeout)
	for {
		// Try IPv4 loopback explicitly — avoids macOS resolving localhost→[::1]
		// while the collector only binds 0.0.0.0 (IPv4).
		conn, err := net.DialTimeout("tcp", "127.0.0.1:4318", time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		select {
		case crashErr := <-crashed:
			if crashErr != nil {
				return fmt.Errorf("collector process exited unexpectedly: %w", crashErr)
			}
			return fmt.Errorf("collector process exited unexpectedly")
		default:
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("collector did not open port 4318 within %s: %w", timeout, err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// verifyOtelInstall sends a verification log through the running collector,
// waits for it to arrive in Dynatrace, then prints the UI deep-link.
// Log search is performed via `dtctl query` using the active dtctl context.
func verifyOtelInstall(envURL string, crashed <-chan error) error {
	hostname, _ := os.Hostname()
	// Unique search token: hostname + unix seconds — short and searchable.
	uniqueID := fmt.Sprintf("dtingest-%s-%d", strings.ReplaceAll(hostname, ".", "-"), time.Now().Unix())

	body := fmt.Sprintf(
		"OpenTelemetry Collector Successfully installed with dtingest [host: %s, os: %s/%s, id: %s]",
		hostname, runtime.GOOS, runtime.GOARCH, uniqueID,
	)

	fmt.Println()
	fmt.Printf("  Waiting for collector to be ready...")
	if err := waitForOtelCollectorReady(30*time.Second, crashed); err != nil {
		return fmt.Errorf("collector not ready: %w", err)
	}
	fmt.Println(" ✓")

	fmt.Printf("  Sending verification log to collector...\n")
	if err := sendOtelVerificationLog(body); err != nil {
		return fmt.Errorf("sending verification log: %w", err)
	}
	fmt.Printf("  Log sent. Waiting for it to appear in Dynatrace")

	if err := waitForLogInDynatrace(uniqueID, 2*time.Minute); err != nil {
		return err
	}

	fmt.Println(" ✓")
	fmt.Println()
	fmt.Println("  View the logline:")
	fmt.Println(" ", buildOtelLogsUIURL(envURL, uniqueID))
	return nil
}

// generateOtelConfig renders otel.tmpl and returns a collector configuration YAML string.
func generateOtelConfig(apiURL, token string) (string, error) {
	tmpl, err := template.New("otel").Parse(otelConfigTemplateText)
	if err != nil {
		return "", fmt.Errorf("parsing otel template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, otelConfigData{
		Endpoint:   strings.TrimRight(apiURL, "/"),
		AuthHeader: AuthHeader(token),
	}); err != nil {
		return "", fmt.Errorf("rendering otel template: %w", err)
	}
	return buf.String(), nil
}

// findRunningOtelCollectors returns the PIDs of all running dynatrace-otel-collector
// processes (there may be more than one if a previous kill was incomplete).
func findRunningOtelCollectors() []int {
	if runtime.GOOS == "windows" {
		out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq dynatrace-otel-collector.exe", "/FO", "CSV", "/NH").Output()
		if err != nil {
			return nil
		}
		var pids []int
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if !strings.Contains(line, "dynatrace-otel-collector.exe") {
				continue
			}
			parts := strings.Split(line, ",")
			if len(parts) < 2 {
				continue
			}
			pidStr := strings.Trim(parts[1], "\"")
			pid, err := strconv.Atoi(pidStr)
			if err == nil {
				pids = append(pids, pid)
			}
		}
		return pids
	}
	// Unix: use -f to match the full command line, catching processes started
	// via an absolute path or through a wrapper (e.g. go run).
	out, err := exec.Command("pgrep", "-f", "dynatrace-otel-collector").Output()
	if err != nil {
		return nil
	}
	var pids []int
	for _, s := range strings.Fields(strings.TrimSpace(string(out))) {
		pid, err := strconv.Atoi(s)
		if err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}

// startOtelCollector starts the collector as a background process.
// It waits briefly to detect immediate startup failures; if the process is
// still running after the check it is detached (the parent does not Wait on it).
// The returned channel receives the exit error (or nil) if the process later dies.
func startOtelCollector(binaryPath, configPath string) (<-chan error, error) {
	cmd := exec.Command(binaryPath, "--config", configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting OTel Collector: %w", err)
	}

	pid := cmd.Process.Pid
	fmt.Printf("  Dynatrace OTel Collector started (PID %d).\n", pid)
	fmt.Printf("  Config: %s\n", configPath)

	// Monitor the process; send its exit status on the channel.
	crashed := make(chan error, 1)
	go func() {
		crashed <- cmd.Wait()
	}()

	// Give it a moment to fail fast on obvious misconfigurations.
	select {
	case err := <-crashed:
		if err != nil {
			return nil, fmt.Errorf("OTel Collector exited immediately: %w", err)
		}
		fmt.Println("  Collector exited.")
		close(crashed)
		return crashed, nil
	case <-time.After(3 * time.Second):
		fmt.Printf("  Collector is running in the background (PID %d). Detaching...\n", pid)
		_ = cmd.Process.Release()
	}

	return crashed, nil
}

// InstallOtelCollector downloads, configures, and starts the Dynatrace OTel
// Collector, pointing it at the given environment.
//
// Parameters:
//   - envURL:       Dynatrace environment URL
//   - token:        token used for Dynatrace API calls (e.g. log search)
//   - ingestToken:  token written into the collector config for OTLP export;
//                  should be a classic API token with logs.ingest / metrics.ingest /
//                  traces.ingest scopes.  Pass empty string to fall back to token.
//   - dryRun:       when true, only print what would be done
func InstallOtelCollector(envURL, token, ingestToken string, dryRun bool) error {
	apiURL := APIURL(envURL)

	// Resolve which token to write into the collector config.
	collectorToken := ingestToken
	if collectorToken == "" {
		collectorToken = token
		// Warn when the fallback token is OAuth — it may lack ingest scopes.
		if !strings.HasPrefix(collectorToken, "dt0c01.") {
			fmt.Println()
			fmt.Println("  ⚠  No --access-token provided. The OAuth token from dtctl will be used")
			fmt.Println("     for the collector config, but it may lack the required ingest scopes.")
			fmt.Println("     For reliable OTLP export, create an API token with:")
			fmt.Println("       logs.ingest  metrics.ingest  traces.ingest")
			fmt.Println("     and pass it via --access-token.")
			fmt.Println()
		}
	}

	if dryRun {
		assetName, _ := otelPlatformAssetName("latest")
		fmt.Println("[dry-run] Would install Dynatrace OpenTelemetry Collector")
		fmt.Printf("  API URL:      %s\n", apiURL)
		fmt.Printf("  Asset:        %s\n", assetName)
		fmt.Printf("  Ingest token: %s\n", func() string {
			if ingestToken != "" {
				return "(from --access-token)"
			}
			return "(from dtctl context — may lack ingest scopes)"
		}())
		fmt.Println("  Steps:")
		fmt.Println("    1. Download collector binary from GitHub releases")
		fmt.Println("    2. Generate collector config YAML with DT OTLP exporter")
		fmt.Println("    3. Start collector process")
		return nil
	}

	// Determine install directory — create ./opentelemetry in the cwd.
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	installDir := filepath.Join(cwd, "opentelemetry")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return fmt.Errorf("creating install directory: %w", err)
	}

	fmt.Printf("  Installing to: %s\n", installDir)

	// 1. Download binary.
	binaryPath, err := downloadOtelCollector(installDir)
	if err != nil {
		return err
	}

	// 2. Generate config.
	configContent, err := generateOtelConfig(apiURL, collectorToken)
	if err != nil {
		return fmt.Errorf("generating OTel Collector config: %w", err)
	}
	configPath := filepath.Join(installDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0o600); err != nil {
		return fmt.Errorf("writing OTel Collector config: %w", err)
	}
	fmt.Printf("  Config written to: %s\n", configPath)

	// 3. Check for already-running collectors and offer to replace them.
	if pids := findRunningOtelCollectors(); len(pids) > 0 {
		fmt.Printf("\n  Dynatrace OTel Collector already running (PIDs: %v).\n", pids)
		fmt.Print("  Kill them and start the new one? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			return fmt.Errorf("aborted: collector already running (PIDs: %v)", pids)
		}
		for _, pid := range pids {
			proc, err := os.FindProcess(pid)
			if err != nil {
				fmt.Printf("  Warning: could not find process %d: %v\n", pid, err)
				continue
			}
			if err := proc.Kill(); err != nil {
				fmt.Printf("  Warning: could not kill process %d: %v\n", pid, err)
				continue
			}
			fmt.Printf("  Stopped collector (PID %d).\n", pid)
		}
	}

	// 4. Start collector.
	crashed, err := startOtelCollector(binaryPath, configPath)
	if err != nil {
		return err
	}

	fmt.Println("  OpenTelemetry Collector installed and running.")

	// 5. Send a verification log and wait for it to arrive in Dynatrace.
	if err := verifyOtelInstall(envURL, crashed); err != nil {
		fmt.Printf("\n  Warning: log verification failed: %v\n", err)
		fmt.Println("  The collector may still be working — check the Dynatrace UI.")
	}
	return nil
}
