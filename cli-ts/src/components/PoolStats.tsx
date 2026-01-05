import React from 'react';
import { Box, Text, useApp } from 'ink';
import { useStats, useRefreshAndQuit } from '../hooks/index.js';
import { Spinner, ErrorBox, WorkerDots, ProgressBar, Row, RefreshHint } from './common/index.js';
import type { AgentStats, GlobalStats } from '../types/index.js';

// ─────────────────────────────────────────────────────────────────────────────
// Helper Components
// ─────────────────────────────────────────────────────────────────────────────

interface QueueTextProps {
  pending: number;
}

const QueueText: React.FC<QueueTextProps> = ({ pending }) => {
  // Color-code queue depth
  if (pending === 0) {
    return <Text dimColor>0</Text>;
  }
  if (pending <= 10) {
    return <Text>{pending}</Text>;
  }
  if (pending < 50) {
    return <Text color="yellow">{pending}</Text>;
  }
  return <Text color="red">{pending}</Text>;
};

interface UtilTextProps {
  util: number;
}

const UtilText: React.FC<UtilTextProps> = ({ util }) => {
  // Color-code utilization
  const color = util < 60 ? 'green' : util < 80 ? 'yellow' : 'red';
  return <Text color={color}>{util.toFixed(0)}%</Text>;
};

// ─────────────────────────────────────────────────────────────────────────────
// Empty State
// ─────────────────────────────────────────────────────────────────────────────

const EmptyState: React.FC = () => (
  <Box flexDirection="column">
    <Text bold>Pool Statistics</Text>
    <Box marginTop={1}>
      <Text dimColor>No agents deployed.</Text>
    </Box>
    <Box marginTop={1} flexDirection="column">
      <Text dimColor>Deploy an agent:</Text>
      <Text color="cyan">  orpheus deploy ./my-agent</Text>
    </Box>
    <RefreshHint />
  </Box>
);

// ─────────────────────────────────────────────────────────────────────────────
// Legacy Agents State (deployed but no pool)
// ─────────────────────────────────────────────────────────────────────────────

interface LegacyAgentsStateProps {
  agents: AgentStats[];
}

const LegacyAgentsState: React.FC<LegacyAgentsStateProps> = ({ agents }) => (
  <Box flexDirection="column">
    <Text bold>Pool Statistics</Text>
    <Box marginTop={1} flexDirection="column">
      <Text dimColor>No pool statistics available.</Text>
      <Box marginTop={1}>
        <Text>{agents.length} agent{agents.length > 1 ? 's' : ''} deployed without worker pools:</Text>
      </Box>
      {agents.map((agent) => (
        <Box key={agent.agent_name} marginLeft={2}>
          <Text dimColor>• </Text>
          <Text>{agent.agent_name}</Text>
          <Text dimColor> (legacy)</Text>
        </Box>
      ))}
    </Box>
    <Box marginTop={1} flexDirection="column">
      <Text dimColor>To enable pool stats, redeploy:</Text>
      <Text color="cyan">  orpheus undeploy {agents[0]?.agent_name ?? '<agent>'}</Text>
      <Text color="cyan">  orpheus deploy ./path/to/agent</Text>
    </Box>
    <RefreshHint />
  </Box>
);

// ─────────────────────────────────────────────────────────────────────────────
// Single Agent View (Simplified)
// ─────────────────────────────────────────────────────────────────────────────

interface SingleAgentStatsProps {
  agent: AgentStats;
}

const SingleAgentStats: React.FC<SingleAgentStatsProps> = ({ agent }) => {
  const busy = agent.pool?.busy_workers ?? 0;
  const total = agent.pool?.total_workers ?? 0;
  const max = agent.pool?.desired_size ?? 10;
  const pending = agent.queue?.pending ?? 0;
  const util = agent.derived?.utilization_percentage ?? 0;

  return (
    <Box flexDirection="column">
      <Text bold>Pool Statistics</Text>
      <Box marginTop={1} flexDirection="column">
        <Row label="Agent" value={agent.agent_name} labelWidth={18} />
        <Box>
          <Text dimColor>{'Workers'.padEnd(18)}</Text>
          <WorkerDots active={busy} total={max} />
          <Text>  {total}/{max}</Text>
        </Box>
        <Row
          label="Queue"
          labelWidth={18}
          value={
            <Box>
              <QueueText pending={pending} />
              <Text dimColor> pending</Text>
            </Box>
          }
        />
        <Box>
          <Text dimColor>{'Utilization'.padEnd(18)}</Text>
          <ProgressBar value={util} max={100} width={16} colorByValue />
          <Text>  </Text>
          <UtilText util={util} />
        </Box>
      </Box>
      <RefreshHint />
    </Box>
  );
};

// ─────────────────────────────────────────────────────────────────────────────
// Multi Agent View (Table)
// ─────────────────────────────────────────────────────────────────────────────

interface AgentRowProps {
  agent: AgentStats;
}

