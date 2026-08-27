// Package workspace provides SARIF export for diagnostics.
package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iSundram/Automergent/internal/diagnostics/types"
)

// SARIFReport represents a SARIF 2.1.0 report.
type SARIFReport struct {
	Schema  string    `json:"$schema"`
	Version string    `json:"version"`
	Runs    []SARIFRun `json:"runs"`
}

// SARIFRun represents a single tool run.
type SARIFRun struct {
	Tool     SARIFTool     `json:"tool"`
	Results  []SARIFResult `json:"results"`
	Invocations []SARIFInvocation `json:"invocations,omitempty"`
}

// SARIFTool represents the tool that produced the results.
type SARIFTool struct {
	Driver SARIFDriver `json:"driver"`
}

// SARIFDriver represents the tool driver.
type SARIFDriver struct {
	Name            string            `json:"name"`
	Version         string            `json:"version,omitempty"`
	InformationURI  string            `json:"informationUri,omitempty"`
	Rules           []SARIFRule       `json:"rules"`
}

// SARIFRule represents a rule that produced results.
type SARIFRule struct {
	ID          string          `json:"id"`
	Name        string          `json:"name,omitempty"`
	ShortDescription SARIFMessage `json:"shortDescription,omitempty"`
	FullDescription  SARIFMessage  `json:"fullDescription,omitempty"`
	Help        SARIFMessage    `json:"help,omitempty"`
	DefaultConfiguration SARIFDefaultConfig `json:"defaultConfiguration,omitempty"`
}

// SARIFMessage represents a message in SARIF.
type SARIFMessage struct {
	Text string `json:"text"`
}

// SARIFDefaultConfig represents default configuration for a rule.
type SARIFDefaultConfig struct {
	Level string `json:"level"` // "error", "warning", "note", "none"
}

// SARIFInvocation represents a tool invocation.
type SARIFInvocation struct {
	ExecutionSuccessful bool   `json:"executionSuccessful"`
	StartTime           string `json:"startTimeUtc"`
	EndTime             string `json:"endTimeUtc"`
}

// SARIFResult represents a single result (diagnostic).
type SARIFResult struct {
	RuleID    string         `json:"ruleId"`
	RuleIndex int            `json:"ruleIndex,omitempty"`
	Level     string         `json:"level"` // "error", "warning", "note", "none"
	Message   SARIFMessage   `json:"message"`
	Locations []SARIFLocation `json:"locations"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	PartialFingerprints map[string]string `json:"partialFingerprints,omitempty"`
}

// SARIFLocation represents a location in a file.
type SARIFLocation struct {
	PhysicalLocation SARIFPhysicalLocation `json:"physicalLocation"`
}

// SARIFPhysicalLocation represents a physical location.
type SARIFPhysicalLocation struct {
	ArtifactLocation SARIFArtifactLocation `json:"artifactLocation"`
	Region           SARIFRegion           `json:"region,omitempty"`
}

// SARIFArtifactLocation represents a file location.
type SARIFArtifactLocation struct {
	URI          string `json:"uri"`
	URIBaseID    string `json:"uriBaseId,omitempty"`
	Index        int    `json:"index,omitempty"`
}

// SARIFRegion represents a region in a file.
type SARIFRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn,omitempty"`
	EndLine     int `json:"endLine,omitempty"`
	EndColumn   int `json:"endColumn,omitempty"`
}

