//@DEPLOY_UTILS
import { createHash, randomBytes } from 'node:crypto';
import { statSync } from 'node:fs';
import { basename, join, resolve } from 'node:path';
import { request as httpRequest } from 'node:http';
import { request as httpsRequest, RequestOptions } from 'node:https';
import { URL } from 'node:url';
import * as tar from 'tar';
import type { ServerConfig, DeployResponse } from '../types/index.js';

//@TARBALL_CREATION
export async function createTarball(agentPath: string): Promise<Buffer> {
  const resolvedPath = resolve(agentPath);
  const agentName = basename(resolvedPath);

  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];

    const tarStream = tar.create(
      {
        gzip: true,
        cwd: join(resolvedPath, '..'),
        prefix: '',
      },
      [agentName]
    );

    tarStream.on('data', (chunk: Buffer) => {
      chunks.push(chunk);
    });

    tarStream.on('end', () => {
      resolve(Buffer.concat(chunks));
    });

    tarStream.on('error', (err: unknown) => {
      reject(err);
    });
  });
}

//@CHECKSUM
export function calculateChecksum(buffer: Buffer): string {
  return createHash('sha256').update(buffer).digest('hex');
}

//@MULTIPART_UPLOAD
export async function uploadAgent(
  serverConfig: ServerConfig,
  agentName: string,
  tarball: Buffer,
  checksum: string,
  env?: string[]
): Promise<DeployResponse> {
  const boundary = '----FormBoundary' + randomBytes(16).toString('hex');

  // Build multipart form data
  const parts: Buffer[] = [];

  // agent_name field
  parts.push(Buffer.from(
    `--${boundary}\r\n` +
    `Content-Disposition: form-data; name="agent_name"\r\n\r\n` +
    `${agentName}\r\n`
  ));

  // checksum field
  parts.push(Buffer.from(
    `--${boundary}\r\n` +
    `Content-Disposition: form-data; name="checksum"\r\n\r\n` +
    `${checksum}\r\n`
  ));

  // env field (optional)
  if (env && env.length > 0) {
    parts.push(Buffer.from(
      `--${boundary}\r\n` +
      `Content-Disposition: form-data; name="env"\r\n\r\n` +
      `${JSON.stringify(env)}\r\n`
    ));
  }

  // agent_tar file
  parts.push(Buffer.from(
    `--${boundary}\r\n` +
    `Content-Disposition: form-data; name="agent_tar"; filename="${agentName}.tar.gz"\r\n` +
    `Content-Type: application/gzip\r\n\r\n`
  ));
  parts.push(tarball);
  parts.push(Buffer.from('\r\n'));

  // End boundary
  parts.push(Buffer.from(`--${boundary}--\r\n`));

  const body = Buffer.concat(parts);

  return new Promise((resolve, reject) => {
    let options: RequestOptions;
    let requestFn: typeof httpRequest;

    if (serverConfig.mode === 'unix_socket') {
      const socketPath = serverConfig.socket_path;
      if (!socketPath) {
        reject(new Error('Socket path not configured'));
        return;
      }

      options = {
        socketPath,
        path: '/v1/deploy',
        method: 'POST',
        headers: {
          'Content-Type': `multipart/form-data; boundary=${boundary}`,
          'Content-Length': body.length,
        },
      };
      requestFn = httpRequest;
    } else {
      const url = serverConfig.url;
      if (!url) {
        reject(new Error('Server URL not configured'));
        return;
      }

      const parsedUrl = new URL('/v1/deploy', url);
      const isHttps = parsedUrl.protocol === 'https:';

      options = {
        hostname: parsedUrl.hostname,
        port: parsedUrl.port || (isHttps ? 443 : 80),
        path: parsedUrl.pathname,
        method: 'POST',
        headers: {
          'Content-Type': `multipart/form-data; boundary=${boundary}`,
          'Content-Length': body.length,
          ...(serverConfig.auth_key && {
            Authorization: `Bearer ${serverConfig.auth_key}`,
          }),
        },
      };
      requestFn = isHttps ? httpsRequest : httpRequest;
    }

    const req = requestFn(options, (res) => {
      let data = '';

      res.on('data', (chunk: Buffer) => {
        data += chunk.toString();
      });

      res.on('end', () => {
        if (res.statusCode && res.statusCode >= 200 && res.statusCode < 300) {
          try {
            const parsed = JSON.parse(data) as DeployResponse;
            resolve(parsed);
          } catch {
            reject(new Error(`Invalid response: ${data}`));
          }
        } else {
          let errorMessage = `HTTP ${res.statusCode}`;
          try {
            const errorData = JSON.parse(data);
            errorMessage = errorData.error || errorData.message || errorMessage;
          } catch {
            if (data) errorMessage = data;
          }
          reject(new Error(errorMessage));
        }
      });
    });

    req.on('error', (err: Error) => {
      if ((err as NodeJS.ErrnoException).code === 'ECONNREFUSED') {
        reject(new Error('Cannot connect to daemon. Is it running?'));
      } else if ((err as NodeJS.ErrnoException).code === 'ENOENT') {
        reject(new Error('Daemon socket not found. Is the daemon running?'));
      } else {
        reject(err);
      }
    });

    req.write(body);
    req.end();
  });
}

