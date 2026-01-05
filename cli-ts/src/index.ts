#!/usr/bin/env node

import React from 'react';
import { Command } from 'commander';
import { createClient, testConnection, getHealth, getStats } from './lib/api.js';
import {
  getActiveServerName,
  getActiveServer,
  getDefaultSocketPath,
  socketExists,
  listServers,
  addServer,
  removeServer,
  setActiveServer,
} from './lib/config.js';
import {
  createTarball,
  calculateChecksum,
  validateAgentPath,
  uploadAgentWithSSE,
  type DeployProgressEvent,
} from './lib/deploy.js';
import {
  checkMacOS,
  checkLimaInstalled,
  getVMStatus,
  startVM,
  stopVM,
  deleteVM,
  sshVM,
} from './lib/vm.js';
import { renderApp } from './lib/render.js';
import { DeployProgress } from './components/DeployProgress.js';
import { c, sym, ok, err as fmtErr, warn, info, label, table, box, statusDot } from './lib/format.js';

//@VERSION
const VERSION = '0.1.0';

//@PROGRAM
const program = new Command();

program
  .name('orpheus')
  .description('Orpheus CLI - Infrastructure for AI agents')
  .version(VERSION, '-v, --version', 'Show version number');

//@CORE_COMMANDS

program
  .command('status')
  .description('Show system status and health')
  .action(async () => {
    const lines: string[] = [];
    const activeServer = getActiveServerName();
    lines.push(label('Server', activeServer));

    if (!socketExists()) {
      lines.push(label('Daemon', `${c.red}not running${c.reset}`));
      lines.push(label('Socket', getDefaultSocketPath()));
      console.log(box('Orpheus Status', lines.join('\n')));
      return;
    }

    const health = await getHealth();
    if (health) {
      lines.push(label('Daemon', `${statusDot(health.status)} ${health.status}`));
      lines.push(label('Uptime', formatUptime(health.uptime_seconds)));

      await new Promise(resolve => setTimeout(resolve, 50));
      const stats = await getStats();
      if (stats && stats.global) {
        lines.push('');
        lines.push(label('Agents', `${stats.global.total_agents} deployed`));
        lines.push(label('Workers', `${stats.global.total_workers} total`));
        lines.push(label('Pending', `${stats.global.total_pending} requests`));
      }
    } else {
      lines.push(label('Daemon', `${c.red}not responding${c.reset}`));
    }

    console.log(box('Orpheus Status', lines.join('\n')));
  });

program
  .command('deploy <path>')
  .description('Deploy an agent to the configured server')
  .option('-f, --force', 'Overwrite existing agent')
  .option('-e, --env <vars...>', 'Environment variables (KEY=VALUE)')
  .action(async (agentPath: string, options: { force?: boolean; env?: string[] }) => {
    const path = await import('node:path');

    // Extract agent name from path
    const resolvedPath = path.resolve(agentPath);
    const agentName = path.basename(resolvedPath);

    // Validate agent path upfront
    const validation = validateAgentPath(resolvedPath);
    if (!validation.valid) {
      console.error(`\x1b[31mError:\x1b[0m ${validation.error}`);
      process.exit(1);
    }

    // TUI deploy with SSE progress streaming
    const onDeploy = async (onProgress: (event: DeployProgressEvent) => void) => {
      try {
        const serverConfig = getActiveServer();

        // Create tarball and checksum
        const tarball = await createTarball(resolvedPath);
        const checksum = calculateChecksum(tarball);

        // Deploy with SSE progress streaming
        const result = await uploadAgentWithSSE(
          serverConfig,
          agentName,
          tarball,
          checksum,
          options.env,
          onProgress
        );

        return {
          success: result.status === 'deployed',
          endpoints: result.endpoints,
          dependencies: result.dependencies,
          error: result.message,
        };
      } catch (err) {
        return {
          success: false,
          error: err instanceof Error ? err.message : 'Unknown error',
        };
      }
    };

    renderApp(
      React.createElement(DeployProgress, {
        agentName,
        agentPath: resolvedPath,
        onDeploy,
      })
    );
  });

