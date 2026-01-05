import React, { useState, useEffect } from 'react';
import { Box, Text, useApp } from 'ink';
import { platform } from 'node:os';
import { execSync } from 'node:child_process';
import { getHealth } from '../lib/api.js';
import { socketExists, getActiveServerName, getActiveServer } from '../lib/config.js';
import { CheckItem, StatusBadge, type CheckStatus } from './common/index.js';
import type { HealthResponse } from '../types/index.js';

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

interface CheckResult {
  status: CheckStatus;
  label: string;
  detail?: string;
  error?: string;
  fix?: string;
}

interface HealthCheckState {
  configCheck: CheckResult;
  connectionCheck: CheckResult;
  statusCheck: CheckResult;
  versionCheck: CheckResult;
  uptimeCheck: CheckResult;
  vmCheck: CheckResult | null;
  phase: 'running' | 'done';
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

function formatUptime(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
  const hours = Math.floor(seconds / 3600);
  const mins = Math.floor((seconds % 3600) / 60);
  return `${hours}h ${mins}m`;
}

function getVmStatus(): { status: CheckStatus; detail: string; error?: string } {
  try {
    const output = execSync('limactl list --json 2>/dev/null', { encoding: 'utf-8' });
    const parsed = JSON.parse(output);
    const vms = Array.isArray(parsed) ? parsed : [parsed];
    const orpheusVm = vms.find((vm: { name: string }) => vm.name === 'orpheus');

    if (orpheusVm && orpheusVm.status === 'Running') {
      return { status: 'passed', detail: 'Running' };
    } else if (orpheusVm) {
      return { status: 'warning', detail: orpheusVm.status.toLowerCase(), error: 'VM not running' };
    } else {
      return { status: 'warning', detail: 'Not found', error: 'Run: orpheus vm start' };
    }
  } catch {
    return { status: 'skipped', detail: 'Lima not installed' };
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Section Component
// ─────────────────────────────────────────────────────────────────────────────

interface SectionProps {
  title: string;
  children: React.ReactNode;
}

const Section: React.FC<SectionProps> = ({ title, children }) => (
  <Box flexDirection="column" marginBottom={1}>
    <Text bold dimColor>{title}</Text>
    <Box flexDirection="column" marginLeft={2}>
      {children}
    </Box>
  </Box>
);

// ─────────────────────────────────────────────────────────────────────────────
// Connection Row with Badge
// ─────────────────────────────────────────────────────────────────────────────

interface ConnectionRowProps {
  check: CheckResult;
}

const ConnectionRow: React.FC<ConnectionRowProps> = ({ check }) => {
  const isConnected = check.status === 'passed';
  const isFailed = check.status === 'failed';
  const isRunning = check.status === 'running';

  return (
    <Box flexDirection="column">
      <Box>
        {isRunning ? (
          <Text color="cyan">◐</Text>
        ) : isConnected ? (
          <Text color="green">✓</Text>
        ) : isFailed ? (
          <Text color="red">✗</Text>
        ) : (
          <Text dimColor>○</Text>
        )}
        <Text> {check.label.padEnd(20)}</Text>
        {check.status !== 'running' && check.status !== 'pending' && (
          <StatusBadge status={isConnected ? 'connected' : 'disconnected'} />
        )}
      </Box>
      {isFailed && check.error && (
        <Box marginLeft={4} flexDirection="column">
          <Text dimColor>{check.error}</Text>
          {check.fix && (
            <Box>
              <Text color="cyan">Fix: </Text>
              <Text>{check.fix}</Text>
            </Box>
          )}
        </Box>
      )}
    </Box>
  );
};

// ─────────────────────────────────────────────────────────────────────────────
// Main Component
// ─────────────────────────────────────────────────────────────────────────────

export const HealthCheck: React.FC = () => {
  const { exit } = useApp();
  const isMac = platform() === 'darwin';

  const [state, setState] = useState<HealthCheckState>({
    configCheck: { status: 'running', label: 'Config' },
    connectionCheck: { status: 'pending', label: 'Daemon reachable' },
    statusCheck: { status: 'pending', label: 'Status' },
    versionCheck: { status: 'pending', label: 'Version' },
    uptimeCheck: { status: 'pending', label: 'Uptime' },
    vmCheck: isMac ? { status: 'pending', label: 'Lima VM' } : null,
    phase: 'running',
  });

  useEffect(() => {
    async function runChecks() {
      let health: HealthResponse | null = null;

      // Check 1: Config
      const configValid = socketExists() || getActiveServerName() !== 'local';
      const server = getActiveServer();
      const serverMode = server.mode === 'tcp' ? server.url : 'unix socket';

      setState((prev) => ({
        ...prev,
        configCheck: {
          status: configValid ? 'passed' : 'warning',
          label: 'Config',
          detail: configValid ? serverMode : 'Using defaults',
          error: configValid ? undefined : 'No config found',
        },
        connectionCheck: { ...prev.connectionCheck, status: 'running' },
      }));

      // Check 2: Connection + Health (single call)
      health = await getHealth();
      const connected = health !== null;

      setState((prev) => ({
        ...prev,
        connectionCheck: {
          status: connected ? 'passed' : 'failed',
          label: 'Daemon reachable',
          detail: connected ? 'Connected' : undefined,
          error: connected ? undefined : 'Cannot connect to daemon',
          fix: connected ? undefined : 'Start daemon: orpheus vm start',
        },
        statusCheck: { ...prev.statusCheck, status: connected ? 'running' : 'skipped' },
      }));

      // Check 3-5: Health (only if connected)
      if (connected) {

        setState((prev) => ({
          ...prev,
          statusCheck: {
            status: health?.status === 'healthy' ? 'passed' : health?.status === 'degraded' ? 'warning' : 'failed',
            label: 'Status',
            detail: health?.status ?? 'Unknown',
            error: health?.status !== 'healthy' ? `Daemon is ${health?.status}` : undefined,
          },
          versionCheck: {
            status: health?.version ? 'passed' : 'warning',
            label: 'Version',
            detail: health?.version ?? 'Unknown',
          },
          uptimeCheck: {
            status: 'passed',
            label: 'Uptime',
            detail: health?.uptime_seconds ? formatUptime(health.uptime_seconds) : 'Unknown',
          },
          vmCheck: prev.vmCheck ? { ...prev.vmCheck, status: 'running' } : null,
        }));
      } else {
        setState((prev) => ({
          ...prev,
          statusCheck: { status: 'skipped', label: 'Status', detail: 'Skipped' },
          versionCheck: { status: 'skipped', label: 'Version', detail: 'Skipped' },
          uptimeCheck: { status: 'skipped', label: 'Uptime', detail: 'Skipped' },
          vmCheck: prev.vmCheck ? { ...prev.vmCheck, status: 'running' } : null,
        }));
      }

      // Check 6: Lima VM (macOS only)
      if (isMac) {
        const vmResult = getVmStatus();
        setState((prev) => ({
          ...prev,
          vmCheck: {
            status: vmResult.status,
            label: 'Lima VM',
            detail: vmResult.detail,
            error: vmResult.error,
          },
        }));
      }

      // Done
      setState((prev) => ({ ...prev, phase: 'done' }));

      // Exit after a brief delay to show results
      setTimeout(() => exit(), 100);
    }

    runChecks();
  }, [exit, isMac]);

  // Calculate overall status
  const allChecks = [
    state.configCheck,
    state.connectionCheck,
    state.statusCheck,
    state.versionCheck,
    state.uptimeCheck,
    state.vmCheck,
  ].filter(Boolean) as CheckResult[];

  const hasFailed = allChecks.some((c) => c.status === 'failed');
  const hasWarning = allChecks.some((c) => c.status === 'warning');

  return (
    <Box flexDirection="column">
      <Text bold>Health Checks</Text>
      <Box marginTop={1} />

      {/* Connection Section */}
      <Section title="Connection">
        <CheckItem
          status={state.configCheck.status}
          label={state.configCheck.label}
          timing={state.configCheck.detail}
          error={state.configCheck.error}
          labelWidth={20}
        />
        <ConnectionRow check={state.connectionCheck} />
      </Section>

      {/* Daemon Status Section */}
      <Section title="Daemon Status">
        <CheckItem
          status={state.statusCheck.status}
          label={state.statusCheck.label}
          timing={state.statusCheck.detail}
          error={state.statusCheck.error}
          labelWidth={20}
        />
        <CheckItem
          status={state.versionCheck.status}
          label={state.versionCheck.label}
          timing={state.versionCheck.detail}
          labelWidth={20}
        />
        <CheckItem
          status={state.uptimeCheck.status}
          label={state.uptimeCheck.label}
          timing={state.uptimeCheck.detail}
          labelWidth={20}
        />
      </Section>

      {/* Infrastructure Section (macOS only) */}
      {state.vmCheck && (
        <Section title="Infrastructure">
          <CheckItem
            status={state.vmCheck.status}
            label={state.vmCheck.label}
            timing={state.vmCheck.detail}
            error={state.vmCheck.error}
            labelWidth={20}
          />
        </Section>
      )}

      {/* Summary */}
      {state.phase === 'done' && (
        <Box marginTop={1}>
          {hasFailed ? (
            <Text color="red">Some checks failed</Text>
          ) : hasWarning ? (
            <Text color="yellow">Checks passed with warnings</Text>
          ) : (
            <Text color="green">All systems operational</Text>
          )}
        </Box>
      )}
    </Box>
  );
};
