package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/iSundram/Automergent/internal/config"
)

var configProfile string

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect and validate resolved configuration",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show effective config values, sources, and validation status",
	RunE: func(cmd *cobra.Command, args []string) error {
		info, err := loadConfigDiagnostics()
		if err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), "Precedence: defaults < global < project < profile < env < session < cli")
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "KEY\tEFFECTIVE\tSOURCE\tVALIDATION")

		keys := sortedSchemaKeys(info.schema)
		for _, key := range keys {
			value := info.effective[key]
			source := formatSource(info.loader, key)
			if source == "" {
				source = "unknown"
			}
			validation := "ok"
			if msg, hasError := info.validationByField[key]; hasError {
				validation = "error: " + msg
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", key, formatValue(value), source, validation)
		}
		return w.Flush()
	},
}

var configSourcesCmd = &cobra.Command{
	Use:   "sources",
	Short: "Show config value provenance by layer",
	RunE: func(cmd *cobra.Command, args []string) error {
		info, err := loadConfigDiagnostics()
		if err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), "Precedence: defaults < global < project < profile < env < session < cli")
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "KEY\tDEFAULTS\tGLOBAL\tPROJECT\tPROFILE\tENV\tSESSION\tCLI\tEFFECTIVE_SOURCE")

		keys := sortedSchemaKeys(info.schema)
		layers := info.loader.Layers()
		for _, key := range keys {
			fmt.Fprintf(
				w,
				"%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				key,
				layerValue(layers, config.LayerDefaults, key),
				layerValue(layers, config.LayerGlobal, key),
				layerValue(layers, config.LayerProject, key),
				layerValue(layers, config.LayerProfile, key),
				layerValue(layers, config.LayerEnv, key),
				layerValue(layers, config.LayerSession, key),
				layerValue(layers, config.LayerCLI, key),
				formatSource(info.loader, key),
			)
		}
		return w.Flush()
	},
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the resolved configuration and report failures",
	RunE: func(cmd *cobra.Command, args []string) error {
		info, err := loadConfigDiagnostics()
		if err != nil {
			return err
		}

		if len(info.validationErrors) == 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "Validation: PASS (%d fields checked)\n", len(sortedSchemaKeys(info.schema)))
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Validation: FAIL (%d error(s))\n", len(info.validationErrors))
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "FIELD\tVALUE\tSOURCE\tERROR")
		for _, validationErr := range info.validationErrors {
			fmt.Fprintf(
				w,
				"%s\t%s\t%s\t%s\n",
				validationErr.Field,
				formatValue(validationErr.Value),
				formatSource(info.loader, validationErr.Field),
				validationErr.Message,
			)
		}
		_ = w.Flush()
		return fmt.Errorf("configuration validation failed")
	},
}

func init() {
	configCmd.PersistentFlags().StringVar(&configProfile, "profile", "", "profile name to load for diagnostics")
	configCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "global config file path override")
	configCmd.AddCommand(configShowCmd, configSourcesCmd, configValidateCmd)
	rootCmd.AddCommand(configCmd)
}

type diagnosticsInfo struct {
	loader            *config.Loader
	schema            *config.Schema
	effective         map[string]any
	validationErrors  []config.ValidationError
	validationByField map[string]string
}

func loadConfigDiagnostics() (*diagnosticsInfo, error) {
	loader, err := config.NewLoader(&config.LoaderOptions{
		GlobalPath: cfgFile,
		Profile:    configProfile,
	})
	if err != nil {
		return nil, fmt.Errorf("create config loader: %w", err)
	}

	cfg, err := loader.Load()
	if err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}

	schema := config.DefaultSchema()
	validationErrors := schema.Validate(cfg)
	validationByField := make(map[string]string, len(validationErrors))
	for _, validationErr := range validationErrors {
		if _, exists := validationByField[validationErr.Field]; !exists {
			validationByField[validationErr.Field] = validationErr.Message
		}
	}

	effective := flattenConfig(cfg)
	return &diagnosticsInfo{
		loader:            loader,
		schema:            schema,
		effective:         effective,
		validationErrors:  validationErrors,
		validationByField: validationByField,
	}, nil
}

func flattenConfig(cfg *config.Config) map[string]any {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return map[string]any{}
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return map[string]any{}
	}
	return flattenAnyMap(data)
}

func flattenAnyMap(data map[string]any) map[string]any {
	out := make(map[string]any)
	var walk func(prefix string, value any)
	walk = func(prefix string, value any) {
		switch v := value.(type) {
		case map[string]any:
			for k, child := range v {
				k = lowerFirst(k) // Config has no json tags: "Provider" -> "provider"
				next := k
				if prefix != "" {
					next = prefix + "." + k
				}
				walk(next, child)
			}
		default:
			if prefix != "" {
				out[prefix] = v
			}
		}
	}
	for k, v := range data {
		walk(lowerFirst(k), v)
	}
	return out
}

func sortedSchemaKeys(schema *config.Schema) []string {
	keys := make([]string, 0, len(schema.Fields))
	for key := range schema.Fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func formatSource(loader *config.Loader, key string) string {
	src, ok := loader.GetSource(key)
	if !ok {
		return ""
	}

	if src.File != "" {
		return fmt.Sprintf("%s(%s)", src.Layer.String(), src.File)
	}
	if src.Key != "" && src.Key != key {
		return fmt.Sprintf("%s(%s)", src.Layer.String(), src.Key)
	}
	return src.Layer.String()
}

func layerValue(layers map[config.Layer]map[string]any, layer config.Layer, key string) string {
	layerMap, ok := layers[layer]
	if !ok {
		return "-"
	}
	value, ok := layerMap[key]
	if !ok {
		return "-"
	}
	return formatValue(value)
}

func formatValue(v any) string {
	if v == nil {
		return "<nil>"
	}
	switch value := v.(type) {
	case string:
		if value == "" {
			return "\"\""
		}
		return value
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return strings.TrimSpace(string(encoded))
	}
}

// lowerFirst lowercases the first rune of s (Go field name -> config key).
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if r[0] >= 'A' && r[0] <= 'Z' {
		r[0] = r[0] + ('a' - 'A')
	}
	return string(r)
}