//@SSE_DEPLOY_PROGRESS
export interface DeployProgressEvent {
  phase: string;
  message: string;
  progress: number;
}

export type DeployProgressCallback = (event: DeployProgressEvent) => void;

//@SSE_UPLOAD
export async function uploadAgentWithSSE(
  serverConfig: ServerConfig,
  agentName: string,
  tarball: Buffer,
  checksum: string,
  env?: string[],
  onProgress?: DeployProgressCallback
): Promise<DeployResponse> {
  const boundary = '----FormBoundary' + randomBytes(16).toString('hex');

  // Build multipart form data (same as uploadAgent)
  const parts: Buffer[] = [];

  parts.push(Buffer.from(
    `--${boundary}\r\n` +
    `Content-Disposition: form-data; name="agent_name"\r\n\r\n` +
    `${agentName}\r\n`
  ));

  parts.push(Buffer.from(
    `--${boundary}\r\n` +
    `Content-Disposition: form-data; name="checksum"\r\n\r\n` +
    `${checksum}\r\n`
  ));

  if (env && env.length > 0) {
    parts.push(Buffer.from(
      `--${boundary}\r\n` +
      `Content-Disposition: form-data; name="env"\r\n\r\n` +
      `${JSON.stringify(env)}\r\n`
    ));
  }

  parts.push(Buffer.from(
    `--${boundary}\r\n` +
    `Content-Disposition: form-data; name="agent_tar"; filename="${agentName}.tar.gz"\r\n` +
    `Content-Type: application/gzip\r\n\r\n`
  ));
  parts.push(tarball);
  parts.push(Buffer.from('\r\n'));
  parts.push(Buffer.from(`--${boundary}--\r\n`));

  const body = Buffer.concat(parts);

  return new Promise((resolve, reject) => {
    let options: RequestOptions;
    let requestFn: typeof httpRequest;

    if (serverConfig.mode === 'unix_socket') {
      const socketPath = serverConfig.socket_path;
      if (!socketPath) {
        reject(new Error('Socket path not configured'));
        return;
      }

      options = {
        socketPath,
        path: '/v1/deploy',
        method: 'POST',
        headers: {
          'Content-Type': `multipart/form-data; boundary=${boundary}`,
          'Content-Length': body.length,
          'Accept': 'text/event-stream',
        },
      };
      requestFn = httpRequest;
    } else {
      const url = serverConfig.url;
      if (!url) {
        reject(new Error('Server URL not configured'));
        return;
      }

      const parsedUrl = new URL('/v1/deploy', url);
      const isHttps = parsedUrl.protocol === 'https:';

      options = {
        hostname: parsedUrl.hostname,
        port: parsedUrl.port || (isHttps ? 443 : 80),
        path: parsedUrl.pathname,
        method: 'POST',
        headers: {
          'Content-Type': `multipart/form-data; boundary=${boundary}`,
          'Content-Length': body.length,
          'Accept': 'text/event-stream',
          ...(serverConfig.auth_key && {
            Authorization: `Bearer ${serverConfig.auth_key}`,
          }),
        },
      };
      requestFn = isHttps ? httpsRequest : httpRequest;
    }

    const req = requestFn(options, (res) => {
      let buffer = '';
      let result: DeployResponse | null = null;

      res.on('data', (chunk: Buffer) => {
        buffer += chunk.toString();

        // Parse SSE events from buffer
        const lines = buffer.split('\n');
        buffer = lines.pop() || ''; // Keep incomplete line in buffer

        let currentEvent = '';
        for (const line of lines) {
          if (line.startsWith('event: ')) {
            currentEvent = line.slice(7).trim();
          } else if (line.startsWith('data: ')) {
            try {
              const data = JSON.parse(line.slice(6));

              if (currentEvent === 'deploy_progress' && onProgress) {
                onProgress({
                  phase: data.phase,
                  message: data.message,
                  progress: data.progress,
                });
              } else if (currentEvent === 'deploy_complete') {
                result = data as DeployResponse;
              } else if (currentEvent === 'deploy_error') {
                reject(new Error(data.error || 'Deploy failed'));
                return;
              }
            } catch {
              // Ignore parse errors for incomplete data
            }
            currentEvent = '';
          }
        }
      });

      res.on('end', () => {
        if (result) {
          resolve(result);
        } else if (res.statusCode && res.statusCode >= 200 && res.statusCode < 300) {
          // Fallback: try to parse as JSON if no SSE result
          try {
            const parsed = JSON.parse(buffer) as DeployResponse;
            resolve(parsed);
          } catch {
            reject(new Error('No valid response received'));
          }
        } else {
          let errorMessage = `HTTP ${res.statusCode}`;
          try {
            const errorData = JSON.parse(buffer);
            errorMessage = errorData.error || errorData.message || errorMessage;
          } catch {
            if (buffer) errorMessage = buffer;
          }
          reject(new Error(errorMessage));
        }
      });
    });

    req.on('error', (err: Error) => {
      if ((err as NodeJS.ErrnoException).code === 'ECONNREFUSED') {
        reject(new Error('Cannot connect to daemon. Is it running?'));
      } else if ((err as NodeJS.ErrnoException).code === 'ENOENT') {
        reject(new Error('Daemon socket not found. Is the daemon running?'));
      } else {
        reject(err);
      }
    });

    req.write(body);
    req.end();
  });
}

