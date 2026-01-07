import React from 'react';
import { Box, Text, useApp } from 'ink';
import { useAgentDetails, useStats, useAgentList, useTabNavigation } from '../../hooks/index.js';
import { Spinner, StatusBadge, ErrorBox } from '../common/index.js';
import { TabBar } from './TabBar.js';
import { OverviewTab } from './OverviewTab.js';
import { WorkersTab } from './WorkersTab.js';
import type { AgentDetails, AgentStats } from '../../types/index.js';
import type { BadgeStatus } from '../common/index.js';

//@HELPERS
function getAgentStatus(agent: AgentDetails, agentStats?: AgentStats): BadgeStatus {
  if (agentStats?.pool) {
    if (agentStats.pool.busy_workers > 0) return 'running';
    if (agentStats.pool.total_workers > 0) return 'idle';
  }
  if (agent.status === 'running') return 'running';
  if (agent.status === 'stopped') return 'error';
  return 'idle';
}

//@TABS
const TABS = ['Overview', 'Workers'];

//@ERROR_STATE
interface InspectErrorProps {
  agentName: string;
  error: string | null;
  availableAgents: string[];
}

const InspectError: React.FC<InspectErrorProps> = ({ agentName, error, availableAgents }) => (
  <Box flexDirection="column" paddingY={1}>
    <ErrorBox
      message={error || `Agent '${agentName}' not found`}
      hint={availableAgents.length > 0 ? undefined : 'Use `orpheus list` to see all agents'}
    />
    {availableAgents.length > 0 && (
      <Box flexDirection="column" marginTop={1}>
        <Text dimColor>Available agents:</Text>
        {availableAgents.map((name) => (
          <Text key={name}>  - {name}</Text>
        ))}
        <Box marginTop={1}>
          <Text dimColor>Use `orpheus list` to see all agents</Text>
        </Box>
      </Box>
    )}
  </Box>
);

//@COMPONENT
interface AgentInspectProps {
  agentName: string;
}

export const AgentInspect: React.FC<AgentInspectProps> = ({ agentName }) => {
  const { exit } = useApp();
  const { agent, loading, error } = useAgentDetails(agentName);
  const { stats } = useStats(agentName);
  const { agents: allAgents } = useAgentList();
  const { activeTab, isInteractive } = useTabNavigation({
    tabCount: TABS.length,
    onQuit: exit,
  });

  const serverUrl = 'http://localhost:7777'; // TODO: Get from config

  // Loading state
  if (loading) {
    return (
      <Box paddingY={1}>
        <Spinner label={`Loading ${agentName}...`} />
      </Box>
    );
  }

  // Error state
  if (error || !agent) {
    const availableAgents = allAgents
      .filter((a) => a.name !== agentName)
      .map((a) => a.name);
    return <InspectError agentName={agentName} error={error} availableAgents={availableAgents} />;
  }

  // Get agent stats
  const agentStats = stats?.agents?.find((s) => s.agent_name === agentName);
  const status = getAgentStatus(agent, agentStats);

  return (
    <Box flexDirection="column" paddingY={1}>
      {/* Tab bar */}
      <TabBar tabs={TABS} activeIndex={activeTab} />

      <Box marginTop={1} />

      {/* Header */}
      <Box justifyContent="space-between">
        <Text bold>{agent.name}</Text>
        <StatusBadge status={status} />
      </Box>

      <Box marginTop={1} />

      {/* Tab content */}
      {activeTab === 0 && <OverviewTab agent={agent} serverUrl={serverUrl} />}
      {activeTab === 1 && <WorkersTab agent={agent} agentStats={agentStats} />}

      {/* Hint bar */}
      {isInteractive && (
        <Box marginTop={1}>
          <Text dimColor>esc to exit · tab to switch</Text>
        </Box>
      )}
    </Box>
  );
};

export default AgentInspect;
