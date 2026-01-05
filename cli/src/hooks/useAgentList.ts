import { useState, useEffect, useCallback } from 'react';
import { createClient } from '../lib/api.js';
import type { AgentListItem } from '../types/index.js';

export interface UseAgentListResult {
  agents: AgentListItem[];
  loading: boolean;
  error: string | null;
  refetch: () => void;
}

export function useAgentList(): UseAgentListResult {
  const [agents, setAgents] = useState<AgentListItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchAgents = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const client = createClient();
      const data = await client.list();
      setAgents(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to fetch agents');
      setAgents([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchAgents();
  }, [fetchAgents]);

  return { agents, loading, error, refetch: fetchAgents };
}
