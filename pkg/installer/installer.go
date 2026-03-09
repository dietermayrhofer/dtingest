// Package installer provides shared utilities for Dynatrace ingestion installers.
package installer

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

// AuthHeader returns the correct Authorization header value for a given token.
// API tokens (starting with "dt0c01.") use "Api-Token" scheme; all others use "Bearer".
func AuthHeader(token string) string {
	if strings.HasPrefix(token, "dt0c01.") {
		return "Api-Token " + token
	}
	return "Bearer " + token
}

// ClassicAPIURL converts a Dynatrace Platform URL to the equivalent Classic API
// base URL used by the Classic API and the OneAgent installer endpoint.
//
// Mapping rules:
//   - *.apps.dynatrace.com      → *.live.dynatrace.com      (production SaaS)
//   - *.apps.dynatracelabs.com  → *.dynatracelabs.com       (dev/sprint envs)
func ClassicAPIURL(environmentURL string) string {
	s := environmentURL
	s = strings.Replace(s, ".apps.dynatrace.com", ".live.dynatrace.com", 1)
	s = strings.Replace(s, ".apps.dynatracelabs.com", ".dynatracelabs.com", 1)
	return s
}

// APIURL converts any Dynatrace environment URL to the Classic API base URL.
//
// Mapping rules:
//   - *.apps.dynatrace.com      → *.live.dynatrace.com      (production SaaS)
//   - *.apps.dynatracelabs.com  → *.dynatracelabs.com       (dev/sprint envs — drop .apps only)
func APIURL(environmentURL string) string {
	s := environmentURL
	s = strings.Replace(s, ".apps.dynatrace.com", ".live.dynatrace.com", 1)
	s = strings.Replace(s, ".apps.dynatracelabs.com", ".dynatracelabs.com", 1)
	return strings.TrimRight(s, "/")
}

// ExtractTenantID extracts the tenant/environment ID (first DNS label) from a
// Dynatrace environment URL.
// e.g. "https://abc12345.live.dynatrace.com" → "abc12345"
func ExtractTenantID(environmentURL string) string {
	u, err := url.Parse(environmentURL)
	if err != nil || u.Host == "" {
		// Fallback: take everything before the first dot.
		s := strings.TrimPrefix(environmentURL, "https://")
		s = strings.TrimPrefix(s, "http://")
		if idx := strings.Index(s, "."); idx > 0 {
			return s[:idx]
		}
		return s
	}
	host := u.Hostname()
	if idx := strings.Index(host, "."); idx > 0 {
		return host[:idx]
	}
	return host
}

// RunCommand runs a named executable with the provided arguments, streaming its
// stdout and stderr to the current process's stdout/stderr.  The working
// directory is inherited from the current process.
func RunCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command %q failed: %w", name, err)
	}
	return nil
}

// RunCommandQuiet runs a named executable suppressing stdout. Stderr is still
// captured and included in the returned error when the command fails, so error
// details are never silently swallowed.
func RunCommandQuiet(name string, args ...string) error {
	var stderr strings.Builder
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil // discard
	cmd.Stderr = &stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("command %q failed: %w\n%s", name, err, msg)
		}
		return fmt.Errorf("command %q failed: %w", name, err)
	}
	return nil
}
