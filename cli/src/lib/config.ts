//@IMPORTS
import { readFileSync, writeFileSync, mkdirSync, existsSync } from 'node:fs';
import { homedir, platform } from 'node:os';
import { join } from 'node:path';
import { parse, stringify } from 'yaml';
import type { CLIConfig, ServerConfig } from '../types/index.js';

//@CONSTANTS
const CONFIG_DIR = join(homedir(), '.orpheus');
const CONFIG_FILE = join(CONFIG_DIR, 'config.yaml');

//@SOCKET_PATH
export function getDefaultSocketPath(): string {
  if (platform() === 'darwin') {
    // macOS: Lima VM forwarded socket
    return join(homedir(), '.lima', 'orpheus', 'sock', 'orpheus.sock');
  }
  // Linux: Local socket
  return '/var/run/orpheus.sock';
}

//@DEFAULT_CONFIG
export function getDefaultConfig(): CLIConfig {
  return {
    active: 'local',
    servers: {
      local: {
        mode: 'unix_socket',
        socket_path: getDefaultSocketPath(),
      },
    },
  };
}

//@CONFIG_FILE_OPS
export function loadConfig(): CLIConfig {
  if (!existsSync(CONFIG_FILE)) {
    return getDefaultConfig();
  }

  try {
    const content = readFileSync(CONFIG_FILE, 'utf-8');
    const config = parse(content) as CLIConfig | null;
    return config || getDefaultConfig();
  } catch {
    return getDefaultConfig();
  }
}

export function saveConfig(config: CLIConfig): void {
  // Ensure config directory exists
  if (!existsSync(CONFIG_DIR)) {
    mkdirSync(CONFIG_DIR, { recursive: true });
  }

  const content = stringify(config, { indent: 2 });
  writeFileSync(CONFIG_FILE, content, 'utf-8');
}

//@SERVER_MANAGEMENT
export function getActiveServer(): ServerConfig {
  const config = loadConfig();
  const active = config.active || 'local';

  if (!(active in config.servers)) {
    // Active server doesn't exist, fall back to local
    return config.servers['local'] || getDefaultConfig().servers['local'];
  }

  return config.servers[active];
}

export function getActiveServerName(): string {
  const config = loadConfig();
  return config.active || 'local';
}

export function addServer(name: string, url: string, authKey?: string): void {
  const config = loadConfig();

  config.servers[name] = {
    mode: 'tcp',
    url,
    ...(authKey && { auth_key: authKey }),
  };

  saveConfig(config);
}

export function removeServer(name: string): void {
  const config = loadConfig();

  if (name in config.servers) {
    delete config.servers[name];

    // If removing active server, switch to local
    if (config.active === name) {
      config.active = 'local';
    }

    saveConfig(config);
  }
}

export function setActiveServer(name: string): void {
  const config = loadConfig();

  if (!(name in config.servers)) {
    throw new Error(`Server '${name}' not found in configuration`);
  }

  config.active = name;
  saveConfig(config);
}

export function listServers(): Record<string, ServerConfig> {
  const config = loadConfig();
  return config.servers;
}

export function hasServer(name: string): boolean {
  const config = loadConfig();
  return name in config.servers;
}

//@UTILITIES
export function getConfigPath(): string {
  return CONFIG_FILE;
}

export function getConfigDir(): string {
  return CONFIG_DIR;
}

export function socketExists(): boolean {
  const server = getActiveServer();
  if (server.mode !== 'unix_socket') {
    return true; // TCP mode doesn't need socket check
  }

  const socketPath = server.socket_path || getDefaultSocketPath();
  return existsSync(socketPath);
}
