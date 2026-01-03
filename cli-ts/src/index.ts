#!/usr/bin/env node

import React from 'react';
import { Command } from 'commander';
import { createClient, testConnection, getHealth, getStats } from './lib/api.js';
import {
  getActiveServerName,
  getDefaultSocketPath,
  socketExists,
  listServers,
  addServer,
  removeServer,
  setActiveServer,
} from './lib/config.js';
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
import { StatusDashboard } from './components/StatusDashboard.js';
import { DeployProgress } from './components/DeployProgress.js';
import { LogViewer } from './components/LogViewer.js';

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
  .option('--simple', 'Simple text output (no TUI)')
  .action(async (options: { simple?: boolean }) => {
    if (options.simple) {
      // Simple text output
      console.log('Orpheus Status\n');
      const activeServer = getActiveServerName();
      console.log(`Server: ${activeServer}`);

      if (!socketExists()) {
        console.log('Daemon: \x1b[31mnot running\x1b[0m (socket not found)');
        console.log(`Socket: ${getDefaultSocketPath()}`);
        return;
      }

      const health = await getHealth();
      if (health) {
        const statusColor = health.status === 'healthy' ? '\x1b[32m' : '\x1b[33m';
        console.log(`Daemon: ${statusColor}${health.status}\x1b[0m`);
        console.log(`Uptime: ${formatUptime(health.uptime_seconds)}`);

        await new Promise(resolve => setTimeout(resolve, 50));
        const stats = await getStats();
        if (stats && stats.global) {
          console.log(`\nAgents: ${stats.global.total_agents} deployed`);
          console.log(`Workers: ${stats.global.total_workers} total`);
          console.log(`Pending: ${stats.global.total_pending} requests`);
        }
      } else {
        console.log('Daemon: \x1b[31mnot responding\x1b[0m');
      }
      return;
    }

    // TUI dashboard (default)
    renderApp(React.createElement(StatusDashboard));
  });

