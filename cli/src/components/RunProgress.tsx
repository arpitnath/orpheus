//@RUN_PROGRESS
import React, { useState, useEffect, useRef } from 'react';
import { Box, Text, useApp } from 'ink';
import { Spinner } from './common/Spinner.js';

type Phase = 'connecting' | 'processing' | 'waiting' | 'completed' | 'error';

interface RunResult {
  success: boolean;
  output?: unknown;
  error?: string;
  duration_ms?: number;
}

interface RunProgressProps {
  agentName: string;
  onRun: () => Promise<RunResult>;
}

//@PHASE_LABELS
const PHASE_LABELS: Record<Phase, string> = {
  connecting: 'Connecting to agent',
  processing: 'Processing request',
  waiting: 'Waiting for response',
  completed: 'Completed',
  error: 'Failed',
};

//@COMPONENT
export const RunProgress: React.FC<RunProgressProps> = ({
  agentName,
  onRun,
}) => {
  const { exit } = useApp();
  const [phase, setPhase] = useState<Phase>('connecting');
  const [result, setResult] = useState<RunResult | null>(null);
  const [startTime] = useState<number>(Date.now());
  const [duration, setDuration] = useState<number>(0);
  const hasStarted = useRef(false);

  // Phase progression timers
  useEffect(() => {
    const timer1 = setTimeout(() => {
      if (phase === 'connecting') setPhase('processing');
    }, 400);

    const timer2 = setTimeout(() => {
      if (phase === 'processing') setPhase('waiting');
    }, 1200);

    return () => {
      clearTimeout(timer1);
      clearTimeout(timer2);
    };
  }, [phase]);

  // Run the agent
  useEffect(() => {
    if (hasStarted.current) return;
    hasStarted.current = true;

    const runAgent = async () => {
      try {
        const res = await onRun();
        const elapsed = Date.now() - startTime;
        setDuration(res.duration_ms || elapsed);
        setResult(res);
        setPhase(res.success ? 'completed' : 'error');

        // Exit after showing result
        setTimeout(() => exit(), 100);
      } catch (err) {
        const elapsed = Date.now() - startTime;
        setDuration(elapsed);
        setResult({
          success: false,
          error: err instanceof Error ? err.message : String(err),
        });
        setPhase('error');
        setTimeout(() => exit(), 100);
      }
    };

    runAgent();
  }, [onRun, exit, startTime]);

  // Render spinner phase
  if (phase !== 'completed' && phase !== 'error') {
    return (
      <Box flexDirection="column">
        <Box>
          <Text color="cyan">
            <Spinner />
          </Text>
          <Text> {PHASE_LABELS[phase]}...</Text>
          <Text dimColor> ({agentName})</Text>
        </Box>
      </Box>
    );
  }

  // Render completed state
  if (phase === 'completed' && result) {
    const durationSec = (duration / 1000).toFixed(2);
    return (
      <Box flexDirection="column">
        <Box>
          <Text color="green">✓</Text>
          <Text> Completed in </Text>
          <Text color="cyan">{durationSec}s</Text>
        </Box>
        <Box marginTop={1}>
          <Text>{JSON.stringify(result.output, null, 2)}</Text>
        </Box>
      </Box>
    );
  }

  // Render error state
  if (phase === 'error') {
    const durationSec = (duration / 1000).toFixed(2);
    return (
      <Box flexDirection="column">
        <Box>
          <Text color="red">✗</Text>
          <Text> Failed after </Text>
          <Text dimColor>{durationSec}s</Text>
        </Box>
        <Box marginTop={1}>
          <Text color="red">Error: {result?.error || 'Unknown error'}</Text>
        </Box>
      </Box>
    );
  }

  return null;
};
