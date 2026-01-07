import React, { useState, useEffect } from 'react';
import { Box, Text, useApp } from 'ink';
import { existsSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { validateAgentYaml, type ValidationResult } from '../lib/validate.js';
import { CheckItem, Row, type CheckStatus } from './common/index.js';

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

interface ValidationStep {
  label: string;
  status: CheckStatus;
  detail?: string;
  error?: string;
}

interface ValidateProps {
  path: string;
}

// ─────────────────────────────────────────────────────────────────────────────
// Main Component
// ─────────────────────────────────────────────────────────────────────────────

export const Validate: React.FC<ValidateProps> = ({ path }) => {
  const { exit } = useApp();
  const [steps, setSteps] = useState<ValidationStep[]>([
    { label: 'Finding agent.yaml', status: 'running' },
    { label: 'Parsing YAML', status: 'pending' },
    { label: 'Required fields', status: 'pending' },
    { label: 'Runtime supported', status: 'pending' },
    { label: 'Module file exists', status: 'pending' },
  ]);
  const [result, setResult] = useState<ValidationResult | null>(null);
  const [phase, setPhase] = useState<'validating' | 'done'>('validating');

  useEffect(() => {
    async function runValidation() {
      // Step 1: Find agent.yaml
      let yamlPath = path;
      if (!path.endsWith('.yaml') && !path.endsWith('.yml')) {
        yamlPath = join(path, 'agent.yaml');
        if (!existsSync(yamlPath)) {
          yamlPath = join(path, 'agent.yml');
        }
      }

      const yamlExists = existsSync(yamlPath);
      setSteps((prev) => [
        {
          ...prev[0],
          status: yamlExists ? 'passed' : 'failed',
          detail: yamlExists ? yamlPath : undefined,
          error: yamlExists ? undefined : `agent.yaml not found at ${path}`,
        },
        { ...prev[1], status: yamlExists ? 'running' : 'skipped' },
        ...prev.slice(2),
      ]);

      if (!yamlExists) {
        setPhase('done');
        setTimeout(() => exit(), 100);
        return;
      }

      // Small delay for visual effect
      await new Promise((r) => setTimeout(r, 50));

      // Run full validation
      const validationResult = validateAgentYaml(path);
      setResult(validationResult);

      // Step 2: Parse YAML
      const parseError = validationResult.errors.find((e) => e.includes('parse'));
      setSteps((prev) => [
        prev[0],
        {
          ...prev[1],
          status: parseError ? 'failed' : 'passed',
          error: parseError,
        },
        { ...prev[2], status: parseError ? 'skipped' : 'running' },
        ...prev.slice(3),
      ]);

      if (parseError) {
        setPhase('done');
        setTimeout(() => exit(), 100);
        return;
      }

      await new Promise((r) => setTimeout(r, 50));

      // Step 3: Required fields
      const missingFields = validationResult.errors.filter((e) => e.includes('Missing required'));
      setSteps((prev) => [
        ...prev.slice(0, 2),
        {
          ...prev[2],
          status: missingFields.length === 0 ? 'passed' : 'failed',
          error: missingFields.length > 0 ? missingFields.join(', ') : undefined,
        },
        { ...prev[3], status: 'running' },
        prev[4],
      ]);

      await new Promise((r) => setTimeout(r, 50));

      // Step 4: Runtime supported
      const runtimeError = validationResult.errors.find((e) => e.includes('Invalid runtime'));
      const runtime = validationResult.config?.runtime;
      setSteps((prev) => [
        ...prev.slice(0, 3),
        {
          ...prev[3],
          status: runtimeError ? 'failed' : 'passed',
          detail: runtime ? `(${runtime})` : undefined,
          error: runtimeError,
        },
        { ...prev[4], status: 'running' },
      ]);

      await new Promise((r) => setTimeout(r, 50));

      // Step 5: Module file exists
      // Handle different module formats:
      // - Direct file: agent.js, handler.py
      // - Python package: calculator/ (directory with __init__.py)
      // - Without extension: calculator -> calculator.py or calculator/
      const modulePath = validationResult.config?.module;
      // reuse 'runtime' from Step 4
      let moduleExists = false;
      if (modulePath) {
        const baseDir = dirname(yamlPath);
        const fullModulePath = join(baseDir, modulePath);

        // Check direct path first
        if (existsSync(fullModulePath)) {
          moduleExists = true;
        }
        // For Python: check as package (directory with __init__.py)
        else if (runtime === 'python3') {
          const packageInit = join(fullModulePath, '__init__.py');
          if (existsSync(packageInit)) {
            moduleExists = true;
          }
          // Or with .py extension
          else if (existsSync(fullModulePath + '.py')) {
            moduleExists = true;
          }
        }
        // For Node.js: check with .js extension
        else if (runtime === 'nodejs20') {
          if (existsSync(fullModulePath + '.js')) {
            moduleExists = true;
          }
        }
      }

      setSteps((prev) => [
        ...prev.slice(0, 4),
        {
          ...prev[4],
          status: moduleExists ? 'passed' : validationResult.valid ? 'warning' : 'skipped',
          detail: modulePath ? `(${modulePath})` : undefined,
          error: !moduleExists && modulePath ? `File not found: ${modulePath}` : undefined,
        },
      ]);

      setPhase('done');
      setTimeout(() => exit(), 100);
    }

    runValidation();
  }, [path, exit]);

  // Count results
  const errorCount = steps.filter((s) => s.status === 'failed').length;
  const warningCount = result?.warnings.length ?? 0;

  return (
    <Box flexDirection="column">
      <Text bold>Validating agent.yaml</Text>
      <Box marginTop={1} flexDirection="column">
        {steps.map((step, i) => (
          <CheckItem
            key={i}
            status={step.status}
            label={step.label}
            timing={step.detail}
            error={step.error}
            labelWidth={24}
          />
        ))}
      </Box>

      {/* Warnings */}
      {result && result.warnings.length > 0 && (
        <Box marginTop={1} flexDirection="column">
          {result.warnings.map((warning, i) => (
            <Box key={i}>
              <Text color="yellow">! </Text>
              <Text>{warning}</Text>
            </Box>
          ))}
        </Box>
      )}

      {/* Divider */}
      {phase === 'done' && (
        <Box marginTop={1}>
          <Text dimColor>{'─'.repeat(55)}</Text>
        </Box>
      )}

      {/* Config display when valid */}
      {phase === 'done' && result?.valid && result.config && (
        <Box marginTop={1} flexDirection="column">
          <Text bold>Agent Configuration</Text>
          <Box marginTop={1} flexDirection="column">
            <Row label="Name" value={result.config.name} labelWidth={14} />
            <Row label="Runtime" value={result.config.runtime} labelWidth={14} />
            <Row label="Module" value={result.config.module} labelWidth={14} />
            <Row label="Entrypoint" value={result.config.entrypoint} labelWidth={14} />
            {result.config.scaling && (
              <Row
                label="Scaling"
                value={`${result.config.scaling.min_workers}-${result.config.scaling.max_workers} workers`}
                labelWidth={14}
              />
            )}
          </Box>
          <Box marginTop={1}>
            <Text color="green">Ready to deploy: </Text>
            <Text color="cyan">orpheus deploy {path}</Text>
          </Box>
        </Box>
      )}

      {/* Summary */}
      {phase === 'done' && (
        <Box marginTop={1}>
          {errorCount > 0 ? (
            <Text color="red">
              {errorCount} error{errorCount > 1 ? 's' : ''} found. Fix and re-run validation.
            </Text>
          ) : warningCount > 0 ? (
            <Text color="yellow">
              {warningCount} warning{warningCount > 1 ? 's' : ''}. Ready to deploy with defaults.
            </Text>
          ) : null}
        </Box>
      )}
    </Box>
  );
};
