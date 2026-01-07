//@AGENT_LIST_COMPONENT
import React, { useEffect } from 'react';
import { Box, Text, useApp } from 'ink';
import { useAgentList } from '../hooks/index.js';
import { Spinner, Table, ErrorBox, StatusBadge } from './common/index.js';
import type { Column, BadgeStatus } from './common/index.js';
import type { AgentListItem } from '../types/index.js';

//@COLUMNS
const columns: Column[] = [
  { key: 'name', header: 'NAME', width: 24 },
  { key: 'runtime', header: 'RUNTIME', width: 12, color: 'cyan' },
  { key: 'status', header: 'STATUS', width: 12 },
];

//@EMPTY_STATE
const EmptyState: React.FC = () => (
  <Box flexDirection="column" paddingY={1}>
    <Text bold>Deployed Agents (0)</Text>
    <Box marginTop={1} />
    <Text dimColor>No agents deployed yet.</Text>
    <Box marginTop={1} />
    <Text dimColor>Deploy your first agent:</Text>
    <Text>  orpheus deploy ./my-agent</Text>
  </Box>
);

//@COMPONENT
export const AgentList: React.FC = () => {
  const { exit } = useApp();
  const { agents, loading, error } = useAgentList();

  // Auto-exit after render
  useEffect(() => {
    if (!loading) {
      const timer = setTimeout(() => exit(), 100);
      return () => clearTimeout(timer);
    }
    return undefined;
  }, [loading, exit]);

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

  // Render table
  return (
    <Box flexDirection="column" paddingY={1}>
      <Text bold>Deployed Agents ({agents.length})</Text>
      <Box marginTop={1} />
      <Table<AgentListItem>
        columns={columns}
        data={agents}
        renderCell={(agent, column) => {
          switch (column.key) {
            case 'name':
              return agent.name;
            case 'runtime':
              return agent.runtime;
            case 'status':
              return <StatusBadge status={(agent.status as BadgeStatus) || 'deployed'} />;
            default:
              return '';
          }
        }}
      />
    </Box>
  );
};

export default AgentList;
