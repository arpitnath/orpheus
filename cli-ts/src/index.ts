#!/usr/bin/env node

import { Command } from 'commander';
import { createClient, testConnection, getHealth, getStats } from './lib/api.js';
import {
  getActiveServerName,
  getDefaultSocketPath,
  socketExists,
  listServers,
} from './lib/config.js';

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

      // TODO: fix with connection pooling
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
  });

program
  .command('deploy <path>')
  .description('Deploy an agent')
  .option('-f, --force', 'Overwrite existing agent')
  .option('-r, --remote', 'Deploy to remote server')
  .action(async (path: string, options: { force?: boolean; remote?: boolean }) => {
    console.log(`Deploying agent from: ${path}`);
    console.log('Options:', options);
    console.log('\n\x1b[33mDeploy command not yet implemented\x1b[0m');
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
  .action(async (path: string) => {
    console.log(`Validating: ${path}`);
    console.log('\n\x1b[33mValidate command not yet implemented\x1b[0m');
  });

//@OBSERVABILITY_COMMANDS

program
  .command('logs')
  .description('View daemon logs')
  .option('-f, --follow', 'Follow log output')
  .option('-n, --tail <lines>', 'Number of lines to show', '50')
  .option('--grep <pattern>', 'Filter by pattern')
  .action(async (options: { follow?: boolean; tail?: string; grep?: string }) => {
    console.log('Logs options:', options);
    console.log('\n\x1b[33mLogs command not yet implemented\x1b[0m');
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
  .action(async (agent: string, options: { format?: string }) => {
    console.log(`Inspecting agent: ${agent}`);
    console.log('Format:', options.format);
    console.log('\n\x1b[33mInspect command not yet implemented\x1b[0m');
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
  .action(async (options: { fix?: boolean }) => {
    console.log('Running health checks...');
    console.log('Fix mode:', options.fix);
    console.log('\n\x1b[33mHealthcheck command not yet implemented\x1b[0m');
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

program
  .command('login')
  .description('Manage API keys')
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

    console.log('\nUse subcommands to manage servers:');
    console.log('  orpheus login add <name> <url> [--key <api-key>]');
    console.log('  orpheus login use <name>');
    console.log('  orpheus login remove <name>');
  });

//@VM_COMMANDS
const vmCommand = program.command('vm').description('Lima VM management (macOS)');

vmCommand
  .command('start')
  .description('Start Lima VM')
  .action(async () => {
    console.log('\n\x1b[33mVM start not yet implemented\x1b[0m');
  });

vmCommand
  .command('stop')
  .description('Stop Lima VM')
  .action(async () => {
    console.log('\n\x1b[33mVM stop not yet implemented\x1b[0m');
  });

vmCommand
  .command('status')
  .description('Show VM status')
  .action(async () => {
    console.log('\n\x1b[33mVM status not yet implemented\x1b[0m');
  });

vmCommand
  .command('ssh')
  .description('SSH into VM')
  .action(async () => {
    console.log('\n\x1b[33mVM ssh not yet implemented\x1b[0m');
  });

vmCommand
  .command('delete')
  .description('Delete VM')
  .action(async () => {
    console.log('\n\x1b[33mVM delete not yet implemented\x1b[0m');
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
  .action(async () => {
    console.log('\n\x1b[33mCreate-key not yet implemented\x1b[0m');
  });

serverCommand
  .command('list-keys')
  .description('List API keys')
  .action(async () => {
    console.log('\n\x1b[33mList-keys not yet implemented\x1b[0m');
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