program
  .command('run <agent>')
  .description('Execute a deployed agent')
  .argument('[input]', 'Input JSON')
  .action(async (agentName: string, inputArg?: string) => {
    try {
      const client = createClient();
      const input = inputArg ? JSON.parse(inputArg) : {};
      const result = await client.invoke(agentName, input);

      // Handle daemon response format: {status, output, duration_ms}
      const isSuccess = result.status === 'success' || result.success;
      const output = result.output ?? result.result;

      if (isSuccess) {
        console.log(JSON.stringify(output, null, 2));
      } else {
        const errorMsg = result.error || (typeof output === 'object' && output && 'error' in output ? (output as {error: string}).error : 'Execution failed');
        console.error(`\x1b[31mError:\x1b[0m ${errorMsg}`);
        process.exit(1);
      }
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

program
  .command('invoke <agent> [input]')
  .description('Invoke a deployed agent')
  .action(async (agent: string, input?: string) => {
    try {
      const client = createClient();
      const parsedInput = input ? JSON.parse(input) : {};
      console.log(`Invoking ${agent}...`);
      const result = await client.invoke(agent, parsedInput);
      console.log('\nResult:');
      console.log(JSON.stringify(result, null, 2));
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

program
  .command('undeploy <agent>')
  .description('Remove a deployed agent')
  .action(async (agent: string) => {
    try {
      const client = createClient();
      console.log(`Undeploying ${agent}...`);
      const result = await client.undeploy(agent);
      // Daemon returns { message: "agent undeployed", name: "..." } on success
      if (result.message?.includes('undeployed')) {
        console.log(`\x1b[32m✓\x1b[0m Agent ${agent} undeployed`);
      } else {
        console.log(`\x1b[31m✗\x1b[0m ${result.message || 'Failed to undeploy'}`);
      }
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

program
  .command('validate <path>')
  .description('Validate an agent.yaml file')
  .action(async (agentPath: string) => {
    const { validateAgentYaml } = await import('./lib/validate.js');
    const result = validateAgentYaml(agentPath);

    if (result.valid) {
      console.log(ok('Valid agent.yaml'));
      if (result.config) {
        console.log(`  ${label('Name', result.config.name, 14)}`);
        console.log(`  ${label('Runtime', result.config.runtime, 14)}`);
        console.log(`  ${label('Module', result.config.module, 14)}`);
        console.log(`  ${label('Entrypoint', result.config.entrypoint, 14)}`);
        if (result.config.scaling) {
          console.log(`  ${label('Scaling', `${result.config.scaling.min_workers}-${result.config.scaling.max_workers} workers`, 14)}`);
        }
      }
    } else {
      console.log(fmtErr('Invalid agent.yaml'));
      for (const error of result.errors) {
        console.log(`  ${c.red}${sym.bullet}${c.reset} ${error}`);
      }
    }

    if (result.warnings.length > 0) {
      console.log('\nWarnings:');
      for (const warning of result.warnings) {
        console.log(`  ${c.yellow}${sym.bullet}${c.reset} ${warning}`);
      }
    }

    if (!result.valid) {
      process.exit(1);
    }
  });

//@OBSERVABILITY_COMMANDS

program
  .command('logs')
  .description('View daemon logs')
  .argument('[agent]', 'Filter by agent name')
  .option('-f, --follow', 'Follow log output')
  .option('-n, --tail <lines>', 'Number of lines to show', '50')
  .option('--grep <pattern>', 'Filter by pattern')
  .action(async (agentName: string | undefined, options: { follow?: boolean; tail?: string; grep?: string }) => {
    const { execSync } = await import('node:child_process');
    const { platform } = await import('node:os');
    const logPath = '/var/log/orpheusd.log';
    const tailNum = parseInt(options.tail || '50', 10);

    try {
      let cmd: string;
      if (platform() === 'darwin') {
        // macOS - read via Lima VM
        cmd = `limactl shell orpheus -- cat ${logPath} 2>/dev/null || echo ""`;
      } else {
        // Linux - read directly
        cmd = `cat ${logPath} 2>/dev/null || echo ""`;
      }

      let output = execSync(cmd, { encoding: 'utf-8' });

      // Filter by agent if specified
      if (agentName) {
        output = output.split('\n').filter(l => l.includes(agentName)).join('\n');
      }

      // Filter by grep pattern
      if (options.grep) {
        output = output.split('\n').filter(l => l.includes(options.grep!)).join('\n');
      }

      // Tail to last N lines
      const lines = output.split('\n').filter(l => l.trim());
      output = lines.slice(-tailNum).join('\n');

      if (!output.trim()) {
        console.log('No logs found');
        return;
      }

      console.log(output);

      // Follow mode - poll for new logs
      if (options.follow) {
        console.log('\n\x1b[2m(following logs, Ctrl+C to exit)\x1b[0m\n');
        let lastLength = lines.length;
        setInterval(async () => {
          try {
            const newOutput = execSync(cmd, { encoding: 'utf-8' });
            const newLines = newOutput.split('\n').filter(l => l.trim());
            if (newLines.length > lastLength) {
              const diff = newLines.slice(lastLength);
              diff.forEach(line => console.log(line));
              lastLength = newLines.length;
            }
          } catch {
            // Ignore errors in follow mode
          }
        }, 1000);
      }
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

program
  .command('list')
  .description('List deployed agents')
  .option('--images', 'Show base images instead')
  .action(async (options: { images?: boolean }) => {
    if (options.images) {
      console.log('Base images:');
      console.log(`\n${c.yellow}Images listing not yet implemented${c.reset}`);
      return;
    }

    try {
      const client = createClient();
      const agents = await client.list();

      if (agents.length === 0) {
        console.log(info('No agents deployed'));
        return;
      }

      console.log(`${c.bold}Deployed Agents${c.reset}\n`);
      const rows = agents.map(agent => {
        const status = agent.status || 'deployed';
        return [
          agent.name || 'unknown',
          agent.runtime || `${c.dim}unknown${c.reset}`,
          `${statusDot(status)} ${status}`,
        ];
      });
      console.log(table(['NAME', 'RUNTIME', 'STATUS'], rows, [24, 14, 16]));
    } catch (err) {
      console.error(`${c.red}Error:${c.reset} ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

program
  .command('stats')
  .description('Show pool statistics')
  .argument('[agent]', 'Optional agent name filter')
  .action(async (agent?: string) => {
    try {
      const stats = await getStats(agent);
      if (!stats) {
        console.error(`${c.red}Error:${c.reset} Cannot connect to daemon`);
        process.exit(1);
      }

      // Defensive: check if global stats exist
      if (!stats.global) {
        console.error(`${c.red}Error:${c.reset} Invalid response from daemon (missing global stats)`);
        process.exit(1);
      }

      // Global stats with safe defaults
      const g = stats.global;
      const globalLines = [
        label('Agents', String(g.total_agents ?? 0)),
        label('Workers', String(g.total_workers ?? 0)),
        label('Pending', String(g.total_pending ?? 0)),
        label('Processing', String(g.total_processing ?? 0)),
        label('Utilization', `${(g.average_utilization ?? 0).toFixed(1)}%`),
      ];
      console.log(box('Pool Statistics', globalLines.join('\n')));

      if (stats.agents && stats.agents.length > 0) {
        // Separate agents with pools from those without
        const agentsWithPools = stats.agents.filter(a => a.pool && a.queue && a.derived);
        const agentsWithoutPools = stats.agents.filter(a => !a.pool || !a.queue || !a.derived);

        if (agentsWithPools.length > 0) {
          console.log(`\n${c.bold}Per-Agent Stats${c.reset}\n`);
          const rows = agentsWithPools.map(a => [
            a.agent_name.length > 22 ? a.agent_name.slice(0, 19) + '...' : a.agent_name,
            String(a.pool!.total_workers),
            String(a.queue!.pending),
            String(a.queue!.processing),
            `${a.derived!.utilization_percentage.toFixed(1)}%`,
          ]);
          console.log(table(['AGENT', 'WORKERS', 'PENDING', 'PROC', 'UTIL'], rows, [24, 10, 10, 8, 8]));
        }

        // Show agents without pools (legacy agents)
        if (agentsWithoutPools.length > 0) {
          console.log(`\n${c.dim}Agents without pools (legacy):${c.reset}`);
          for (const a of agentsWithoutPools) {
            const status = a.pool_status || 'not_available';
            console.log(`  ${c.dim}•${c.reset} ${a.agent_name} ${c.dim}(${status})${c.reset}`);
          }
        }
      }
    } catch (err) {
      console.error(`${c.red}Error:${c.reset} ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

program
  .command('inspect <agent>')
  .description('Show agent details')
  .option('-f, --format <format>', 'Output format (json, yaml, text)', 'text')
  .action(async (agentName: string, options: { format?: string }) => {
    try {
      const client = createClient();
      const details = await client.inspect(agentName);

      if (options.format === 'json') {
        console.log(JSON.stringify(details, null, 2));
        return;
      }

      // Text format (default) - box layout
      const lines: string[] = [];
      lines.push(label('Runtime', details.runtime));
      lines.push(label('Module', details.module));
      lines.push(label('Entrypoint', details.entrypoint));
      lines.push('');

      if (details.endpoints?.http) {
        lines.push(label('HTTP', details.endpoints.http));
        if (details.endpoints.mcp) {
          lines.push(label('MCP', details.endpoints.mcp));
        }
      } else {
        lines.push(`${c.dim}Endpoints not available${c.reset}`);
      }
      lines.push('');

      const scalingInfo = details.scaling
        ? `${details.workers} (min: ${details.scaling.min_workers}, max: ${details.scaling.max_workers})`
        : `${details.workers}`;
      lines.push(label('Workers', scalingInfo));
      lines.push(label('Status', `${statusDot(details.status)} ${details.status}`));
      if (details.deployed_at) {
        lines.push(label('Deployed', details.deployed_at));
      }

      console.log(box(details.name, lines.join('\n')));
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

program
  .command('ps')
  .description('Show running agents and workers')
  .option('-a, --all', 'Show all agents including idle')
  .action(async (_options: { all?: boolean }) => {
    try {
      const client = createClient();
      const agents = await client.list();

      if (agents.length === 0) {
        console.log(info('No agents deployed'));
        return;
      }

      const rows = agents.map(agent => {
        const status = agent.status || 'deployed';
        return [
          agent.name || 'unknown',
          agent.runtime || `${c.dim}unknown${c.reset}`,
          `${statusDot(status)} ${status}`,
        ];
      });
      console.log(table(['NAME', 'RUNTIME', 'STATUS'], rows, [24, 14, 16]));
    } catch (err) {
      console.error(`${c.red}Error:${c.reset} ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

program
  .command('runs')
  .description('Show execution history')
  .argument('[agent]', 'Filter by agent name')
  .option('-n, --limit <count>', 'Number of runs to show', '20')
  .action(async (agentName: string | undefined, options: { limit?: string }) => {
    const { execSync } = await import('node:child_process');
    const { platform } = await import('node:os');
    const logPath = '/var/log/orpheusd.log';
    const limit = parseInt(options.limit || '20', 10);

    try {
      let cmd: string;
      if (platform() === 'darwin') {
        // macOS - grep execution lines via Lima VM
        cmd = `limactl shell orpheus -- grep -E "Executing|execution completed|execution failed" ${logPath} 2>/dev/null || echo ""`;
      } else {
        // Linux - grep directly
        cmd = `grep -E "Executing|execution completed|execution failed" ${logPath} 2>/dev/null || echo ""`;
      }

      let output = execSync(cmd, { encoding: 'utf-8' });

      // Filter by agent if specified
      if (agentName) {
        output = output.split('\n').filter(l => l.includes(agentName)).join('\n');
      }

      const lines = output.split('\n').filter(l => l.trim());
      const limitedLines = lines.slice(-limit);

      if (limitedLines.length === 0) {
        console.log('No execution history found');
        return;
      }

      console.log('Recent Executions:\n');
      console.log('─'.repeat(70));
      for (const line of limitedLines) {
        // Color-code based on content
        if (line.includes('failed')) {
          console.log(`\x1b[31m${line}\x1b[0m`);
        } else if (line.includes('completed')) {
          console.log(`\x1b[32m${line}\x1b[0m`);
        } else {
          console.log(line);
        }
      }
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

program
  .command('healthcheck')
  .description('Run system health diagnostics')
  .option('--fix', 'Attempt to fix issues')
  .action(async (_options: { fix?: boolean }) => {
    console.log(`${c.bold}Health Checks${c.reset}\n`);

    let allPassed = true;
    const { platform } = await import('node:os');
    const { execSync } = await import('node:child_process');

    // Check 1: Config exists
    const configValid = socketExists() || getActiveServerName() !== 'local';
    if (configValid) {
      console.log(ok('Config valid'));
    } else {
      console.log(warn('Config: using defaults'));
    }

    // Check 2: Daemon reachable
    const connected = await testConnection();
    if (connected) {
      console.log(ok('Daemon reachable'));
    } else {
      console.log(fmtErr('Daemon not reachable'));
      allPassed = false;
    }

    // Check 3: Daemon health
    if (connected) {
      const health = await getHealth();
      if (health && health.status === 'healthy') {
        console.log(ok(`Daemon healthy (uptime: ${formatUptime(health.uptime_seconds)})`));
      } else if (health) {
        console.log(warn(`Daemon ${health.status}`));
        allPassed = false;
      } else {
        console.log(fmtErr('Daemon health check failed'));
        allPassed = false;
      }
    }

    // Check 4: Lima VM (macOS only)
    if (platform() === 'darwin') {
      try {
        const output = execSync('limactl list --json 2>/dev/null', { encoding: 'utf-8' });
        const parsed = JSON.parse(output);
        const vms = Array.isArray(parsed) ? parsed : [parsed];
        const orpheusVm = vms.find((vm: { name: string }) => vm.name === 'orpheus');
        if (orpheusVm && orpheusVm.status === 'Running') {
          console.log(ok('Lima VM running'));
        } else if (orpheusVm) {
          console.log(warn(`Lima VM ${orpheusVm.status.toLowerCase()}`));
        } else {
          console.log(warn('Lima VM not found'));
        }
      } catch {
        console.log(warn('Lima not installed or not configured'));
      }
    }

    console.log('');
    if (allPassed) {
      console.log(`${c.green}All checks passed!${c.reset}`);
    } else {
      console.log(`${c.red}Some checks failed${c.reset}`);
      process.exit(1);
    }
  });

//@UTILITY_COMMANDS

program
  .command('test <agent>')
  .description('Test an agent with timing')
  .argument('[input]', 'Test input JSON')
  .option('--verbose', 'Verbose output')
  .action(async (agentName: string, inputArg?: string, options?: { verbose?: boolean }) => {
    try {
      const client = createClient();
      const input = inputArg ? JSON.parse(inputArg) : {};

      if (options?.verbose) {
        console.log(`\x1b[36mAgent:\x1b[0m ${agentName}`);
        console.log(`\x1b[36mInput:\x1b[0m ${JSON.stringify(input)}`);
        console.log('');
      }

      console.log('Executing...');
      const startTime = Date.now();
      const result = await client.invoke(agentName, input);
      const elapsed = Date.now() - startTime;

      // Use daemon duration if available, else client-side measurement
      const duration = result.duration_ms ?? elapsed;
      console.log(`\x1b[36mExecution time:\x1b[0m ${duration}ms\n`);

      // Handle daemon response format: {status, output, duration_ms}
      const isSuccess = result.status === 'success' || result.success;
      const output = result.output ?? result.result;

      if (isSuccess) {
        console.log('\x1b[32m✓\x1b[0m Result:');
        console.log(JSON.stringify(output, null, 2));
      } else {
        const errorMsg = result.error || (typeof output === 'object' && output && 'error' in output ? (output as {error: string}).error : 'Execution failed');
        console.error(`\x1b[31m✗\x1b[0m Error: ${errorMsg}`);
        process.exit(1);
      }
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

program
  .command('shell')
  .description('Start interactive shell (macOS: VM shell)')
  .action(async () => {
    const { spawn } = await import('node:child_process');
    const { platform } = await import('node:os');

    if (platform() !== 'darwin') {
      console.error('\x1b[31mError:\x1b[0m shell command only available on macOS (Lima VM)');
      console.log('On Linux, use: ssh user@server or orpheus exec <command>');
      process.exit(1);
    }

    console.log('Connecting to Orpheus VM...\n');
    const shell = spawn('limactl', ['shell', 'orpheus'], { stdio: 'inherit' });
    shell.on('exit', code => process.exit(code || 0));
  });

program
  .command('exec <command...>')
  .description('Execute a command in daemon context')
  .action(async (command: string[]) => {
    const { execSync } = await import('node:child_process');
    const { platform } = await import('node:os');
    const cmd = command.join(' ');

    try {
      let output: string;
      if (platform() === 'darwin') {
        // macOS - execute in Lima VM
        output = execSync(`limactl shell orpheus -- ${cmd}`, { encoding: 'utf-8' });
      } else {
        // Linux - execute directly
        output = execSync(cmd, { encoding: 'utf-8' });
      }
      console.log(output);
    } catch (err: unknown) {
      const error = err as { stdout?: string; stderr?: string; message?: string };
      if (error.stdout) console.log(error.stdout);
      if (error.stderr) console.error(error.stderr);
      console.error(`\x1b[31mError:\x1b[0m ${error.message || 'Command failed'}`);
      process.exit(1);
    }
  });

//@LOGIN_COMMANDS
const loginCommand = program.command('login').description('Manage server connections');

loginCommand
  .command('list', { isDefault: true })
  .description('List configured servers')
  .action(async () => {
    const servers = listServers();
    const active = getActiveServerName();

    console.log(`${c.bold}Configured Servers${c.reset}\n`);
    for (const [name, config] of Object.entries(servers)) {
      const marker = name === active ? `${c.green}${sym.arrow}${c.reset}` : ' ';
      const mode = config.mode === 'unix_socket' ? `${c.dim}socket${c.reset}` : `${c.dim}tcp${c.reset}`;
      const endpoint = config.mode === 'unix_socket' ? config.socket_path : config.url;
      console.log(`${marker} ${name} (${mode}): ${endpoint}`);
    }
  });

loginCommand
  .command('add <name> <url>')
  .description('Add a new server')
  .option('-k, --key <key>', 'API key for authentication')
  .action(async (name: string, url: string, options: { key?: string }) => {
    try {
      addServer(name, url, options.key);
      console.log(ok(`Added server: ${name}`));
      console.log(`  ${label('URL', url)}`);
      if (options.key) {
        console.log(`  ${label('Auth', 'configured')}`);
      }
    } catch (err) {
      console.error(`${c.red}Error:${c.reset} ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

loginCommand
  .command('use <name>')
  .description('Set active server')
  .action(async (name: string) => {
    try {
      setActiveServer(name);
      console.log(ok(`Active server set to: ${name}`));
    } catch (err) {
      console.error(`${c.red}Error:${c.reset} ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

loginCommand
  .command('remove <name>')
  .description('Remove a server')
  .action(async (name: string) => {
    try {
      removeServer(name);
      console.log(ok(`Removed server: ${name}`));
    } catch (err) {
      console.error(`${c.red}Error:${c.reset} ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

//@VM_COMMANDS
const vmCommand = program.command('vm').description('Lima VM management (macOS)');

vmCommand
  .command('start')
  .description('Start Lima VM')
  .action(async () => {
    if (!checkMacOS()) {
      console.error('\x1b[31mError:\x1b[0m VM commands only available on macOS');
      process.exit(1);
    }
    if (!checkLimaInstalled()) {
      console.error('\x1b[31mError:\x1b[0m Lima not installed. Install with: brew install lima');
      process.exit(1);
    }
    try {
      startVM();
      console.log('\x1b[32m✓\x1b[0m VM started');

      // Wait for daemon to be ready (up to 30 seconds)
      process.stdout.write(`${c.dim}Waiting for daemon...${c.reset}`);
      let daemonReady = false;
      for (let i = 0; i < 30; i++) {
        if (await testConnection()) {
          daemonReady = true;
          break;
        }
        await new Promise(resolve => setTimeout(resolve, 1000));
        process.stdout.write('.');
      }
      console.log(''); // newline

      if (daemonReady) {
        console.log('\x1b[32m✓\x1b[0m Daemon ready');
      } else {
        console.log('\x1b[33m⚠\x1b[0m Daemon not responding yet (may still be starting)');
      }
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

vmCommand
  .command('stop')
  .description('Stop Lima VM')
  .action(async () => {
    if (!checkMacOS()) {
      console.error('\x1b[31mError:\x1b[0m VM commands only available on macOS');
      process.exit(1);
    }
    if (!checkLimaInstalled()) {
      console.error('\x1b[31mError:\x1b[0m Lima not installed');
      process.exit(1);
    }
    try {
      stopVM();
      console.log('\x1b[32m✓\x1b[0m VM stopped');
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

vmCommand
  .command('status')
  .description('Show VM status')
  .action(async () => {
    if (!checkMacOS()) {
      console.error('\x1b[31mError:\x1b[0m VM commands only available on macOS');
      process.exit(1);
    }
    if (!checkLimaInstalled()) {
      console.log('Lima: \x1b[31mnot installed\x1b[0m');
      console.log('Install with: brew install lima');
      return;
    }

    const status = getVMStatus();
    if (!status.exists) {
      console.log('VM: \x1b[33mnot created\x1b[0m');
      console.log('Create with: limactl start --name=orpheus template://default');
      return;
    }

    const statusColor = status.status === 'Running' ? '\x1b[32m' : '\x1b[33m';
    console.log(`VM: ${statusColor}${status.status}\x1b[0m`);
    if (status.arch) console.log(`Arch: ${status.arch}`);
    if (status.cpus) console.log(`CPUs: ${status.cpus}`);
    if (status.memory) console.log(`Memory: ${status.memory}`);
    if (status.disk) console.log(`Disk: ${status.disk}`);
  });

vmCommand
  .command('ssh')
  .description('SSH into VM')
  .action(async () => {
    if (!checkMacOS()) {
      console.error('\x1b[31mError:\x1b[0m VM commands only available on macOS');
      process.exit(1);
    }
    if (!checkLimaInstalled()) {
      console.error('\x1b[31mError:\x1b[0m Lima not installed');
      process.exit(1);
    }
    const status = getVMStatus();
    if (!status.exists) {
      console.error('\x1b[31mError:\x1b[0m VM not created');
      process.exit(1);
    }
    if (status.status !== 'Running') {
      console.error('\x1b[31mError:\x1b[0m VM not running. Start with: orpheus vm start');
      process.exit(1);
    }
    sshVM();
  });

vmCommand
  .command('delete')
  .description('Delete VM')
  .action(async () => {
    if (!checkMacOS()) {
      console.error('\x1b[31mError:\x1b[0m VM commands only available on macOS');
      process.exit(1);
    }
    if (!checkLimaInstalled()) {
      console.error('\x1b[31mError:\x1b[0m Lima not installed');
      process.exit(1);
    }
    try {
      deleteVM();
      console.log('\x1b[32m✓\x1b[0m VM deleted');
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

//@DAEMON_COMMANDS
const daemonCommand = program.command('daemon').description('Daemon management (Linux)');

daemonCommand
  .command('start')
  .description('Start daemon (systemd)')
  .action(async () => {
    const { execSync } = await import('node:child_process');
    const { platform } = await import('node:os');

    try {
      if (platform() === 'darwin') {
        // macOS - ensure VM is running, then start daemon via systemd
        console.log('Starting Lima VM...');
        execSync('limactl start orpheus 2>/dev/null || true', { encoding: 'utf-8' });
        console.log('Starting daemon...');
        execSync('limactl shell orpheus -- sudo systemctl start orpheusd 2>/dev/null || true', { encoding: 'utf-8' });
      } else {
        // Linux - start directly via systemd
        execSync('sudo systemctl start orpheusd', { encoding: 'utf-8' });
      }
      console.log('\x1b[32m✓\x1b[0m Daemon started');
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

daemonCommand
  .command('stop')
  .description('Stop daemon (systemd)')
  .action(async () => {
    const { execSync } = await import('node:child_process');
    const { platform } = await import('node:os');

    try {
      if (platform() === 'darwin') {
        execSync('limactl shell orpheus -- sudo systemctl stop orpheusd 2>/dev/null || true', { encoding: 'utf-8' });
      } else {
        execSync('sudo systemctl stop orpheusd', { encoding: 'utf-8' });
      }
      console.log('\x1b[32m✓\x1b[0m Daemon stopped');
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

daemonCommand
  .command('status')
  .description('Show daemon status (systemd)')
  .action(async () => {
    const { execSync } = await import('node:child_process');
    const { platform } = await import('node:os');

    try {
      let output: string;
      if (platform() === 'darwin') {
        output = execSync('limactl shell orpheus -- sudo systemctl status orpheusd 2>&1 || true', { encoding: 'utf-8' });
      } else {
        output = execSync('systemctl status orpheusd 2>&1 || true', { encoding: 'utf-8' });
      }
      console.log(output);
    } catch (err: unknown) {
      const error = err as { stdout?: string };
      console.log(error.stdout || 'Daemon status unknown');
    }
  });

//@SERVER_COMMANDS
const serverCommand = program.command('server').description('TCP server management');

serverCommand
  .command('start')
  .description('Start TCP server (direct process)')
  .option('-p, --port <port>', 'Port number', '7777')
  .action(async (options: { port?: string }) => {
    const { execSync } = await import('node:child_process');
    const { platform } = await import('node:os');
    const port = options.port || '7777';

    try {
      if (platform() === 'darwin') {
        // macOS - start daemon in Lima VM
        execSync(
          `limactl shell orpheus -- bash -c "nohup /usr/local/bin/orpheusd --tcp-bind :${port} > /var/log/orpheusd.log 2>&1 &"`,
          { encoding: 'utf-8' }
        );
      } else {
        // Linux - start daemon directly
        execSync(
          `nohup /usr/local/bin/orpheusd --tcp-bind :${port} > /var/log/orpheusd.log 2>&1 &`,
          { encoding: 'utf-8' }
        );
      }
      console.log(`\x1b[32m✓\x1b[0m Server started on port ${port}`);
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

serverCommand
  .command('stop')
  .description('Stop TCP server (kill process)')
  .action(async () => {
    const { execSync } = await import('node:child_process');
    const { platform } = await import('node:os');

    try {
      if (platform() === 'darwin') {
        execSync('limactl shell orpheus -- sudo pkill -f orpheusd 2>/dev/null || true', { encoding: 'utf-8' });
      } else {
        execSync('pkill -f orpheusd 2>/dev/null || true', { encoding: 'utf-8' });
      }
      console.log('\x1b[32m✓\x1b[0m Server stopped');
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

serverCommand
  .command('status')
  .description('Show server status')
  .action(async () => {
    const connected = await testConnection();
    if (connected) {
      const health = await getHealth();
      console.log(`\x1b[32m✓\x1b[0m Server is running`);
      if (health) {
        console.log(`  Status: ${health.status}`);
        console.log(`  Uptime: ${formatUptime(health.uptime_seconds)}`);
      }
    } else {
      console.log('\x1b[31m✗\x1b[0m Server is not running');
    }
  });

serverCommand
  .command('create-key')
  .description('Generate new API key')
  .option('-n, --name <name>', 'Key name', 'default')
  .option('--rpm <limit>', 'Rate limit (requests per minute)', '100')
  .action(async (options: { name?: string; rpm?: string }) => {
    const { platform } = await import('node:os');
    const { execSync } = await import('node:child_process');

    try {
      let output: string;
      const keyName = options.name || 'default';
      const rpm = options.rpm || '100';

      if (platform() === 'darwin') {
        // macOS - use limactl to execute in VM
        output = execSync(
          `limactl shell orpheus -- sudo /usr/local/bin/orpheusd create-key --name "${keyName}" --rpm ${rpm}`,
          { encoding: 'utf-8' }
        );
      } else {
        // Linux - execute directly
        output = execSync(
          `sudo orpheusd create-key --name "${keyName}" --rpm ${rpm}`,
          { encoding: 'utf-8' }
        );
      }
      console.log(output);
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

serverCommand
  .command('list-keys')
  .description('List API keys')
  .action(async () => {
    const { platform } = await import('node:os');
    const { execSync } = await import('node:child_process');

    try {
      let output: string;

      if (platform() === 'darwin') {
        // macOS - use limactl to execute in VM
        output = execSync(
          'limactl shell orpheus -- sudo /usr/local/bin/orpheusd list-keys',
          { encoding: 'utf-8' }
        );
      } else {
        // Linux - execute directly
        output = execSync('sudo orpheusd list-keys', { encoding: 'utf-8' });
      }
      console.log(output);
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

//@HELPERS
function formatUptime(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  return `${hours}h ${minutes}m`;
}

//@RUN
program.parse();