const AgentRow: React.FC<AgentRowProps> = ({ agent }) => {
  const busy = agent.pool?.busy_workers ?? 0;
  const total = agent.pool?.total_workers ?? 0;
  const max = agent.pool?.desired_size ?? 10;
  const pending = agent.queue?.pending ?? 0;
  const util = agent.derived?.utilization_percentage ?? 0;

  // Truncate agent name if too long
  const name = agent.agent_name.length > 24
    ? agent.agent_name.slice(0, 21) + '...'
    : agent.agent_name;

  return (
    <Box>
      <Text>{name.padEnd(26)}</Text>
      <Box width={22}>
        <WorkerDots active={busy} total={max} />
        <Text dimColor>  {total}/{max}</Text>
      </Box>
      <Box width={10}>
        <QueueText pending={pending} />
      </Box>
      <UtilText util={util} />
    </Box>
  );
};

interface TotalRowProps {
  agents: AgentStats[];
}

const TotalRow: React.FC<TotalRowProps> = ({ agents }) => {
  // Calculate totals
  const totalWorkers = agents.reduce((sum, a) => sum + (a.pool?.total_workers ?? 0), 0);
  const totalMax = agents.reduce((sum, a) => sum + (a.pool?.desired_size ?? 10), 0);
  const totalPending = agents.reduce((sum, a) => sum + (a.queue?.pending ?? 0), 0);
  const avgUtil = agents.length > 0
    ? agents.reduce((sum, a) => sum + (a.derived?.utilization_percentage ?? 0), 0) / agents.length
    : 0;

  return (
    <Box>
      <Text bold>{'Total'.padEnd(26)}</Text>
      <Box width={22}>
        <Text dimColor>{totalWorkers}/{totalMax}</Text>
      </Box>
      <Box width={10}>
        <QueueText pending={totalPending} />
      </Box>
      <UtilText util={avgUtil} />
    </Box>
  );
};

interface MultiAgentStatsProps {
  global: GlobalStats;
  agents: AgentStats[];
}

const MultiAgentStats: React.FC<MultiAgentStatsProps> = ({ global, agents }) => {
  // Calculate total max workers from individual agents
  const totalMax = agents.reduce((sum, a) => sum + (a.pool?.desired_size ?? 10), 0);
  const totalBusy = agents.reduce((sum, a) => sum + (a.pool?.busy_workers ?? 0), 0);

  return (
    <Box flexDirection="column">
      {/* Header */}
      <Text bold>Pool Statistics</Text>

      {/* Global Stats */}
      <Box marginTop={1} flexDirection="column">
        <Row label="Total Agents" value={global.total_agents.toString()} labelWidth={18} />
        <Box>
          <Text dimColor>{'Total Workers'.padEnd(18)}</Text>
          <WorkerDots active={totalBusy} total={totalMax} />
          <Text>  {global.total_workers}/{totalMax}</Text>
        </Box>
        <Row
          label="Queue Depth"
          labelWidth={18}
          value={
            <Box>
              <QueueText pending={global.total_pending} />
              <Text dimColor> pending</Text>
            </Box>
          }
        />
        <Box>
          <Text dimColor>{'Utilization'.padEnd(18)}</Text>
          <ProgressBar value={global.average_utilization} max={100} width={16} colorByValue />
          <Text>  </Text>
          <UtilText util={global.average_utilization} />
        </Box>
      </Box>

      {/* Per-Agent Table */}
      <Box marginTop={1} flexDirection="column">
        <Text bold>Per-Agent Breakdown</Text>
        <Text dimColor>{'─'.repeat(65)}</Text>
        {/* Header */}
        <Box>
          <Text dimColor>{'AGENT'.padEnd(26)}</Text>
          <Text dimColor>{'WORKERS'.padEnd(22)}</Text>
          <Text dimColor>{'QUEUE'.padEnd(10)}</Text>
          <Text dimColor>UTIL</Text>
        </Box>
        <Text dimColor>{'─'.repeat(65)}</Text>
        {/* Rows */}
        {agents.map((agent) => (
          <AgentRow key={agent.agent_name} agent={agent} />
        ))}
        <Text dimColor>{'─'.repeat(65)}</Text>
        {/* Total */}
        <TotalRow agents={agents} />
      </Box>

      <RefreshHint />
    </Box>
  );
};

// ─────────────────────────────────────────────────────────────────────────────
// Main Component
// ─────────────────────────────────────────────────────────────────────────────

export const PoolStats: React.FC = () => {
  const { exit } = useApp();
  const { stats, loading, error, refetch } = useStats();

  // Keyboard: r to refresh, q to quit
  useRefreshAndQuit(refetch, exit);

  if (loading) {
    return <Spinner label="Loading stats..." />;
  }

  if (error) {
    return <ErrorBox message={error} hint="Is the daemon running? Try: orpheus healthcheck" />;
  }

  if (!stats || stats.agents.length === 0) {
    return <EmptyState />;
  }

  // Filter to agents with pools
  const agentsWithPools = stats.agents.filter((a) => a.pool);
  const legacyAgents = stats.agents.filter((a) => !a.pool);

  // All agents are legacy (no pools)
  if (agentsWithPools.length === 0 && legacyAgents.length > 0) {
    return <LegacyAgentsState agents={legacyAgents} />;
  }

  if (agentsWithPools.length === 0) {
    return <EmptyState />;
  }

  // Single agent - simplified view
  if (agentsWithPools.length === 1) {
    return <SingleAgentStats agent={agentsWithPools[0]} />;
  }

  // Multiple agents - table view
  return <MultiAgentStats global={stats.global} agents={agentsWithPools} />;
};
