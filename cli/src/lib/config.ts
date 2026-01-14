//@IMPORTS
import { existsSync, readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { homedir, platform } from 'node:os';
import { join } from 'node:path';
import YAML from 'yaml';
import type { ServerConfig, OrpheusConfigFile } from '../types/index.js';

//@ENV_VARS
// Environment variables for overriding defaults:
// - ORPHEUS_SOCKET: Path to unix socket (e.g., /var/run/orpheus.sock)
// - ORPHEUS_URL: URL for TCP connection (e.g., http://localhost:7080)

//@CONSTANTS
const CONFIG_DIR = join(homedir(), '.orpheus');
const CONFIG_FILE = join(CONFIG_DIR, 'config.yaml');

//@SOCKET_PATH
export function getDefaultSocketPath(): string {
  if (process.env.ORPHEUS_SOCKET) {
    return process.env.ORPHEUS_SOCKET;
  }

  if (platform() === 'darwin') {
    return join(homedir(), '.lima', 'orpheus', 'sock', 'orpheus.sock');
  }
  return '/var/run/orpheus.sock';
}

//@CONFIG_FILE
export function loadConfig(): OrpheusConfigFile {
  if (!existsSync(CONFIG_FILE)) {
    return {};
  }
  try {
    const content = readFileSync(CONFIG_FILE, 'utf-8');
    return YAML.parse(content) || {};
  } catch {
    return {};
  }
}

export function saveConfig(config: OrpheusConfigFile): void {
  if (!existsSync(CONFIG_DIR)) {
    mkdirSync(CONFIG_DIR, { recursive: true });
  }
  writeFileSync(CONFIG_FILE, YAML.stringify(config), 'utf-8');
}

export function saveServer(name: string, server: ServerConfig, setActive: boolean = true): void {
  const config = loadConfig();

  if (!config.servers) {
    config.servers = {};
  }
  config.servers[name] = server;

  if (setActive) {
    config.active = name;
  }

  saveConfig(config);
}

//@SERVER_CONFIG
export function getActiveServer(): ServerConfig {
  // Priority 1: Environment variable (for scripting/CI)
  if (process.env.ORPHEUS_URL) {
    return {
      mode: 'tcp',
      url: process.env.ORPHEUS_URL,
    };
  }

  // Priority 2: Config file
  const config = loadConfig();
  if (config.active && config.servers?.[config.active]) {
    return config.servers[config.active];
  }

  // Priority 3: Default unix socket
  return {
    mode: 'unix_socket',
    socket_path: getDefaultSocketPath(),
  };
}

export function getActiveServerName(): string {
  if (process.env.ORPHEUS_URL) {
    return process.env.ORPHEUS_URL;
  }

  const config = loadConfig();
  if (config.active) {
    return config.active;
  }

  return 'local';
}

//@UTILITIES
export function getConfigDir(): string {
  return CONFIG_DIR;
}

export function getConfigFilePath(): string {
  return CONFIG_FILE;
}

export function socketExists(): boolean {
  const server = getActiveServer();
  if (server.mode !== 'unix_socket') {
    return true; // TCP mode doesn't need socket check
  }

  const socketPath = server.socket_path || getDefaultSocketPath();
  return existsSync(socketPath);
}
