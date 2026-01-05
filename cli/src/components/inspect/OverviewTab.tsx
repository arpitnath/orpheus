import React from 'react';
import { Box, Text } from 'ink';
import { Section, Row } from '../common/index.js';
import { formatRelativeTime } from '../../lib/format.js';
import type { AgentDetails } from '../../types/index.js';

interface OverviewTabProps {
  agent: AgentDetails;
  serverUrl: string;
}

export const OverviewTab: React.FC<OverviewTabProps> = ({ agent, serverUrl }) => (
  <Box flexDirection="column">
    {/* Configuration section */}
    <Section title="Configuration">
      <Row label="Runtime" value={agent.runtime} />
      <Row label="Module" value={agent.module} />
      <Row label="Entrypoint" value={agent.entrypoint} />
    </Section>

    {/* Endpoints section */}
    <Section title="Endpoints">
      <Row
        label="HTTP"
        value={<Text color="cyan">{serverUrl}/v1/agents/{agent.name}</Text>}
      />
      <Row
        label="MCP"
        value={<Text color="cyan">{serverUrl}/v1/mcp/{agent.name}</Text>}
      />
    </Section>

    {/* Metadata */}
    {agent.deployed_at && (
      <Box marginTop={1}>
        <Row label="Deployed" value={formatRelativeTime(agent.deployed_at)} />
      </Box>
    )}
  </Box>
);
