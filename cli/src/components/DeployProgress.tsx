//@DEPLOY_PROGRESS
import React, { useState, useEffect, useRef } from 'react';
import { Box, Text, useApp } from 'ink';
import { Spinner } from './common/Spinner.js';
import type { DeployProgressEvent } from '../lib/deploy.js';

type StepStatus = 'pending' | 'running' | 'completed' | 'error';

interface Step {
  name: string;
  status: StepStatus;
  error?: string;
}

interface DependencyInfo {
  installed: boolean;
  runtime: string;
  source?: string;
}

interface DeployResult {
  success: boolean;
  endpoints?: {
    http: string;
    mcp?: string;
  };
  dependencies?: DependencyInfo;
  error?: string;
}

// Callback type for SSE-enabled deploy
type DeployWithProgressFn = (
  onProgress: (event: DeployProgressEvent) => void
) => Promise<DeployResult>;

interface DeployProgressProps {
  agentName: string;
  agentPath: string;
  onDeploy: DeployWithProgressFn;
}

// Map daemon phases to step indices
const PHASE_TO_STEP: Record<string, number> = {
  extracting: 1,   // Packaging agent files (extraction on daemon)
  validating: 1,   // Still in packaging phase
  copying: 2,      // Uploading to daemon (copying base image)
  installing: 3,   // Installing dependencies
  registering: 4,  // Registering agent
};

//@STEP_ICON
const StepIcon: React.FC<{ status: StepStatus }> = ({ status }) => {
  switch (status) {
    case 'completed':
      return <Text color="green">✓</Text>;
    case 'running':
      return <Spinner />;
    case 'error':
      return <Text color="red">✗</Text>;
    default:
      return <Text dimColor>○</Text>;
  }
};

//@COMPONENT
export const DeployProgress: React.FC<DeployProgressProps> = ({
  agentName,
  agentPath,
  onDeploy,
}) => {
  const { exit } = useApp();
  const [steps, setSteps] = useState<Step[]>([
    { name: 'Validating agent.yaml', status: 'pending' },
    { name: 'Packaging agent files', status: 'pending' },
    { name: 'Uploading to daemon', status: 'pending' },
    { name: 'Installing dependencies', status: 'pending' },
    { name: 'Registering agent', status: 'pending' },
  ]);
  const [result, setResult] = useState<DeployResult | null>(null);

  const updateStep = (index: number, status: StepStatus, error?: string) => {
    setSteps(prev => {
      const next = [...prev];
      next[index] = { ...next[index], status, error };
      return next;
    });
  };

  // Track the last completed step to avoid re-running transitions
  const lastCompletedStep = useRef(-1);

  useEffect(() => {
    const runDeploy = async () => {
      try {
        // Step 0: Validate (CLI-side, quick)
        updateStep(0, 'running');
        await new Promise(r => setTimeout(r, 100)); // Minimal delay for UI
        updateStep(0, 'completed');
        lastCompletedStep.current = 0;

        // Step 1: Package (CLI-side, quick)
        updateStep(1, 'running');

        // Handle progress events from daemon
        const handleProgress = (event: DeployProgressEvent) => {
          const stepIndex = PHASE_TO_STEP[event.phase];
          if (stepIndex === undefined) return;

          // Complete all previous steps
          for (let i = 0; i <= stepIndex - 1; i++) {
            if (i > lastCompletedStep.current) {
              updateStep(i, 'completed');
              lastCompletedStep.current = i;
            }
          }

          // Mark current step as running
          updateStep(stepIndex, 'running');
        };

        // Call deploy with progress callback
        const deployResult = await onDeploy(handleProgress);

        // Mark all remaining steps as completed on success
        if (deployResult.success) {
          for (let i = 0; i < 5; i++) {
            updateStep(i, 'completed');
          }
          setResult(deployResult);
        } else {
          // Find current running step and mark as error
          const currentStep = steps.findIndex(s => s.status === 'running');
          if (currentStep >= 0) {
            updateStep(currentStep, 'error', deployResult.error);
          }
          setResult(deployResult);
        }
      } catch (err) {
        const errorMsg = err instanceof Error ? err.message : 'Unknown error';
        // Find current running step and mark as error
        setSteps(prev => {
          const next = [...prev];
          const runningIdx = next.findIndex(s => s.status === 'running');
          if (runningIdx >= 0) {
            next[runningIdx] = { ...next[runningIdx], status: 'error', error: errorMsg };
          }
          return next;
        });
        setResult({ success: false, error: errorMsg });
      }

      // Exit after showing result
      setTimeout(() => exit(), 2000);
    };

    runDeploy();
  }, []);

  return (
    <Box flexDirection="column" padding={1}>
      <Text bold>Deploying: {agentName}</Text>
      <Text dimColor>{agentPath}</Text>

      <Box marginTop={1} flexDirection="column">
        {steps.map((step, i) => (
          <Box key={i}>
            <Text>  </Text>
            <StepIcon status={step.status} />
            <Text> {step.name}</Text>
            {step.error && <Text color="red"> - {step.error}</Text>}
          </Box>
        ))}
      </Box>

      {result && (
        <Box marginTop={1} flexDirection="column">
          <Text>{'─'.repeat(40)}</Text>
          {result.success ? (
            <>
              <Text color="green">✓ Agent deployed successfully!</Text>
              {result.dependencies?.installed && (
                <Text dimColor>  Dependencies installed from {result.dependencies.source}</Text>
              )}
              {result.endpoints && (
                <Box marginTop={1} flexDirection="column">
                  <Text>  HTTP: {result.endpoints.http}</Text>
                  {result.endpoints.mcp && (
                    <Text>  MCP:  {result.endpoints.mcp}</Text>
                  )}
                </Box>
              )}
            </>
          ) : (
            <Text color="red">✗ Deployment failed: {result.error}</Text>
          )}
        </Box>
      )}
    </Box>
  );
};