// ExportSARIF exports workspace diagnostics to SARIF format.
func ExportSARIF(rootDir string, diagnostics map[string][]types.Diagnostic, toolName, toolVersion string) (*SARIFReport, error) {
	report := &SARIFReport{
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Version: "2.1.0",
		Runs: []SARIFRun{{
			Tool: SARIFTool{
				Driver: SARIFDriver{
					Name:           toolName,
					Version:        toolVersion,
					InformationURI: "https://github.com/iSundram/Automergent",
				},
			},
		}},
	}

	run := &report.Runs[0]
	ruleMap := make(map[string]int) // ruleID -> index
	rules := []SARIFRule{}

	for filePath, diags := range diagnostics {
		// Make path relative to root
		relPath, err := filepath.Rel(rootDir, filePath)
		if err != nil {
			relPath = filePath
		}
		relPath = filepath.ToSlash(relPath)

		for _, diag := range diags {
			// Get or create rule
			ruleID := diag.Code
			if ruleID == "" {
				ruleID = fmt.Sprintf("%s-%s", diag.Source, diag.Severity)
			}

			ruleIdx, ok := ruleMap[ruleID]
			if !ok {
				ruleIdx = len(rules)
				ruleMap[ruleID] = ruleIdx
				level := mapSeverityToSARIF(diag.Severity)
				rules = append(rules, SARIFRule{
					ID:   ruleID,
					Name: fmt.Sprintf("%s: %s", diag.Source, ruleID),
					ShortDescription: SARIFMessage{Text: diag.Message},
					FullDescription: SARIFMessage{Text: fmt.Sprintf("%s (%s)", diag.Message, diag.Source)},
					DefaultConfiguration: SARIFDefaultConfig{Level: level},
				})
			}

			// Create result
			result := SARIFResult{
				RuleID:    ruleID,
				RuleIndex: ruleIdx,
				Level:     mapSeverityToSARIF(diag.Severity),
				Message:   SARIFMessage{Text: diag.Message},
				Locations: []SARIFLocation{{
					PhysicalLocation: SARIFPhysicalLocation{
						ArtifactLocation: SARIFArtifactLocation{
							URI: relPath,
						},
						Region: SARIFRegion{
							StartLine:   diag.Line,
							StartColumn: diag.Column + 1, // SARIF is 1-indexed
							EndLine:     diag.EndLine,
							EndColumn:   diag.EndColumn + 1,
						},
					},
				}},
				Properties: map[string]interface{}{
					"tags":          diag.Tags,
					"suggestions":   diag.Suggestions,
					"related_files": diag.RelatedFiles,
				},
				PartialFingerprints: map[string]string{
					"diagnosticKey": diag.Key(),
				},
			}

			// Handle zero end line/column
			if result.Locations[0].PhysicalLocation.Region.EndLine == 0 {
				result.Locations[0].PhysicalLocation.Region.EndLine = diag.Line
			}
			if result.Locations[0].PhysicalLocation.Region.EndColumn == 0 {
				result.Locations[0].PhysicalLocation.Region.EndColumn = diag.Column + 1
			}

			run.Results = append(run.Results, result)
		}
	}

	run.Tool.Driver.Rules = rules

	// Add invocation info
	run.Invocations = []SARIFInvocation{{
		ExecutionSuccessful: true,
		StartTime:           time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
		EndTime:             time.Now().Format(time.RFC3339),
	}}

	return report, nil
}

// WriteSARIF writes a SARIF report to a file.
func WriteSARIF(report *SARIFReport, outputPath string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal SARIF: %w", err)
	}
	return os.WriteFile(outputPath, data, 0644)
}

// ExportSARIFToFile exports diagnostics to a SARIF file.
func ExportSARIFToFile(rootDir string, diagnostics map[string][]types.Diagnostic, outputPath, toolName, toolVersion string) error {
	report, err := ExportSARIF(rootDir, diagnostics, toolName, toolVersion)
	if err != nil {
		return err
	}
	return WriteSARIF(report, outputPath)
}

// mapSeverityToSARIF maps diagnostic severity to SARIF level.
func mapSeverityToSARIF(severity string) string {
	switch strings.ToLower(severity) {
	case "error":
		return "error"
	case "warning":
		return "warning"
	case "info":
		return "note"
	case "hint":
		return "note"
	default:
		return "warning"
	}
}

// SARIFSummary provides a summary of SARIF results.
type SARIFSummary struct {
	TotalResults int            `json:"total_results"`
	ByLevel      map[string]int `json:"by_level"`
	ByRule       map[string]int `json:"by_rule"`
	ByFile       map[string]int `json:"by_file"`
}

// SummarizeSARIF creates a summary of a SARIF report.
func SummarizeSARIF(report *SARIFReport) SARIFSummary {
	summary := SARIFSummary{
		ByLevel: make(map[string]int),
		ByRule:  make(map[string]int),
		ByFile:  make(map[string]int),
	}

	for _, run := range report.Runs {
		for _, result := range run.Results {
			summary.TotalResults++
			summary.ByLevel[result.Level]++
			summary.ByRule[result.RuleID]++

			for _, loc := range result.Locations {
				uri := loc.PhysicalLocation.ArtifactLocation.URI
				summary.ByFile[uri]++
			}
		}
	}

	return summary
}

// GitHubAnnotations exports diagnostics as GitHub Actions annotations format.
func GitHubAnnotations(rootDir string, diagnostics map[string][]types.Diagnostic) string {
	var lines []string

	for filePath, diags := range diagnostics {
		relPath, _ := filepath.Rel(rootDir, filePath)
		relPath = filepath.ToSlash(relPath)

		for _, diag := range diags {
			level := "warning"
			if diag.Severity == "error" {
				level = "error"
			} else if diag.Severity == "hint" || diag.Severity == "info" {
				level = "notice"
			}

			// GitHub annotation format: ::level file=path,line=1,col=1::message
			annotation := fmt.Sprintf("::%s file=%s,line=%d,col=%d::%s",
				level, relPath, diag.Line, diag.Column+1, diag.Message)

			lines = append(lines, annotation)
		}
	}

	return strings.Join(lines, "\n")
}

// WriteGitHubAnnotations writes GitHub Actions annotations to a file.
func WriteGitHubAnnotations(rootDir string, diagnostics map[string][]types.Diagnostic, outputPath string) error {
	annotations := GitHubAnnotations(rootDir, diagnostics)
	return os.WriteFile(outputPath, []byte(annotations), 0644)
}