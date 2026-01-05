//@STATUS_COMPONENT
import React, { useState, useEffect } from 'react';
import { Box, Text, useApp } from 'ink';
import { Spinner } from './common/Spinner.js';
import { getHealth, getStats } from '../lib/api.js';
import { getActiveServerName, getActiveServer } from '../lib/config.js';
import type { HealthResponse, StatsResponse } from '../types/index.js';

//@HELPERS
function formatUptime(seconds: number): string {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = Math.floor(seconds % 60);

  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  }
  if (minutes > 0) {
    return `${minutes}m ${secs}s`;
  }
  return `${secs}s`;
}

function getStatusIndicator(health: HealthResponse | null, stats: StatsResponse | null): {
  icon: string;
  color: string;
  text: string;
} {
  if (!health) {
    return { icon: '✗', color: 'red', text: 'not running' };
  }

  // Check for degraded state: workers=0 but queue>0
  if (stats?.global) {
    const { total_workers, total_pending } = stats.global;
    if (total_workers === 0 && total_pending > 0) {
      return { icon: '!', color: 'yellow', text: 'degraded' };
    }
  }

  if (health.status === 'healthy') {
    return { icon: '●', color: 'green', text: 'healthy' };
  }

  if (health.status === 'degraded') {
    return { icon: '!', color: 'yellow', text: 'degraded' };
  }

  return { icon: '✗', color: 'red', text: 'unhealthy' };
}

function getHint(health: HealthResponse | null, stats: StatsResponse | null): string | null {
  if (!health) {
    return 'Run: orpheus vm start';
  }

  if (stats?.global) {
    const { total_workers, total_pending } = stats.global;
    if (total_workers === 0 && total_pending > 0) {
      return `${total_pending} requests queued, no workers available`;
    }
  }

  return null;
}

function getServerUrl(): string {
  const server = getActiveServer();
  if (server.mode === 'tcp' && server.url) {
    return server.url;
  }
  if (server.mode === 'unix_socket' && server.socket_path) {
    return server.socket_path;
  }
  return '';
}

//@COMPONENT
export const Status: React.FC = () => {
  const { exit } = useApp();
  const [loading, setLoading] = useState(true);
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [stats, setStats] = useState<StatsResponse | null>(null);

  const serverName = getActiveServerName();
  const serverUrl = getServerUrl();

  useEffect(() => {
    const fetchData = async () => {
      try {
        const healthData = await getHealth();
        setHealth(healthData);

        if (healthData) {
          const statsData = await getStats();
          setStats(statsData);
        }
      } catch {
        // Error handled by null health
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, []);

  // Auto-exit after data is loaded and rendered
  useEffect(() => {
    if (!loading) {
      const timer = setTimeout(() => {
        exit();
      }, 100);
      return () => clearTimeout(timer);
    }
    return undefined;
  }, [loading, exit]);

  // Loading state
  if (loading) {
    return (
      <Box>
        <Spinner label="Checking status..." />
      </Box>
    );
  }

  const status = getStatusIndicator(health, stats);
  const hint = getHint(health, stats);
  const uptime = health ? formatUptime(health.uptime_seconds) : '';

  // Stats values with defaults
  const agents = stats?.global?.total_agents ?? 0;
  const workers = stats?.global?.total_workers ?? 0;
  const queue = stats?.global?.total_pending ?? 0;

  return (
    <Box flexDirection="column" paddingY={1}>
      {/* Row 1: Status indicator + Server name */}
      <Box justifyContent="space-between" width={60}>
        <Text color={status.color}>{status.icon} {status.text}</Text>
        <Text>{serverName}</Text>
      </Box>

      {/* Row 2: Uptime + URL */}
      <Box justifyContent="space-between" width={60}>
        {health ? (
          <Text dimColor>  Uptime {uptime}</Text>
        ) : (
          <Text> </Text>
        )}
        <Text dimColor>{serverUrl}</Text>
      </Box>

      {/* Stats row (only if daemon is running) */}
      {health && (
        <>
          <Box marginY={1} />
          <Box gap={4}>
            <Box>
              <Text dimColor>  Agents</Text>
              <Text>     {agents}</Text>
            </Box>
            <Box>
              <Text dimColor>Workers</Text>
              <Text>    {workers}</Text>
            </Box>
            <Box>
              <Text dimColor>Queue</Text>
              <Text>    {queue}</Text>
            </Box>
          </Box>
        </>
      )}

      {/* Hint (only if there's an issue) */}
      {hint && (
        <Box marginTop={1}>
          <Text color="cyan">  → {hint}</Text>
        </Box>
      )}
    </Box>
  );
};

export default Status;
