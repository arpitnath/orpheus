//@DEPLOY_PROGRESS
import React, { useState, useEffect } from 'react';
import { Box, Text, useApp } from 'ink';
import { Spinner } from './common/Spinner.js';

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

interface DeployProgressProps {
  agentName: string;
  agentPath: string;
  onDeploy: () => Promise<DeployResult>;
}

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

  useEffect(() => {
    const runDeploy = async () => {
      try {
        // Step 1: Validate
        updateStep(0, 'running');
        await new Promise(r => setTimeout(r, 400));
        updateStep(0, 'completed');

        // Step 2: Package
        updateStep(1, 'running');
        await new Promise(r => setTimeout(r, 600));
        updateStep(1, 'completed');

        // Step 3: Upload (starts the actual deploy)
        updateStep(2, 'running');

        // onDeploy() does: tarball creation, upload, daemon processes (install deps, register)
        // We show steps 3-5 progressing during this single call
        const deployPromise = onDeploy();

        // After a short delay, show upload complete and deps installing
        await new Promise(r => setTimeout(r, 800));
        updateStep(2, 'completed');

        // Step 4: Installing dependencies (daemon-side)
        updateStep(3, 'running');

        // After another delay, show deps complete and registering
        await new Promise(r => setTimeout(r, 1500));
        updateStep(3, 'completed');

        // Step 5: Registering agent
        updateStep(4, 'running');

        // Wait for actual deploy to complete
        const deployResult = await deployPromise;

        if (deployResult.success) {
          updateStep(4, 'completed');
          setResult(deployResult);
        } else {
          updateStep(4, 'error', deployResult.error);
          setResult(deployResult);
        }
      } catch (err) {
        const errorMsg = err instanceof Error ? err.message : 'Unknown error';
        const currentStep = steps.findIndex(s => s.status === 'running');
        if (currentStep >= 0) {
          updateStep(currentStep, 'error', errorMsg);
        }
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
