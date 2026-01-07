import { useState, useEffect, useCallback } from 'react';
import { getStats } from '../lib/api.js';
import type { StatsResponse } from '../types/index.js';

export interface UseStatsResult {
  stats: StatsResponse | null;
  loading: boolean;
  error: string | null;
  refetch: () => void;
}

export function useStats(agentName?: string): UseStatsResult {
  const [stats, setStats] = useState<StatsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchStats = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await getStats(agentName);
      setStats(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to fetch stats');
      setStats(null);
    } finally {
      setLoading(false);
    }
  }, [agentName]);

  useEffect(() => {
    fetchStats();
  }, [fetchStats]);

  return { stats, loading, error, refetch: fetchStats };
}
