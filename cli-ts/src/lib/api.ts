//@IMPORTS
import { request as httpRequest } from 'node:http';
import { request as httpsRequest, RequestOptions } from 'node:https';
import { URL } from 'node:url';
import { getActiveServer } from './config.js';
import type {
  ServerConfig,
  HealthResponse,
  StatsResponse,
  DeployResponse,
  InvokeResponse,
  AgentListItem,
  OrpheusClient,
  DeployOptions,
} from '../types/index.js';

//@HTTP_CLIENT
async function makeRequest<T>(
  method: string,
  path: string,
  serverConfig: ServerConfig,
  body?: unknown,
  timeout: number = 600000 // 600s default for long agent executions
): Promise<T> {
  return new Promise((resolve, reject) => {
    let options: RequestOptions;
    let requestFn: typeof httpRequest;

    if (serverConfig.mode === 'unix_socket') {
      // Unix socket mode
      const socketPath = serverConfig.socket_path;
      if (!socketPath) {
        reject(new Error('Socket path not configured'));
        return;
      }

      options = {
        socketPath,
        path,
        method,
        headers: {
          'Content-Type': 'application/json',
        },
        timeout,
      };
      requestFn = httpRequest;
    } else {
      // TCP mode
      const url = serverConfig.url;
      if (!url) {
        reject(new Error('Server URL not configured'));
        return;
      }

      const parsedUrl = new URL(path, url);
      const isHttps = parsedUrl.protocol === 'https:';

      options = {
        hostname: parsedUrl.hostname,
        port: parsedUrl.port || (isHttps ? 443 : 80),
        path: parsedUrl.pathname + parsedUrl.search,
        method,
        headers: {
          'Content-Type': 'application/json',
          ...(serverConfig.auth_key && {
            Authorization: `Bearer ${serverConfig.auth_key}`,
          }),
        },
        timeout,
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
            const parsed = data ? JSON.parse(data) : {};
            resolve(parsed as T);
          } catch {
            resolve(data as unknown as T);
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

    req.on('timeout', () => {
      req.destroy();
      reject(new Error('Request timed out'));
    });

    if (body) {
      req.write(JSON.stringify(body));
    }

    req.end();
  });
}

//@CLIENT_FACTORY
export function createClient(_serverName?: string): OrpheusClient {
  // For now, we just use the active server
  // TODO: Support _serverName parameter to switch servers
  const serverConfig = getActiveServer();

  return {
    async health(): Promise<HealthResponse> {
      return makeRequest<HealthResponse>('GET', '/v1/health', serverConfig, undefined, 5000);
    },

    async stats(agentName?: string): Promise<StatsResponse> {
      const path = agentName ? `/v1/stats?agent=${encodeURIComponent(agentName)}` : '/v1/stats';
      return makeRequest<StatsResponse>('GET', path, serverConfig);
    },

    async deploy(_agentPath: string, _options?: DeployOptions): Promise<DeployResponse> {
      // TODO: Implement deploy with multipart form data
      // This will need to:
      // 1. Create tar archive of agent directory
      // 2. Calculate checksum
      // 3. Upload via multipart form
      throw new Error('Deploy not yet implemented in TypeScript CLI');
    },

    async invoke(agentName: string, input: unknown): Promise<InvokeResponse> {
      return makeRequest<InvokeResponse>(
        'POST',
        `/v1/agents/${encodeURIComponent(agentName)}/run`,
        serverConfig,
        { input }
      );
    },

    async undeploy(agentName: string): Promise<{ success: boolean; message?: string }> {
      return makeRequest<{ success: boolean; message?: string }>(
        'DELETE',
        `/v1/agents/${encodeURIComponent(agentName)}`,
        serverConfig
      );
    },

    async list(): Promise<AgentListItem[]> {
      const response = await makeRequest<{ agents: AgentListItem[] }>(
        'GET',
        '/v1/agents',
        serverConfig
      );
      return response.agents || [];
    },

    close(): void {
      // no-op for HTTP
    },
  };
}

//@CONVENIENCE_FUNCTIONS
export async function testConnection(): Promise<boolean> {
  try {
    const client = createClient();
    await client.health();
    return true;
  } catch {
    return false;
  }
}

export async function getHealth(): Promise<HealthResponse | null> {
  try {
    const client = createClient();
    return await client.health();
  } catch {
    return null;
  }
}

export async function getStats(agentName?: string): Promise<StatsResponse | null> {
  try {
    const client = createClient();
    return await client.stats(agentName);
  } catch {
    return null;
  }
}
