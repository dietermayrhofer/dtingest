package installer

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// otelCollectorBinaryName returns the expected binary name inside the release archive.
func otelCollectorBinaryName() string {
	if runtime.GOOS == "windows" {
		return "dynatrace-otel-collector.exe"
	}
	return "dynatrace-otel-collector"
}

// otelPlatformAssetName returns the GitHub release asset filename for the
// current OS/architecture combination.
func otelPlatformAssetName() (string, error) {
	var osName, archName string
	switch runtime.GOOS {
	case "linux":
		osName = "linux"
	case "darwin":
		osName = "darwin"
	case "windows":
		osName = "windows"
	default:
		return "", fmt.Errorf("unsupported OS for OTel Collector: %s", runtime.GOOS)
	}

	switch runtime.GOARCH {
	case "amd64":
		archName = "amd64"
	case "arm64":
		archName = "arm64"
	default:
		return "", fmt.Errorf("unsupported architecture for OTel Collector: %s", runtime.GOARCH)
	}

	if runtime.GOOS == "windows" {
		return fmt.Sprintf("dynatrace-otel-collector_%s_%s.zip", osName, archName), nil
	}
	return fmt.Sprintf("dynatrace-otel-collector_%s_%s.tar.gz", osName, archName), nil
}

// otelLatestReleaseURL returns the download URL for the latest Dynatrace OTel
// Collector release asset.
func otelLatestReleaseURL(assetName string) string {
	return fmt.Sprintf(
		"https://github.com/Dynatrace/dynatrace-otel-collector/releases/latest/download/%s",
		assetName,
	)
}

// downloadOtelCollector downloads and extracts the OTel Collector binary to
// the specified destination path.
func downloadOtelCollector(destDir string) (string, error) {
	assetName, err := otelPlatformAssetName()
	if err != nil {
		return "", err
	}

	downloadURL := otelLatestReleaseURL(assetName)
	fmt.Printf("  Downloading Dynatrace OTel Collector from GitHub...\n")
	fmt.Printf("  URL: %s\n", downloadURL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

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

	return destPath, nil
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

// otelConfigTemplate is the YAML configuration template for the Dynatrace
// OTel Collector.  %s placeholders are: endpoint, token.
const otelConfigTemplate = `receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:
    timeout: 5s
    send_batch_size: 1000

exporters:
  otlphttp/dynatrace:
    endpoint: %s/api/v2/otlp
    headers:
      Authorization: "Api-Token %s"

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [otlphttp/dynatrace]
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [otlphttp/dynatrace]
    logs:
      receivers: [otlp]
      processors: [batch]
      exporters: [otlphttp/dynatrace]
`

// generateOtelConfig returns a collector configuration YAML string.
func generateOtelConfig(apiURL, token string) string {
	return fmt.Sprintf(otelConfigTemplate, strings.TrimRight(apiURL, "/"), token)
}

// startOtelCollector starts the collector as a background process.
// It waits briefly to detect immediate startup failures; if the process is
// still running after the check it is detached (the parent does not Wait on it).
func startOtelCollector(binaryPath, configPath string) error {
	cmd := exec.Command(binaryPath, "--config", configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting OTel Collector: %w", err)
	}

	pid := cmd.Process.Pid
	fmt.Printf("  Dynatrace OTel Collector started (PID %d).\n", pid)
	fmt.Printf("  Config: %s\n", configPath)

	// Give the process a moment to fail fast if misconfigured.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("OTel Collector exited immediately: %w", err)
		}
		// Process exited cleanly (unlikely for a long-running collector).
		fmt.Println("  Collector exited.")
	case <-time.After(3 * time.Second):
		// Still running — release the process so it continues after this
		// process exits.  The goroutine above is intentionally leaked; the
		// child will become an orphan adopted by init/PID 1.
		if err := cmd.Process.Release(); err != nil {
			return fmt.Errorf("releasing collector process: %w", err)
		}
		fmt.Printf("  Collector is running in the background (PID %d). Detaching...\n", pid)
	}

	return nil
}

// InstallOtelCollector downloads, configures, and starts the Dynatrace OTel
// Collector, pointing it at the given environment.
//
// Parameters:
//   - envURL: Dynatrace environment URL
//   - token:  API token (Ingest scope)
//   - dryRun: when true, only print what would be done
func InstallOtelCollector(envURL, token string, dryRun bool) error {
	apiURL := APIURL(envURL)

	if dryRun {
		assetName, _ := otelPlatformAssetName()
		fmt.Println("[dry-run] Would install Dynatrace OpenTelemetry Collector")
		fmt.Printf("  API URL:      %s\n", apiURL)
		fmt.Printf("  Asset:        %s\n", assetName)
		fmt.Println("  Steps:")
		fmt.Println("    1. Download collector binary from GitHub releases")
		fmt.Println("    2. Generate collector config YAML with DT OTLP exporter")
		fmt.Println("    3. Start collector process")
		return nil
	}

	// Determine install directory.
	installDir, err := os.MkdirTemp("", "dynatrace-otel-collector-*")
	if err != nil {
		return fmt.Errorf("creating install directory: %w", err)
	}

	fmt.Printf("  Installing to: %s\n", installDir)

	// 1. Download binary.
	binaryPath, err := downloadOtelCollector(installDir)
	if err != nil {
		os.RemoveAll(installDir)
		return err
	}

	// 2. Generate config.
	configContent := generateOtelConfig(apiURL, token)
	configPath := filepath.Join(installDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0o600); err != nil {
		os.RemoveAll(installDir)
		return fmt.Errorf("writing OTel Collector config: %w", err)
	}
	fmt.Printf("  Config written to: %s\n", configPath)

	// 3. Start collector.
	if err := startOtelCollector(binaryPath, configPath); err != nil {
		return err
	}

	fmt.Println("  OpenTelemetry Collector installed and running.")
	return nil
}
