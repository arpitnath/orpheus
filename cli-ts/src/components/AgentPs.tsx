//@AGENT_PS_COMPONENT
import React from 'react';
import { Box, Text, useApp } from 'ink';
import { useAgentList, useStats, useKeyboardNavigation } from '../hooks/index.js';
import { Spinner, StatusDot, ErrorBox } from './common/index.js';
import type { AgentListItem, AgentStats } from '../types/index.js';

//@HELPERS
function getAgentStatus(agent: AgentListItem, agentStats?: AgentStats): 'running' | 'idle' | 'error' {
  // Check stats for worker activity
  if (agentStats?.pool) {
    if (agentStats.pool.busy_workers > 0) return 'running';
    if (agentStats.pool.total_workers > 0) return 'idle';
  }
  // Fall back to agent status
  if (agent.status === 'running') return 'running';
  if (agent.status === 'stopped') return 'error';
  return 'idle';
}

function getWorkerCount(agentStats?: AgentStats): number {
  return agentStats?.pool?.total_workers ?? 0;
}

function getQueueDepth(agentStats?: AgentStats): number {
  return agentStats?.queue?.pending ?? 0;
}

//@EMPTY_STATE
const EmptyState: React.FC = () => (
  <Box flexDirection="column" paddingY={1}>
    <Text bold>Running Agents (0)</Text>
    <Box marginTop={1} />
    <Text dimColor>No agents running.</Text>
    <Box marginTop={1} />
    <Text dimColor>Deploy an agent:</Text>
    <Text>  orpheus deploy ./my-agent</Text>
  </Box>
);

//@HINT_BAR
interface HintBarProps {
  isExpanded: boolean;
  isInteractive: boolean;
}

const HintBar: React.FC<HintBarProps> = ({ isExpanded, isInteractive }) => {
  if (!isInteractive) return null;

  return (
    <Box marginTop={1}>
      <Text dimColor>
        ↑/↓ navigate, {isExpanded ? '← collapse' : '→ expand'}, Enter inspect, r refresh, q quit
      </Text>
    </Box>
  );
};

//@AGENT_ROW
interface AgentRowProps {
  agent: AgentListItem;
  agentStats?: AgentStats;
  isSelected: boolean;
  isExpanded: boolean;
  serverUrl: string;
}

const AgentRow: React.FC<AgentRowProps> = ({
  agent,
  agentStats,
  isSelected,
  isExpanded,
  serverUrl,
}) => {
  const status = getAgentStatus(agent, agentStats);
  const workers = getWorkerCount(agentStats);
  const queue = getQueueDepth(agentStats);
  const maxWorkers = agentStats?.pool?.desired_size ?? 10;

  const indicator = isExpanded ? '▼' : isSelected ? '▶' : ' ';
  const workerText = workers === 1 ? '1 worker' : `${workers} workers`;

  return (
    <Box flexDirection="column">
      {/* Main row */}
      <Box>
        <Text color={isSelected ? 'cyan' : undefined}>{indicator} </Text>
        <Text>{agent.name.padEnd(22)}</Text>
        <Text color="cyan">{agent.runtime.padEnd(10)}</Text>
        <Text>{workerText.padEnd(12)}</Text>
        <StatusDot status={status} showLabel />
      </Box>

      {/* Expanded details */}
      {isExpanded && (
        <Box flexDirection="column" marginLeft={2}>
          <Text dimColor>│ </Text>
          <Box>
            <Text dimColor>│ Endpoint   </Text>
            <Text color="cyan">{serverUrl}/v1/agents/{agent.name}</Text>
          </Box>
          <Box>
            <Text dimColor>│ Workers    </Text>
            <Text>{workers}/{maxWorkers}</Text>
          </Box>
          <Box>
            <Text dimColor>│ Queue      </Text>
            <Text>{queue} pending</Text>
          </Box>
          <Text dimColor>│</Text>
        </Box>
      )}
    </Box>
  );
};

//@COMPONENT
export const AgentPs: React.FC = () => {
  const { exit } = useApp();
  const { agents, loading, error, refetch } = useAgentList();
  const { stats } = useStats();

  const { selectedIndex, expandedIndex, isInteractive } = useKeyboardNavigation({
    itemCount: agents.length,
    onEnter: (index) => {
      // TODO: Transition to inspect view
      // For now, just log the agent name
      const agent = agents[index];
      if (agent) {
        console.log(`\nInspect: ${agent.name}`);
        exit();
      }
    },
    onRefresh: refetch,
    onQuit: exit,
  });

  // Get server URL for endpoint display
  const serverUrl = 'http://localhost:7777'; // TODO: Get from config

  // Loading state
  if (loading) {
    return (
      <Box>
        <Spinner label="Loading agents..." />
      </Box>
    );
  }

  // Error state
  if (error) {
    return <ErrorBox message={error} hint="Check if daemon is running" />;
  }

  // Empty state
  if (agents.length === 0) {
    return <EmptyState />;
  }

  // Find stats for each agent
  const getAgentStats = (agentName: string): AgentStats | undefined => {
    return stats?.agents?.find((s) => s.agent_name === agentName);
  };

  const isExpanded = expandedIndex !== null;

  return (
    <Box flexDirection="column" paddingY={1}>
      <Text bold>Running Agents ({agents.length})</Text>
      <Box marginTop={1} />

      {/* Agent rows */}
      {agents.map((agent, index) => (
        <AgentRow
          key={agent.name}
          agent={agent}
          agentStats={getAgentStats(agent.name)}
          isSelected={index === selectedIndex}
          isExpanded={index === expandedIndex}
          serverUrl={serverUrl}
        />
      ))}

      {/* Hint bar */}
      <HintBar isExpanded={isExpanded} isInteractive={isInteractive} />
    </Box>
  );
};

export default AgentPs;