program
  .command('deploy <path>')
  .description('Deploy an agent')
  .option('-f, --force', 'Overwrite existing agent')
  .option('-r, --remote', 'Deploy to remote server')
  .option('--simple', 'Simple text output (no TUI)')
  .action(async (agentPath: string, options: { force?: boolean; remote?: boolean; simple?: boolean }) => {
    const path = await import('node:path');

    // Extract agent name from path
    const resolvedPath = path.resolve(agentPath);
    const agentName = path.basename(resolvedPath);

    if (options.simple) {
      console.log(`Deploying agent from: ${agentPath}`);
      console.log('\n\x1b[33mDeploy not yet fully implemented\x1b[0m');
      return;
    }

    // TUI deploy progress
    const onDeploy = async () => {
      try {
        const client = createClient();
        const result = await client.deploy(resolvedPath, { force: options.force });
        return {
          success: result.success,
          endpoints: result.endpoints,
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
  .description('Run an agent locally')
  .option('-i, --input <json>', 'Input JSON')
  .option('--no-isolate', 'Skip container isolation')
  .action(async (agent: string, options: { input?: string; isolate?: boolean }) => {
    console.log(`Running agent: ${agent}`);
    console.log('Options:', options);
    console.log('\n\x1b[33mRun command not yet implemented\x1b[0m');
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
      if (result.success) {
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
      console.log('\x1b[32m✓\x1b[0m Valid agent.yaml');
      if (result.config) {
        console.log(`  Name: ${result.config.name}`);
        console.log(`  Runtime: ${result.config.runtime}`);
        console.log(`  Module: ${result.config.module}`);
        console.log(`  Entrypoint: ${result.config.entrypoint}`);
        if (result.config.scaling) {
          console.log(`  Scaling: ${result.config.scaling.min_workers}-${result.config.scaling.max_workers} workers`);
        }
      }
    } else {
      console.log('\x1b[31m✗\x1b[0m Invalid agent.yaml');
      for (const error of result.errors) {
        console.log(`  \x1b[31m•\x1b[0m ${error}`);
      }
    }

    if (result.warnings.length > 0) {
      console.log('\nWarnings:');
      for (const warning of result.warnings) {
        console.log(`  \x1b[33m•\x1b[0m ${warning}`);
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
  .option('-f, --follow', 'Follow log output')
  .option('-n, --tail <lines>', 'Number of lines to show', '50')
  .option('--grep <pattern>', 'Filter by pattern')
  .option('--simple', 'Simple text output (no TUI)')
  .action(async (options: { follow?: boolean; tail?: string; grep?: string; simple?: boolean }) => {
    const tailNum = parseInt(options.tail || '50', 10);

    if (options.simple) {
      console.log('Logs (simple mode)');
      console.log('\n\x1b[33mLogs endpoint not yet implemented in daemon\x1b[0m');
      return;
    }

    // Mock log fetcher - daemon needs /v1/logs endpoint
    const onFetchLogs = async () => {
      // TODO: Replace with actual daemon API call when available
      // For now, return mock data to demonstrate the UI
      return [
        { timestamp: new Date().toISOString().slice(11, 19), level: 'INFO' as const, message: 'Daemon started' },
        { timestamp: new Date().toISOString().slice(11, 19), level: 'INFO' as const, message: 'Worker pool initialized', agent: 'calculator' },
        { timestamp: new Date().toISOString().slice(11, 19), level: 'DEBUG' as const, message: 'Health check passed' },
      ];
    };

    renderApp(
      React.createElement(LogViewer, {
        follow: options.follow,
        tail: tailNum,
        grep: options.grep,
        onFetchLogs,
      })
    );
  });

program
  .command('list')
  .description('List deployed agents')
  .option('--images', 'Show base images instead')
  .action(async (options: { images?: boolean }) => {
    if (options.images) {
      console.log('Base images:');
      console.log('\n\x1b[33mImages listing not yet implemented\x1b[0m');
      return;
    }

    try {
      const client = createClient();
      const agents = await client.list();

      if (agents.length === 0) {
        console.log('No agents deployed');
        return;
      }

      console.log('Deployed Agents:\n');
      console.log('NAME\t\t\tRUNTIME\t\tWORKERS\t\tSTATUS');
      console.log('─'.repeat(60));
      for (const agent of agents) {
        const statusColor =
          agent.status === 'running' ? '\x1b[32m' : agent.status === 'idle' ? '\x1b[33m' : '\x1b[31m';
        console.log(
          `${agent.name}\t\t${agent.runtime}\t\t${agent.workers}\t\t${statusColor}${agent.status}\x1b[0m`
        );
      }
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
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
        console.error('\x1b[31mError:\x1b[0m Cannot connect to daemon');
        process.exit(1);
      }

      console.log('Pool Statistics\n');
      console.log(`Total Agents: ${stats.global.total_agents}`);
      console.log(`Total Workers: ${stats.global.total_workers}`);
      console.log(`Pending Requests: ${stats.global.total_pending}`);
      console.log(`Processing: ${stats.global.total_processing}`);
      console.log(`Average Utilization: ${stats.global.average_utilization.toFixed(1)}%`);

      if (stats.agents && stats.agents.length > 0) {
        console.log('\nPer-Agent Stats:');
        console.log('AGENT\t\t\t\tWORKERS\tPENDING\tPROCESSING\tUTILIZATION');
        console.log('─'.repeat(70));
        for (const a of stats.agents) {
          const name = a.agent_name.length > 20 ? a.agent_name.slice(0, 17) + '...' : a.agent_name.padEnd(20);
          console.log(
            `${name}\t\t${a.pool.total_workers}\t${a.queue.pending}\t${a.queue.processing}\t\t${a.derived.utilization_percentage.toFixed(1)}%`
          );
        }
      }
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
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

      // Text format (default)
      console.log(`Agent: ${details.name}`);
      console.log(`Runtime: ${details.runtime}`);
      console.log(`Module: ${details.module}`);
      console.log(`Entrypoint: ${details.entrypoint}`);

      console.log('\nEndpoints:');
      console.log(`  HTTP: ${details.endpoints.http}`);
      if (details.endpoints.mcp) {
        console.log(`  MCP:  ${details.endpoints.mcp}`);
      }

      console.log('\nScaling:');
      console.log(`  Workers: ${details.workers}${details.scaling ? ` (min: ${details.scaling.min_workers}, max: ${details.scaling.max_workers})` : ''}`);

      const statusColor = details.status === 'running' ? '\x1b[32m' : details.status === 'idle' ? '\x1b[33m' : '\x1b[31m';
      console.log(`\nStatus: ${statusColor}${details.status}\x1b[0m`);
      if (details.deployed_at) {
        console.log(`Deployed: ${details.deployed_at}`);
      }
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

program
  .command('ps')
  .description('Show running containers')
  .option('-a, --all', 'Show all containers')
  .action(async (options: { all?: boolean }) => {
    console.log('PS options:', options);
    console.log('\n\x1b[33mPS command not yet implemented\x1b[0m');
  });

program
  .command('runs')
  .description('Show execution history')
  .action(async () => {
    console.log('\n\x1b[33mRuns command not yet implemented\x1b[0m');
  });

program
  .command('healthcheck')
  .description('Run system health diagnostics')
  .option('--fix', 'Attempt to fix issues')
  .action(async (_options: { fix?: boolean }) => {
    console.log('Running health checks...\n');

    let allPassed = true;
    const { platform } = await import('node:os');
    const { execSync } = await import('node:child_process');

    // Check 1: Config exists
    const configValid = socketExists() || getActiveServerName() !== 'local';
    if (configValid) {
      console.log('\x1b[32m✓\x1b[0m Config valid');
    } else {
      console.log('\x1b[33m!\x1b[0m Config: using defaults');
    }

    // Check 2: Daemon reachable
    const connected = await testConnection();
    if (connected) {
      console.log('\x1b[32m✓\x1b[0m Daemon reachable');
    } else {
      console.log('\x1b[31m✗\x1b[0m Daemon not reachable');
      allPassed = false;
    }

    // Check 3: Daemon health
    if (connected) {
      const health = await getHealth();
      if (health && health.status === 'healthy') {
        console.log(`\x1b[32m✓\x1b[0m Daemon healthy (uptime: ${formatUptime(health.uptime_seconds)})`);
      } else if (health) {
        console.log(`\x1b[33m!\x1b[0m Daemon ${health.status}`);
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
          console.log('\x1b[32m✓\x1b[0m Lima VM running');
        } else if (orpheusVm) {
          console.log(`\x1b[33m!\x1b[0m Lima VM ${orpheusVm.status.toLowerCase()}`);
        } else {
          console.log('\x1b[33m!\x1b[0m Lima VM not found');
        }
      } catch {
        console.log('\x1b[33m!\x1b[0m Lima not installed or not configured');
      }
    }

    console.log('');
    if (allPassed) {
      console.log('\x1b[32mAll checks passed!\x1b[0m');
    } else {
      console.log('\x1b[31mSome checks failed\x1b[0m');
      process.exit(1);
    }
  });

//@UTILITY_COMMANDS

program
  .command('test <agent>')
  .description('Test an agent')
  .argument('[input]', 'Test input JSON')
  .option('--verbose', 'Verbose output')
  .action(async (agent: string, input?: string, options?: { verbose?: boolean }) => {
    console.log(`Testing agent: ${agent}`);
    console.log('Input:', input);
    console.log('Options:', options);
    console.log('\n\x1b[33mTest command not yet implemented\x1b[0m');
  });

program
  .command('shell')
  .description('Start interactive shell')
  .action(async () => {
    console.log('\n\x1b[33mShell command not yet implemented\x1b[0m');
  });

program
  .command('exec <command...>')
  .description('Execute a command in daemon context')
  .action(async (command: string[]) => {
    console.log('Command:', command.join(' '));
    console.log('\n\x1b[33mExec command not yet implemented\x1b[0m');
  });

//@LOGIN_COMMANDS
const loginCommand = program.command('login').description('Manage server connections');

loginCommand
  .command('list', { isDefault: true })
  .description('List configured servers')
  .action(async () => {
    const servers = listServers();
    const active = getActiveServerName();

    console.log('Configured Servers:\n');
    for (const [name, config] of Object.entries(servers)) {
      const marker = name === active ? '\x1b[32m→\x1b[0m' : ' ';
      const mode = config.mode === 'unix_socket' ? 'socket' : 'tcp';
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
      console.log(`\x1b[32m✓\x1b[0m Added server: ${name}`);
      console.log(`  URL: ${url}`);
      if (options.key) {
        console.log(`  Auth: configured`);
      }
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

loginCommand
  .command('use <name>')
  .description('Set active server')
  .action(async (name: string) => {
    try {
      setActiveServer(name);
      console.log(`\x1b[32m✓\x1b[0m Active server set to: ${name}`);
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

loginCommand
  .command('remove <name>')
  .description('Remove a server')
  .action(async (name: string) => {
    try {
      removeServer(name);
      console.log(`\x1b[32m✓\x1b[0m Removed server: ${name}`);
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
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
  .description('Start daemon')
  .action(async () => {
    console.log('\n\x1b[33mDaemon start not yet implemented\x1b[0m');
  });

daemonCommand
  .command('stop')
  .description('Stop daemon')
  .action(async () => {
    console.log('\n\x1b[33mDaemon stop not yet implemented\x1b[0m');
  });

daemonCommand
  .command('status')
  .description('Show daemon status')
  .action(async () => {
    console.log('\n\x1b[33mDaemon status not yet implemented\x1b[0m');
  });

//@SERVER_COMMANDS
const serverCommand = program.command('server').description('TCP server management');

serverCommand
  .command('start')
  .description('Start TCP server')
  .option('-p, --port <port>', 'Port number', '7777')
  .action(async (options: { port?: string }) => {
    console.log('Port:', options.port);
    console.log('\n\x1b[33mServer start not yet implemented\x1b[0m');
  });

serverCommand
  .command('stop')
  .description('Stop TCP server')
  .action(async () => {
    console.log('\n\x1b[33mServer stop not yet implemented\x1b[0m');
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
