import React from 'react';
import { Box, Text } from 'ink';
import { Section, Row } from '../common/index.js';
import { formatRelativeTime } from '../../lib/format.js';
import type { AgentDetails } from '../../types/index.js';

interface OverviewTabProps {
  agent: AgentDetails;
  serverUrl: string;
}

export const OverviewTab: React.FC<OverviewTabProps> = ({ agent, serverUrl }) => {
  // Get memory value (try new field first, then legacy)
  const memory = agent.memory || agent.memory_mb;
  // Get timeout value (try new field first, then legacy)
  const timeout = agent.timeout || agent.timeout_seconds;

  // Check if we have model/engine config
  const hasModelConfig = agent.model || agent.engine;

  // Check if we have telemetry labels
  const hasTelemetryLabels = agent.telemetry?.labels && Object.keys(agent.telemetry.labels).length > 0;

  // Format scaling info
  const hasScaling = agent.scaling && (agent.scaling.min_workers || agent.scaling.max_workers);

  return (
    <Box flexDirection="column">
      {/* Configuration section */}
      <Section title="Configuration">
        <Row label="Runtime" value={agent.runtime} />
        <Row label="Module" value={agent.module} />
        <Row label="Entrypoint" value={agent.entrypoint} />
      </Section>

      {/* Resources section */}
      {(memory || timeout) && (
        <Section title="Resources">
          {memory && <Row label="Memory" value={`${memory} MB`} />}
          {timeout && <Row label="Timeout" value={`${timeout}s`} />}
        </Section>
      )}

      {/* Model section (ServiceManager integration) */}
      {hasModelConfig && (
        <Section title="Model">
          {agent.model && <Row label="Model" value={agent.model} />}
          {agent.engine && <Row label="Engine" value={agent.engine} />}
        </Section>
      )}

      {/* Scaling section */}
      {hasScaling && (
        <Section title="Scaling">
          <Row
            label="Workers"
            value={`${agent.scaling!.min_workers || 1} - ${agent.scaling!.max_workers || 10}`}
          />
          {agent.scaling!.queue_size && (
            <Row label="Queue Size" value={String(agent.scaling!.queue_size)} />
          )}
          {agent.scaling!.target_utilization && (
            <Row label="Target Util" value={`${agent.scaling!.target_utilization}`} />
          )}
        </Section>
      )}

      {/* Session affinity section */}
      {agent.session?.enabled && (
        <Section title="Session Affinity">
          <Row label="Header" value={agent.session.key} />
          <Row label="TTL" value={agent.session.ttl} />
          <Row label="Wait Timeout" value={agent.session.wait_timeout} />
        </Section>
      )}

      {/* Telemetry labels section */}
      {hasTelemetryLabels && (
        <Section title="Telemetry Labels">
          {Object.entries(agent.telemetry!.labels).map(([key, value]) => (
            <Row key={key} label={key} value={value} />
          ))}
        </Section>
      )}

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
      {(agent.deployed_at || agent.created_at) && (
        <Box marginTop={1}>
          <Row
            label="Deployed"
            value={formatRelativeTime(agent.deployed_at || agent.created_at!)}
          />
        </Box>
      )}
    </Box>
  );
};
