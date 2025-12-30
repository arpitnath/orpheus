// Package generator creates auto-generated entry point files for agents.
package generator

import (
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"agentscale/pkg/config"
)

const (
	entrypointFileNamePython = "_entrypoint.py"
	entrypointFileNameNodeJS = "_entrypoint.mjs" // .mjs for ESM modules
)

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

// Generate creates the entrypoint file in the agent directory.
// For Python: _entrypoint.py
// For Node.js: _entrypoint.mjs
func (g *Generator) Generate(cfg *config.AgentConfig, async bool) (string, error) {
	// Determine runtime and select appropriate template/filename
	var templateStr string
	var entrypointFileName string
	var moduleName string

	switch cfg.Runtime {
	case config.RuntimeNodeJS20:
		templateStr = NodeJSTemplate
		entrypointFileName = entrypointFileNameNodeJS
		moduleName = stripJSExtension(cfg.Module)
	default: // Python (default)
		templateStr = PythonSyncTemplate
		if async {
			templateStr = PythonAsyncTemplate
		}
		entrypointFileName = entrypointFileNamePython
		moduleName = stripPyExtension(cfg.Module)
	}

	// Prepare template data
	data := TemplateData{
		Module:     moduleName,
		Entrypoint: cfg.Entrypoint,
		InputType:  cfg.InputType,
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
	// Determine runtime and select appropriate template
	var templateStr string
	var moduleName string

	switch cfg.Runtime {
	case config.RuntimeNodeJS20:
		templateStr = NodeJSTemplate
		moduleName = stripJSExtension(cfg.Module)
	default: // Python (default)
		templateStr = PythonSyncTemplate
		if async {
			templateStr = PythonAsyncTemplate
		}
		moduleName = stripPyExtension(cfg.Module)
	}

	data := TemplateData{
		Module:     moduleName,
		Entrypoint: cfg.Entrypoint,
		InputType:  cfg.InputType,
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
	// Only remove if it's a generated file (Python or Node.js)
	baseName := filepath.Base(entrypointPath)
	if baseName != entrypointFileNamePython && baseName != entrypointFileNameNodeJS {
		return nil
	}
	return os.Remove(entrypointPath)
}

// GetEntrypointPath returns the path where the entrypoint would be generated.
// For Python agents, use GetEntrypointPathPython.
// For Node.js agents, use GetEntrypointPathNodeJS.
func GetEntrypointPath(agentDir string) string {
	// Default to Python for backwards compatibility
	return filepath.Join(agentDir, entrypointFileNamePython)
}

// GetEntrypointPathForRuntime returns the entrypoint path for a specific runtime
func GetEntrypointPathForRuntime(agentDir string, runtime string) string {
	switch runtime {
	case config.RuntimeNodeJS20:
		return filepath.Join(agentDir, entrypointFileNameNodeJS)
	default:
		return filepath.Join(agentDir, entrypointFileNamePython)
	}
}

// stripPyExtension removes .py extension if present
func stripPyExtension(module string) string {
	return strings.TrimSuffix(module, ".py")
}

// stripJSExtension removes .js, .mjs, or .ts extension if present
func stripJSExtension(module string) string {
	module = strings.TrimSuffix(module, ".js")
	module = strings.TrimSuffix(module, ".mjs")
	module = strings.TrimSuffix(module, ".ts")
	return module
}
