//@IMPORTS
import { existsSync } from 'node:fs';
import { homedir, platform } from 'node:os';
import { join } from 'node:path';
import type { ServerConfig } from '../types/index.js';

//@ENV_VARS
// Environment variables for overriding defaults:
// - ORPHEUS_SOCKET: Path to unix socket (e.g., /var/run/orpheus.sock)
// - ORPHEUS_URL: URL for TCP connection (e.g., http://localhost:7080)

//@CONSTANTS
const CONFIG_DIR = join(homedir(), '.orpheus');

//@SOCKET_PATH
export function getDefaultSocketPath(): string {
  // Check env var first
  if (process.env.ORPHEUS_SOCKET) {
    return process.env.ORPHEUS_SOCKET;
  }

  if (platform() === 'darwin') {
    // macOS: Lima VM forwarded socket
    return join(homedir(), '.lima', 'orpheus', 'sock', 'orpheus.sock');
  }
  // Linux: Local socket
  return '/var/run/orpheus.sock';
}

//@SERVER_CONFIG
export function getActiveServer(): ServerConfig {
  // Check for URL env var (TCP mode)
  if (process.env.ORPHEUS_URL) {
    return {
      mode: 'tcp',
      url: process.env.ORPHEUS_URL,
    };
  }

  // Default to unix socket
  return {
    mode: 'unix_socket',
    socket_path: getDefaultSocketPath(),
  };
}

export function getActiveServerName(): string {
  if (process.env.ORPHEUS_URL) {
    return process.env.ORPHEUS_URL;
  }
  return 'local';
}

//@UTILITIES
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