//@VALIDATE_AGENT_PATH
export function validateAgentPath(agentPath: string): { valid: boolean; error?: string; agentName?: string } {
  const resolvedPath = resolve(agentPath);

  try {
    const stats = statSync(resolvedPath);
    if (!stats.isDirectory()) {
      return { valid: false, error: 'Path is not a directory' };
    }
  } catch {
    return { valid: false, error: 'Path does not exist' };
  }

  // Check for agent.yaml
  const agentYamlPath = join(resolvedPath, 'agent.yaml');
  const agentYmlPath = join(resolvedPath, 'agent.yml');

  try {
    statSync(agentYamlPath);
  } catch {
    try {
      statSync(agentYmlPath);
    } catch {
      return { valid: false, error: 'agent.yaml not found in directory' };
    }
  }

  const agentName = basename(resolvedPath);

  // Validate agent name
  if (!agentName) {
    return { valid: false, error: 'Invalid agent name' };
  }
  if (agentName.includes('/') || agentName.includes('\\')) {
    return { valid: false, error: 'Agent name cannot contain path separators' };
  }
  if (agentName.includes('..')) {
    return { valid: false, error: 'Agent name cannot contain ..' };
  }
  if (agentName.startsWith('.')) {
    return { valid: false, error: 'Agent name cannot start with .' };
  }

  return { valid: true, agentName };
}
