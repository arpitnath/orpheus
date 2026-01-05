import { useState, useEffect, useCallback } from 'react';
import { getHealth } from '../lib/api.js';
import type { HealthResponse } from '../types/index.js';

export interface UseHealthResult {
  health: HealthResponse | null;
  loading: boolean;
  error: string | null;
  refetch: () => void;
}

export function useHealth(): UseHealthResult {
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchHealth = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await getHealth();
      setHealth(data);
      if (!data) {
        setError('Daemon not responding');
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to fetch health');
      setHealth(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchHealth();
  }, [fetchHealth]);

  return { health, loading, error, refetch: fetchHealth };
}
