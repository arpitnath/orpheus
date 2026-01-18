#!/usr/bin/env node

import React from 'react';
import { Command } from 'commander';
import { createClient, testConnection, getHealth, getStats } from './lib/api.js';
import { getActiveServer, getActiveServerName, saveServer, loadConfig, getConfigFilePath } from './lib/config.js';
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
import { Status } from './components/Status.js';
import { AgentList } from './components/AgentList.js';
import { ExecLogCrashed } from './components/ExecLogCrashed.js';
import { AgentPs } from './components/AgentPs.js';
import { AgentInspect } from './components/inspect/index.js';
import { PoolStats } from './components/PoolStats.js';
import type { ExecLogFilters } from './types/index.js';
import { HealthCheck } from './components/HealthCheck.js';
import { Validate } from './components/Validate.js';
import { c, label } from './lib/format.js';

//@VERSION
const VERSION = '0.1.0';

//@PROGRAM
const program = new Command();

program
  .name('orpheus')
  .description('Orpheus CLI - Infrastructure for AI agents')
  .version(VERSION, '-v, --version', 'Show version number');

//@CORE_COMMANDS

//@CONNECT_COMMANDS
program
  .command('connect <url>')
  .description('Connect to a remote Orpheus server')
  .option('-n, --name <name>', 'Name for the connection (default: derived from URL)')
  .option('--no-test', 'Skip connection test')
  .action(async (url: string, options: { name?: string; test?: boolean }) => {
    try {
      let normalizedUrl = url;
      if (!url.startsWith('http://') && !url.startsWith('https://')) {
        normalizedUrl = `http://${url}`;
      }

      const parsedUrl = new URL(normalizedUrl);
      const name = options.name || parsedUrl.hostname.replace(/\./g, '-');

      if (options.test !== false) {
        process.stdout.write(`${c.dim}Testing connection to ${normalizedUrl}...${c.reset}`);

        const originalUrl = process.env.ORPHEUS_URL;
        process.env.ORPHEUS_URL = normalizedUrl;
        const connected = await testConnection();
        process.env.ORPHEUS_URL = originalUrl;

        if (connected) {
          console.log(` ${c.green}OK${c.reset}`);
        } else {
          console.log(` ${c.red}FAILED${c.reset}`);
          console.error(`\n${c.red}Error:${c.reset} Could not connect to ${normalizedUrl}`);
          console.error(`${c.dim}Make sure the daemon is running and accessible.${c.reset}`);
          process.exit(1);
        }
      }

      saveServer(name, {
        mode: 'tcp',
        url: normalizedUrl,
      });

      console.log(`\n${c.green}✓${c.reset} Connected to ${c.cyan}${normalizedUrl}${c.reset}`);
      console.log(`  ${c.dim}Connection saved as '${name}' in ${getConfigFilePath()}${c.reset}`);
      console.log(`\n${c.dim}Now you can use: orpheus status, orpheus list, orpheus deploy, etc.${c.reset}`);
    } catch (err) {
      console.error(`${c.red}Error:${c.reset} ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

program
  .command('disconnect')
  .description('Disconnect from remote server (use local daemon)')
  .action(async () => {
    const config = loadConfig();
    const currentServer = getActiveServerName();

    if (currentServer === 'local' && !config.active) {
      console.log(`${c.dim}Already using local daemon.${c.reset}`);
      return;
    }

    saveServer('local', {
      mode: 'unix_socket',
    });

    console.log(`${c.green}✓${c.reset} Disconnected from '${currentServer}'`);
    console.log(`  ${c.dim}Now using local daemon.${c.reset}`);
  });

program
  .command('connection')
  .description('Show current connection info')
  .action(async () => {
    const serverName = getActiveServerName();
    const server = getActiveServer();
    const config = loadConfig();

    console.log(`\n${c.bold}Current Connection${c.reset}\n`);

    if (process.env.ORPHEUS_URL) {
      console.log(`  ${c.dim}Source:${c.reset} ORPHEUS_URL environment variable`);
      console.log(`  ${c.dim}URL:${c.reset}    ${c.cyan}${process.env.ORPHEUS_URL}${c.reset}`);
    } else if (server.mode === 'tcp') {
      console.log(`  ${c.dim}Name:${c.reset}   ${serverName}`);
      console.log(`  ${c.dim}Mode:${c.reset}   TCP`);
      console.log(`  ${c.dim}URL:${c.reset}    ${c.cyan}${server.url}${c.reset}`);
    } else {
      console.log(`  ${c.dim}Name:${c.reset}   ${serverName}`);
      console.log(`  ${c.dim}Mode:${c.reset}   Unix Socket`);
      console.log(`  ${c.dim}Path:${c.reset}   ${server.socket_path || 'default'}`);
    }

    process.stdout.write(`  ${c.dim}Status:${c.reset} `);
    const connected = await testConnection();
    if (connected) {
      const health = await getHealth();
      console.log(`${c.green}Connected${c.reset}`);
      if (health) {
        console.log(`  ${c.dim}Uptime:${c.reset} ${formatUptime(health.uptime_seconds)}`);
      }
    } else {
      console.log(`${c.red}Not connected${c.reset}`);
    }

    if (config.servers && Object.keys(config.servers).length > 0) {
      console.log(`\n${c.bold}Saved Connections${c.reset}\n`);
      for (const [name, srv] of Object.entries(config.servers)) {
        const active = name === config.active ? ` ${c.green}(active)${c.reset}` : '';
        if (srv.mode === 'tcp') {
          console.log(`  ${name}: ${srv.url}${active}`);
        } else {
          console.log(`  ${name}: ${srv.socket_path || 'unix socket'}${active}`);
        }
      }
    }

    console.log(`\n${c.dim}Config file: ${getConfigFilePath()}${c.reset}\n`);
  });

program
  .command('status')
  .description('Show system status and health')
  .action(async () => {
    renderApp(React.createElement(Status));
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
    renderApp(React.createElement(Validate, { path: agentPath }));
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

    renderApp(React.createElement(AgentList));
  });

program
  .command('stats')
  .description('Show pool statistics')
  .option('-f, --format <format>', 'Output format (json, text)', 'text')
  .action(async (options: { format?: string }) => {
    if (options.format === 'json') {
      // JSON output for scripting
      try {
        const stats = await getStats();
        console.log(JSON.stringify(stats, null, 2));
      } catch (err) {
        console.error(`${c.red}Error:${c.reset} ${err instanceof Error ? err.message : err}`);
        process.exit(1);
      }
    } else {
      // TUI view
      renderApp(React.createElement(PoolStats));
    }
  });

program
  .command('inspect <agent>')
  .description('Show agent details')
  .option('-f, --format <format>', 'Output format (json, text)', 'text')
  .action(async (agentName: string, options: { format?: string }) => {
    if (options.format === 'json') {
      // JSON output for scripting
      try {
        const client = createClient();
        const details = await client.inspect(agentName);
        console.log(JSON.stringify(details, null, 2));
        client.close();
      } catch (err) {
        console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
        process.exit(1);
      }
    } else {
      // TUI with tabs
      renderApp(React.createElement(AgentInspect, { agentName }));
    }
  });

program
  .command('ps')
  .description('Show running agents (interactive)')
  .action(async () => {
    renderApp(React.createElement(AgentPs));
  });

program
  .command('runs')
  .description('Show execution history')
  .argument('[agent]', 'Filter by agent name')
  .option('-n, --limit <count>', 'Number of runs to show', '20')
  .option('--session <id>', 'Filter by session ID')
  .option('--worker <id>', 'Filter by worker ID')
  .option('--status <state>', 'Filter by state (QUEUED/STARTED/COMPLETED/FAILED)')
  .action(async (agentName: string | undefined, options: { limit?: string; session?: string; worker?: string; status?: string }) => {
    try {
      const client = createClient();
      const limit = parseInt(options.limit || '20', 10);

      // Build filters
      const filters: ExecLogFilters = {
        agent: agentName,
        limit,
        status: options.status,
        session: options.session,
        worker: options.worker,
      };

      // Call ExecLog API
      const response = await client.getExecLogs(filters);

      if (!response.data || response.data.length === 0) {
        console.log('No execution history found');
        return;
      }

      // Display header
      const agentFilter = agentName ? ` for ${agentName}` : '';
      console.log(`\nExecution History${agentFilter} (${response.total} total)\n`);
      console.log('─'.repeat(120));
      console.log(
        'TIMESTAMP           REQUEST_ID                            STATE      WORKER                          DURATION    SESSION'
      );
      console.log('─'.repeat(120));

      // Group entries by request_id to show complete lifecycle
      const groupedByRequest = new Map<string, typeof response.data>();
      for (const entry of response.data) {
        if (!groupedByRequest.has(entry.request_id)) {
          groupedByRequest.set(entry.request_id, []);
        }
        groupedByRequest.get(entry.request_id)!.push(entry);
      }

      // Display each request's lifecycle
      for (const [_requestId, entries] of groupedByRequest) {
        const sortedEntries = entries.sort((a, b) => a.timestamp.localeCompare(b.timestamp));

        for (const entry of sortedEntries) {
          const timestamp = new Date(entry.timestamp).toLocaleTimeString('en-US', { hour12: false });
          const requestIdShort = entry.request_id.substring(0, 36);
          const state = entry.state.padEnd(10);
          const workerId = (entry.worker_id || '-').padEnd(30);
          const duration = entry.duration_ms ? `${(entry.duration_ms / 1000).toFixed(1)}s`.padEnd(10) : '-'.padEnd(10);
          const sessionId = entry.session_id ? entry.session_id.substring(0, 20) + '...' : '-';

          // Color code by state
          let color = '\x1b[0m'; // Default
          if (entry.state === 'COMPLETED') color = '\x1b[32m'; // Green
          if (entry.state === 'FAILED' || entry.state === 'CRASHED') color = '\x1b[31m'; // Red
          if (entry.state === 'STARTED') color = '\x1b[33m'; // Yellow
          if (entry.state === 'QUEUED') color = '\x1b[36m'; // Cyan

          console.log(`${color}${timestamp}  ${requestIdShort}  ${state}  ${workerId}  ${duration}  ${sessionId}\x1b[0m`);

          // Show error if present
          if (entry.error) {
            console.log(`  ${'\x1b[31m'}└─ Error: ${entry.error}\x1b[0m`);
          }
        }

        console.log(''); // Blank line between requests
      }

      // Footer with pagination info
      if (response.total_pages > 1) {
        console.log('─'.repeat(120));
        console.log(`Page ${response.page}/${response.total_pages} | Showing ${response.count} of ${response.total} total executions`);
        console.log(`Use --limit to see more, or add filters: --status FAILED --session <id> --worker <id>`);
      }
    } catch (err) {
      console.error(`\x1b[31mError:\x1b[0m ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

program
  .command('healthcheck')
  .description('Run system health diagnostics')
  .action(async () => {
    renderApp(React.createElement(HealthCheck));
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

//@WORKSPACE_COMMANDS
const workspaceCommand = program.command('workspace').description('Manage agent workspaces');

workspaceCommand
  .command('info <agent>')
  .description('Show workspace information')
  .option('-f, --format <format>', 'Output format (json, text)', 'text')
  .action(async (agentName: string, options: { format?: string }) => {
    try {
      const client = createClient();
      const info = await client.workspaceInfo(agentName);

      if (options.format === 'json') {
        console.log(JSON.stringify(info, null, 2));
        return;
      }

      console.log(`\n${c.cyan}Workspace:${c.reset} ${agentName}`);
      console.log(`  ${label('Path', info.path)}`);
      console.log(`  ${label('Exists', info.exists ? c.green + 'yes' + c.reset : c.dim + 'no' + c.reset)}`);

      if (info.exists) {
        console.log(`  ${label('Size', formatBytes(info.size_bytes))}`);
        console.log(`  ${label('Files', String(info.file_count))}`);

        if (info.files && Object.keys(info.files).length > 0) {
          console.log(`\n  ${c.dim}Contents:${c.reset}`);
          for (const [file, size] of Object.entries(info.files)) {
            console.log(`    ${file.padEnd(30)} ${formatBytes(size)}`);
          }
        }
      } else {
        console.log(`\n  ${c.dim}No workspace data yet. Run the agent to create workspace.${c.reset}`);
      }
      console.log('');
    } catch (err) {
      console.error(`${c.red}Error:${c.reset} ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

workspaceCommand
  .command('clean <agent>')
  .description('Clean workspace (delete persistent data)')
  .option('-f, --force', 'Skip confirmation')
  .action(async (agentName: string, options: { force?: boolean }) => {
    try {
      const client = createClient();

      // Get workspace info first to show what will be deleted
      const info = await client.workspaceInfo(agentName);

      if (!info.exists || info.size_bytes === 0) {
        console.log(`${c.dim}Workspace is already empty.${c.reset}`);
        return;
      }

      // Show what will be deleted
      console.log(`\n${c.yellow}Workspace:${c.reset} ${agentName}`);
      console.log(`  ${label('Path', info.path)}`);
      console.log(`  ${label('Size', formatBytes(info.size_bytes))}`);
      console.log(`  ${label('Files', String(info.file_count))}`);

      // Confirm unless --force
      if (!options.force) {
        const readline = await import('node:readline');
        const rl = readline.createInterface({
          input: process.stdin,
          output: process.stdout,
        });

        const answer = await new Promise<string>((resolve) => {
          rl.question(`\n${c.yellow}Clean workspace? This will delete all persistent data. [y/N]:${c.reset} `, resolve);
        });
        rl.close();

        if (answer.toLowerCase() !== 'y' && answer.toLowerCase() !== 'yes') {
          console.log(`${c.dim}Cancelled.${c.reset}`);
          return;
        }
      }

      // Clean workspace
      const result = await client.workspaceClean(agentName);

      if (result.status === 'success') {
        console.log(`\n${c.green}✓${c.reset} Workspace cleaned (${formatBytes(result.freed_bytes)} freed)`);
      } else {
        console.log(`${c.red}✗${c.reset} Failed to clean workspace`);
        process.exit(1);
      }
    } catch (err) {
      console.error(`${c.red}Error:${c.reset} ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

//@EXECLOG_COMMANDS
const execlogCommand = program.command('execlog').description('View execution logs');

execlogCommand
  .command('crashed')
  .description('Show crashed requests')
  .action(async () => {
    renderApp(React.createElement(ExecLogCrashed));
  });

execlogCommand
  .command('list')
  .description('List execution logs')
  .argument('[agent]', 'Filter by agent name')
  .option('-s, --status <status>', 'Filter by status (QUEUED, STARTED, COMPLETED, FAILED, CRASHED)')
  .option('--session <id>', 'Filter by session ID')
  .option('-w, --worker <id>', 'Filter by worker ID')
  .option('-n, --limit <count>', 'Number of records to show', '50')
  .option('--offset <num>', 'Pagination offset', '0')
  .option('-f, --format <format>', 'Output format (table, json)', 'table')
  .action(async (agentName: string | undefined, options: {
    status?: string;
    session?: string;
    worker?: string;
    limit?: string;
    offset?: string;
    format?: string;
  }) => {
    try {
      const filters: import('./types/index.js').ExecLogFilters = {
        agent: agentName,
        status: options.status,
        session: options.session,
        worker: options.worker,
        limit: parseInt(options.limit || '50', 10),
        offset: parseInt(options.offset || '0', 10),
      };

      const client = createClient();
      const response = await client.getExecLogs(filters);

      if (options.format === 'json') {
        console.log(JSON.stringify(response, null, 2));
        return;
      }

      // Table format
      if (response.data.length === 0) {
        console.log(`\n${c.dim}No execution logs found.${c.reset}\n`);
        return;
      }

      console.log(`\n${c.bold}Execution Logs${c.reset} (Showing ${response.count} of ${response.total})\n`);

      // Display table header
      const cols = ['REQUEST ID', 'AGENT', 'STATE', 'WORKER', 'DURATION'];
      const widths = [16, 20, 10, 16, 10];
      let header = '';
      for (let i = 0; i < cols.length; i++) {
        header += cols[i].padEnd(widths[i]);
      }
      console.log(c.dim + header + c.reset);
      console.log(c.dim + '─'.repeat(72) + c.reset);

      // Display rows
      for (const log of response.data) {
        const reqId = log.request_id.substring(0, 12) + '...';
        const agent = log.agent_name.substring(0, 18);
        const state = log.state;
        const worker = log.worker_id ? log.worker_id.substring(0, 14) : '-';
        const duration = log.duration_ms ? `${log.duration_ms}ms` : '-';

        // Color code state
        let stateColored = state;
        if (state === 'COMPLETED') stateColored = c.green + state + c.reset;
        else if (state === 'FAILED') stateColored = c.red + state + c.reset;
        else if (state === 'CRASHED') stateColored = c.yellow + state + c.reset;
        else if (state === 'QUEUED') stateColored = c.dim + state + c.reset;
        else if (state === 'STARTED') stateColored = c.cyan + state + c.reset;

        console.log(
          reqId.padEnd(widths[0]) +
          agent.padEnd(widths[1]) +
          stateColored.padEnd(widths[2] + (stateColored.length - state.length)) +
          worker.padEnd(widths[3]) +
          duration.padEnd(widths[4])
        );
      }

      // Pagination info
      if (response.total_pages > 1) {
        console.log(`\n${c.dim}Page ${response.page} of ${response.total_pages}${c.reset}`);
        if (response.page < response.total_pages) {
          console.log(`${c.dim}Next: orpheus execlog list --offset ${response.offset + response.limit}${c.reset}`);
        }
      }
      console.log('');
    } catch (err) {
      console.error(`${c.red}Error:${c.reset} ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

execlogCommand
  .command('stats')
  .description('Show execution statistics')
  .argument('[agent]', 'Agent name (optional, shows all if omitted)')
  .option('-f, --format <format>', 'Output format (table, json)', 'table')
  .action(async (agentName: string | undefined, options: { format?: string }) => {
    try {
      const client = createClient();
      const response = await client.getExecLogStats(agentName);

      if (options.format === 'json') {
        console.log(JSON.stringify(response, null, 2));
        return;
      }

      // Table format
      if (response.agents.length === 0) {
        console.log(`\n${c.dim}No execution logs found.${c.reset}\n`);
        return;
      }

      console.log(`\n${c.bold}ExecLog Statistics${c.reset}\n`);

      // Table header
      const cols = ['AGENT', 'TOTAL', 'COMPLETED', 'FAILED', 'CRASHED', 'SUCCESS%', 'AVG(ms)', 'HEALTH'];
      const widths = [15, 9, 11, 8, 9, 10, 9, 10];
      let header = '';
      for (let i = 0; i < cols.length; i++) {
        header += cols[i].padEnd(widths[i]);
      }
      console.log(c.dim + header + c.reset);
      console.log(c.dim + '─'.repeat(80) + c.reset);

      // Per-agent rows
      for (const stats of response.agents) {
        const health =
          stats.health_status === 'healthy'
            ? c.green + stats.health_status + c.reset
            : stats.health_status === 'degraded'
            ? c.yellow + stats.health_status + c.reset
            : c.red + stats.health_status + c.reset;

        console.log(
          stats.agent_name.padEnd(widths[0]) +
          String(stats.total).padEnd(widths[1]) +
          String(stats.completed).padEnd(widths[2]) +
          String(stats.failed).padEnd(widths[3]) +
          String(stats.crashed).padEnd(widths[4]) +
          (stats.success_rate.toFixed(1) + '%').padEnd(widths[5]) +
          Math.round(stats.avg_duration_ms).toString().padEnd(widths[6]) +
          health
        );
      }

      // Global summary (if multiple agents)
      if (response.agents.length > 1) {
        console.log(c.dim + '─'.repeat(80) + c.reset);
        console.log(
          'Total'.padEnd(widths[0]) +
          String(response.global.total_requests).padEnd(widths[1]) +
          String(response.global.completed).padEnd(widths[2]) +
          String(response.global.failed).padEnd(widths[3]) +
          String(response.global.crashed).padEnd(widths[4]) +
          (response.global.success_rate.toFixed(1) + '%').padEnd(widths[5]) +
          Math.round(response.global.avg_duration_ms).toString().padEnd(widths[6])
        );
      }

      console.log('');
    } catch (err) {
      console.error(`${c.red}Error:${c.reset} ${err instanceof Error ? err.message : err}`);
      process.exit(1);
    }
  });

//@HELPERS
function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

function formatUptime(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  return `${hours}h ${minutes}m`;
}

//@RUN
program.parse();
