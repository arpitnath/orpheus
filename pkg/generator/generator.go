// Package generator creates auto-generated entry point files for agents.
package generator

import (
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"agentscale/pkg/config"
)

const entrypointFileName = "_entrypoint.py"

// Generator creates entry point files for agents
type Generator struct{}

// New creates a new Generator instance
func New() *Generator {
	return &Generator{}
}

// TemplateData holds data for template rendering
type TemplateData struct {
	Module     string
	Entrypoint string
	InputType  string
}

// Generate creates the _entrypoint.py file in the agent directory
func (g *Generator) Generate(cfg *config.AgentConfig, async bool) (string, error) {
	// Prepare template data
	data := TemplateData{
		Module:     stripPyExtension(cfg.Module),
		Entrypoint: cfg.Entrypoint,
		InputType:  cfg.InputType,
	}

	// Select template based on async flag
	templateStr := PythonSyncTemplate
	if async {
		templateStr = PythonAsyncTemplate
	}

	// Parse template
	tmpl, err := template.New("entrypoint").Parse(templateStr)
	if err != nil {
		return "", err
	}

	// Build output path
	outputPath := filepath.Join(cfg.AgentDir, entrypointFileName)

	// Create output file
	file, err := os.Create(outputPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Execute template
	if err := tmpl.Execute(file, data); err != nil {
		return "", err
	}

	// Make executable
	if err := os.Chmod(outputPath, 0755); err != nil {
		return "", err
	}

	return outputPath, nil
}

// GenerateString returns the generated entry point as a string (for testing)
func (g *Generator) GenerateString(cfg *config.AgentConfig, async bool) (string, error) {
	data := TemplateData{
		Module:     stripPyExtension(cfg.Module),
		Entrypoint: cfg.Entrypoint,
		InputType:  cfg.InputType,
	}

	templateStr := PythonSyncTemplate
	if async {
		templateStr = PythonAsyncTemplate
	}

	tmpl, err := template.New("entrypoint").Parse(templateStr)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// Cleanup removes the generated entry point file
func (g *Generator) Cleanup(entrypointPath string) error {
	if entrypointPath == "" {
		return nil
	}
	// Only remove if it's the generated file
	if filepath.Base(entrypointPath) != entrypointFileName {
		return nil
	}
	return os.Remove(entrypointPath)
}

// GetEntrypointPath returns the path where the entrypoint would be generated
func GetEntrypointPath(agentDir string) string {
	return filepath.Join(agentDir, entrypointFileName)
}

// stripPyExtension removes .py extension if present
func stripPyExtension(module string) string {
	return strings.TrimSuffix(module, ".py")
}
