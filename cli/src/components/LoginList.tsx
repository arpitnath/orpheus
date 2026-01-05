import React, { useState, useEffect, useMemo } from 'react';
import { Box, Text, useApp } from 'ink';
import { listServers, getActiveServerName } from '../lib/config.js';
import { getHealth } from '../lib/api.js';
import type { ServerConfig } from '../types/index.js';
import { StatusBadge, type BadgeStatus } from './common/index.js';

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

interface ServerStatus {
  name: string;
  config: ServerConfig;
  status: 'connected' | 'offline' | 'checking';
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

function truncate(str: string, maxLen: number): string {
  if (str.length <= maxLen) return str;
  return str.slice(0, maxLen - 3) + '...';
}

async function checkServerStatus(): Promise<'connected' | 'offline'> {
  try {
    const health = await getHealth();
    return health ? 'connected' : 'offline';
  } catch {
    return 'offline';
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Server Row Component
// ─────────────────────────────────────────────────────────────────────────────

interface ServerRowProps {
  server: ServerStatus;
  isActive: boolean;
}

const ServerRow: React.FC<ServerRowProps> = ({ server, isActive }) => {
  const mode = server.config.mode === 'unix_socket' ? 'socket' : 'tcp';
  const endpoint = server.config.mode === 'unix_socket'
    ? server.config.socket_path ?? ''
    : server.config.url ?? '';

  const getBadgeStatus = (): BadgeStatus => {
    if (server.status === 'checking') return 'pending';
    return server.status;
  };

  return (
    <Box marginBottom={1}>
      <Text color={isActive ? 'green' : undefined}>
        {isActive ? '→ ' : '  '}
      </Text>
      <Text bold={isActive}>{server.name.padEnd(14)}</Text>
      <Text dimColor>{mode.padEnd(10)}</Text>
      <Text>{truncate(endpoint, 38).padEnd(40)}</Text>
      <StatusBadge status={getBadgeStatus()} />
    </Box>
  );
};

// ─────────────────────────────────────────────────────────────────────────────
// Main Component
// ─────────────────────────────────────────────────────────────────────────────

export const LoginList: React.FC = () => {
  const { exit } = useApp();

  // Memoize servers to prevent re-creating on every render
  const servers = useMemo(() => listServers(), []);
  const activeServerName = useMemo(() => getActiveServerName(), []);

  const [phase, setPhase] = useState<'checking' | 'done'>('checking');
  const [serverStatuses, setServerStatuses] = useState<ServerStatus[]>(
    Object.entries(servers).map(([name, config]) => ({
      name,
      config,
      status: name === activeServerName ? 'checking' : 'offline',
    }))
  );

  // Effect 1: Perform health check on mount
  useEffect(() => {
    let cancelled = false;

    async function checkActiveServer() {
      const activeServer = servers[activeServerName];
      if (!activeServer) {
        setPhase('done');
        return;
      }

      const status = await checkServerStatus();

      if (!cancelled) {
        setServerStatuses((prev) =>
          prev.map((s) =>
            s.name === activeServerName ? { ...s, status } : s
          )
        );
        setPhase('done');
      }
    }

    checkActiveServer();

    return () => {
      cancelled = true;
    };
  }, [activeServerName, servers]);

  // Effect 2: Exit after render completes with final state
  useEffect(() => {
    if (phase === 'done') {
      exit();
    }
  }, [phase, exit]);

  const serverList = Object.keys(servers);

  if (serverList.length === 0) {
    return (
      <Box flexDirection="column">
        <Text bold>Server Configuration</Text>
        <Box marginTop={1}>
          <Text dimColor>No servers configured.</Text>
        </Box>
        <Box marginTop={1} flexDirection="column">
          <Text dimColor>Add a server:</Text>
          <Text color="cyan">  orpheus login add {'<name>'} {'<url>'}</Text>
        </Box>
      </Box>
    );
  }

  return (
    <Box flexDirection="column">
      <Text bold>Server Configuration</Text>
      <Box marginTop={1} flexDirection="column">
        {/* Header */}
        <Box>
          <Text dimColor>{'  '}</Text>
          <Text dimColor>{'NAME'.padEnd(14)}</Text>
          <Text dimColor>{'MODE'.padEnd(10)}</Text>
          <Text dimColor>{'ENDPOINT'.padEnd(40)}</Text>
          <Text dimColor>STATUS</Text>
        </Box>
        <Text dimColor>{'─'.repeat(78)}</Text>

        {/* Rows */}
        {serverStatuses.map((server) => (
          <ServerRow
            key={server.name}
            server={server}
            isActive={server.name === activeServerName}
          />
        ))}
      </Box>

      {/* Active indicator */}
      <Box marginTop={1}>
        <Text dimColor>Active: </Text>
        <Text bold color="green">{activeServerName}</Text>
      </Box>

      {/* Help */}
      <Box marginTop={1} flexDirection="column">
        <Text dimColor>Commands:</Text>
        <Text dimColor>  orpheus login add {'<name>'} {'<url>'}    Add server</Text>
        <Text dimColor>  orpheus login use {'<name>'}          Switch server</Text>
        <Text dimColor>  orpheus login remove {'<name>'}       Remove server</Text>
      </Box>
    </Box>
  );
};
