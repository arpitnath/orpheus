import React, { useEffect, useState } from 'react';
import { Box, Text, useApp } from 'ink';
import { createClient } from '../lib/api.js';
import { Spinner, ErrorBox, Table } from './common/index.js';
import type { Column } from './common/index.js';
import type { CrashedRequest } from '../types/index.js';

const columns: Column[] = [
  { key: 'request_id', header: 'REQUEST ID', width: 20 },
  { key: 'agent_name', header: 'AGENT', width: 20 },
  { key: 'worker_id', header: 'WORKER', width: 12 },
  { key: 'started_at', header: 'STARTED AT', width: 20 },
  { key: 'session_id', header: 'SESSION', width: 16 },
];

const EmptyState: React.FC = () => (
  <Box flexDirection="column" paddingY={1}>
    <Text bold>Crashed Requests (0)</Text>
    <Box marginTop={1} />
    <Text dimColor>No crashed requests found.</Text>
  </Box>
);

export const ExecLogCrashed: React.FC = () => {
  const { exit } = useApp();
  const [requests, setRequests] = useState<CrashedRequest[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const client = createClient();

    client
      .getCrashedRequests()
      .then((data) => {
        setRequests(data.crashed_requests || []);
        setLoading(false);
      })
      .catch((err) => {
        setError(err.message);
        setLoading(false);
      });

    const timer = setTimeout(() => exit(), 100);
    return () => clearTimeout(timer);
  }, [exit]);

  if (loading) {
    return (
      <Box>
        <Spinner label="Loading crashed requests..." />
      </Box>
    );
  }

  if (error) {
    return <ErrorBox message={error} hint="Check if daemon is running" />;
  }

  if (requests.length === 0) {
    return <EmptyState />;
  }

  return (
    <Box flexDirection="column" paddingY={1}>
      <Text bold>Crashed Requests ({requests.length})</Text>
      <Box marginTop={1} />
      <Table<CrashedRequest>
        columns={columns}
        data={requests}
        renderCell={(req, col) => {
          switch (col.key) {
            case 'request_id':
              return req.request_id.substring(0, 12) + '...';
            case 'agent_name':
              return req.agent_name;
            case 'worker_id':
              return req.worker_id;
            case 'started_at':
              return new Date(req.started_at).toLocaleString();
            case 'session_id':
              return req.session_id || '-';
            default:
              return '';
          }
        }}
      />
      <Box marginTop={1}>
        <Text dimColor>Tip: Review for side effects before retrying.</Text>
      </Box>
    </Box>
  );
};

export default ExecLogCrashed;
