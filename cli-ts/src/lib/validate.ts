//@VALIDATE
import { readFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { parse } from 'yaml';

export interface ValidationResult {
  valid: boolean;
  errors: string[];
  warnings: string[];
  config?: {
    name: string;
    runtime: string;
    module: string;
    entrypoint: string;
    scaling?: { min_workers: number; max_workers: number };
  };
}

const VALID_RUNTIMES = ['python3', 'nodejs20'];

export function validateAgentYaml(agentPath: string): ValidationResult {
  const errors: string[] = [];
  const warnings: string[] = [];

  // Find agent.yaml
  let yamlPath = agentPath;
  if (!agentPath.endsWith('.yaml') && !agentPath.endsWith('.yml')) {
    yamlPath = join(agentPath, 'agent.yaml');
    if (!existsSync(yamlPath)) {
      yamlPath = join(agentPath, 'agent.yml');
    }
  }

  if (!existsSync(yamlPath)) {
    return {
      valid: false,
      errors: [`agent.yaml not found at ${agentPath}`],
      warnings: [],
    };
  }

  // Parse YAML
  let config: Record<string, unknown>;
  try {
    const content = readFileSync(yamlPath, 'utf-8');
    config = parse(content) as Record<string, unknown>;
  } catch (err) {
    return {
      valid: false,
      errors: [`Failed to parse YAML: ${err instanceof Error ? err.message : err}`],
      warnings: [],
    };
  }

  // Required fields
  if (!config.name || typeof config.name !== 'string') {
    errors.push('Missing required field: name');
  }

  if (!config.runtime || typeof config.runtime !== 'string') {
    errors.push('Missing required field: runtime');
  } else if (!VALID_RUNTIMES.includes(config.runtime)) {
    errors.push(`Invalid runtime: ${config.runtime}. Must be one of: ${VALID_RUNTIMES.join(', ')}`);
  }

  if (!config.module || typeof config.module !== 'string') {
    errors.push('Missing required field: module');
  }

  if (!config.entrypoint || typeof config.entrypoint !== 'string') {
    errors.push('Missing required field: entrypoint');
  }

  // Optional fields validation
  if (config.scaling) {
    const scaling = config.scaling as Record<string, unknown>;
    if (typeof scaling.min_workers !== 'number' || scaling.min_workers < 0) {
      warnings.push('scaling.min_workers should be a non-negative number');
    }
    if (typeof scaling.max_workers !== 'number' || scaling.max_workers < 1) {
      warnings.push('scaling.max_workers should be a positive number');
    }
    if (typeof scaling.min_workers === 'number' && typeof scaling.max_workers === 'number') {
      if (scaling.min_workers > scaling.max_workers) {
        errors.push('scaling.min_workers cannot be greater than scaling.max_workers');
      }
    }
  }

  if (config.memory_mb && (typeof config.memory_mb !== 'number' || config.memory_mb < 1)) {
    warnings.push('memory_mb should be a positive number');
  }

  if (config.timeout_seconds && (typeof config.timeout_seconds !== 'number' || config.timeout_seconds < 1)) {
    warnings.push('timeout_seconds should be a positive number');
  }

  return {
    valid: errors.length === 0,
    errors,
    warnings,
    config: errors.length === 0 ? {
      name: config.name as string,
      runtime: config.runtime as string,
      module: config.module as string,
      entrypoint: config.entrypoint as string,
      scaling: config.scaling as { min_workers: number; max_workers: number } | undefined,
    } : undefined,
  };
}
