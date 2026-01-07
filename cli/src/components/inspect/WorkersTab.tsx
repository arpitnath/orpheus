import React from 'react';
import { Box, Text } from 'ink';
import { WorkerDots } from '../common/index.js';
import type { AgentDetails, AgentStats } from '../../types/index.js';

interface WorkersTabProps {
  agent: AgentDetails;
  agentStats?: AgentStats;
}

export const WorkersTab: React.FC<WorkersTabProps> = ({ agent, agentStats }) => {
  const busyWorkers = agentStats?.pool?.busy_workers ?? 0;
  const totalWorkers = agentStats?.pool?.total_workers ?? 0;
  const maxWorkers = agentStats?.pool?.desired_size ?? agent.scaling?.max_workers ?? 10;
  const idleWorkers = totalWorkers - busyWorkers;
  const queuePending = agentStats?.queue?.pending ?? 0;
  const queueMax = 50;

  return (
    <Box flexDirection="column">
      {/* Pool summary with worker dots */}
      <Box>
        <Text>Pool: </Text>
        <WorkerDots active={busyWorkers} total={maxWorkers} />
        <Text>  {busyWorkers}/{maxWorkers} active</Text>
      </Box>

      {/* Worker tree */}
      <Box flexDirection="column" marginTop={1}>
        {busyWorkers > 0 && (
          <Box>
            <Text dimColor>└─ </Text>
            <Text color="green">◉ {busyWorkers} busy</Text>
          </Box>
        )}
        {idleWorkers > 0 && (
          <Box>
            <Text dimColor>└─ </Text>
            <Text dimColor>○ {idleWorkers} idle</Text>
          </Box>
        )}
        {totalWorkers === 0 && (
          <Box>
            <Text dimColor>└─ </Text>
            <Text dimColor>No workers spawned</Text>
          </Box>
        )}
      </Box>

      {/* Queue */}
      <Box marginTop={1}>
        <Text>Queue: </Text>
        <Text color={queuePending > 0 ? 'yellow' : undefined}>
          {queuePending} pending
        </Text>
        <Text dimColor> (max: {queueMax})</Text>
      </Box>

      {/* Scaling info */}
      {agent.scaling && (
        <Box marginTop={1}>
          <Text dimColor>
            Scaling: min {agent.scaling.min_workers}, max {agent.scaling.max_workers}
          </Text>
        </Box>
      )}
    </Box>
  );
};
