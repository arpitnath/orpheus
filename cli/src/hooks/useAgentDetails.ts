import { useState, useEffect, useCallback } from 'react';
import { createClient } from '../lib/api.js';
import type { AgentDetails } from '../types/index.js';

export interface UseAgentDetailsResult {
  agent: AgentDetails | null;
  loading: boolean;
  error: string | null;
  refetch: () => void;
}

export function useAgentDetails(name: string): UseAgentDetailsResult {
  const [agent, setAgent] = useState<AgentDetails | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchAgent = useCallback(async () => {
    if (!name) {
      setError('Agent name is required');
      setLoading(false);
      return;
    }

    setLoading(true);
    setError(null);
    try {
      const client = createClient();
      const data = await client.inspect(name);
      setAgent(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to fetch agent details');
      setAgent(null);
    } finally {
      setLoading(false);
    }
  }, [name]);

  useEffect(() => {
    fetchAgent();
  }, [fetchAgent]);

  return { agent, loading, error, refetch: fetchAgent };
}
