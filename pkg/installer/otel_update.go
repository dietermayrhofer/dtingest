package installer

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// exporterSnippet is the YAML block to inject into an existing OTel Collector
// configuration as the `otlphttp/dynatrace` exporter.
const exporterSnippetTemplate = `otlphttp/dynatrace:
  endpoint: %s/api/v2/otlp
  headers:
    Authorization: "Api-Token %s"
`

// pipelineHint is the human-readable instruction for wiring the exporter.
const pipelineHint = `Add "otlphttp/dynatrace" to the exporters list of each pipeline you want
to forward to Dynatrace, for example:

  service:
    pipelines:
      traces:
        exporters: [otlphttp/dynatrace]
      metrics:
        exporters: [otlphttp/dynatrace]
      logs:
        exporters: [otlphttp/dynatrace]
`

// UpdateResult holds the outcome of an OTel config update operation.
type UpdateResult struct {
	ConfigPath  string
	BackupPath  string
	Modified    bool
	Description string
}

// GenerateExporterSnippet returns the YAML snippet for the Dynatrace OTLP
// exporter, ready to paste into an existing OTel Collector config.
func GenerateExporterSnippet(apiURL, token string) string {
	return fmt.Sprintf(exporterSnippetTemplate,
		strings.TrimRight(apiURL, "/"),
		token,
	)
}

// GeneratePipelineHint returns instructions for wiring the DT exporter into
// service pipelines.
func GeneratePipelineHint() string {
	return pipelineHint
}

// GenerateFullInstructions returns a human-readable guide for manually adding
// the Dynatrace exporter to an existing OTel Collector configuration.
func GenerateFullInstructions(apiURL, token string) string {
	var sb strings.Builder
	sb.WriteString("Add the following to the `exporters:` section of your OTel Collector config:\n\n")
	sb.WriteString(GenerateExporterSnippet(apiURL, token))
	sb.WriteString("\n")
	sb.WriteString(GeneratePipelineHint())
	return sb.String()
}

// mergeDynatraceExporter deep-merges the Dynatrace exporter definition into
// the `exporters` key of the provided config map.  It also appends
// `otlphttp/dynatrace` to the exporters list of every existing pipeline.
func mergeDynatraceExporter(cfg map[string]interface{}, apiURL, token string) {
	// Ensure exporters key exists.
	exporters, ok := cfg["exporters"].(map[string]interface{})
	if !ok {
		exporters = make(map[string]interface{})
		cfg["exporters"] = exporters
	}

	exporters["otlphttp/dynatrace"] = map[string]interface{}{
		"endpoint": strings.TrimRight(apiURL, "/") + "/api/v2/otlp",
		"headers": map[string]interface{}{
			"Authorization": "Api-Token " + token,
		},
	}

	// Append to existing pipeline exporters.
	service, ok := cfg["service"].(map[string]interface{})
	if !ok {
		return
	}
	pipelines, ok := service["pipelines"].(map[string]interface{})
	if !ok {
		return
	}
	for pipelineName, pipelineVal := range pipelines {
		pipeline, ok := pipelineVal.(map[string]interface{})
		if !ok {
			continue
		}
		existing, _ := pipeline["exporters"].([]interface{})
		// Don't add duplicates.
		alreadyPresent := false
		for _, e := range existing {
			if e == "otlphttp/dynatrace" {
				alreadyPresent = true
				break
			}
		}
		if !alreadyPresent {
			pipeline["exporters"] = append(existing, "otlphttp/dynatrace")
			pipelines[pipelineName] = pipeline
		}
	}
}

// PatchConfigFile reads an existing OTel Collector YAML config file, backs it
// up, injects the Dynatrace exporter, and writes the updated config back.
func PatchConfigFile(configPath, apiURL, token string) (*UpdateResult, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", configPath, err)
	}

	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing YAML config %s: %w", configPath, err)
	}
	if cfg == nil {
		cfg = make(map[string]interface{})
	}

	// Create a timestamped backup.
	backupPath := fmt.Sprintf("%s.bak.%d", configPath, time.Now().Unix())
	if err := os.WriteFile(backupPath, data, 0o600); err != nil {
		return nil, fmt.Errorf("creating backup at %s: %w", backupPath, err)
	}

	mergeDynatraceExporter(cfg, apiURL, token)

	updated, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("serialising updated config: %w", err)
	}

	if err := os.WriteFile(configPath, updated, 0o600); err != nil {
		return nil, fmt.Errorf("writing updated config to %s: %w", configPath, err)
	}

	return &UpdateResult{
		ConfigPath:  configPath,
		BackupPath:  backupPath,
		Modified:    true,
		Description: "Dynatrace otlphttp/dynatrace exporter merged into existing config",
	}, nil
}

// UpdateOtelConfig updates an existing OTel Collector config file with the
// Dynatrace exporter.  If dryRun is true it prints instructions instead.
func UpdateOtelConfig(configPath, envURL, token string, dryRun bool) error {
	apiURL := APIURL(envURL)

	if dryRun {
		fmt.Println("[dry-run] Would patch OTel Collector config with Dynatrace exporter")
		fmt.Printf("  Config file: %s\n", configPath)
		fmt.Printf("  API URL:     %s\n", apiURL)
		fmt.Println()
		fmt.Println("  Exporter snippet that would be merged:")
		fmt.Println(GenerateExporterSnippet(apiURL, token))
		return nil
	}

	result, err := PatchConfigFile(configPath, apiURL, token)
	if err != nil {
		return err
	}

	fmt.Printf("  Config updated: %s\n", result.ConfigPath)
	fmt.Printf("  Backup created: %s\n", result.BackupPath)
	fmt.Println()
	fmt.Println("  Restart your OTel Collector to apply the new config.")
	fmt.Println()
	fmt.Println(GeneratePipelineHint())
	return nil
}
