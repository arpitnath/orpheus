//@LIMA_VM_CONTROL
import { execSync, spawn } from 'node:child_process';
import { platform } from 'node:os';

const VM_NAME = 'orpheus';

export interface VMStatus {
  exists: boolean;
  status: 'Running' | 'Stopped' | 'Unknown';
  arch?: string;
  cpus?: number;
  memory?: string;
  disk?: string;
}

export function checkLimaInstalled(): boolean {
  try {
    execSync('which limactl', { stdio: 'ignore' });
    return true;
  } catch {
    return false;
  }
}

export function checkMacOS(): boolean {
  return platform() === 'darwin';
}

export function getVMStatus(): VMStatus {
  if (!checkMacOS()) {
    return { exists: false, status: 'Unknown' };
  }

  if (!checkLimaInstalled()) {
    return { exists: false, status: 'Unknown' };
  }

  try {
    const output = execSync('limactl list --json', { encoding: 'utf-8' });
    // limactl list --json can return either a single object or array
    const parsed = JSON.parse(output);
    const vms = Array.isArray(parsed) ? parsed : [parsed];
    const orpheusVm = vms.find((vm: Record<string, unknown>) => vm.name === VM_NAME);

    if (!orpheusVm) {
      return { exists: false, status: 'Unknown' };
    }

    return {
      exists: true,
      status: orpheusVm.status as 'Running' | 'Stopped',
      arch: orpheusVm.arch as string,
      cpus: orpheusVm.cpus as number,
      memory: orpheusVm.memory ? `${Math.round((orpheusVm.memory as number) / 1024 / 1024 / 1024)}GB` : undefined,
      disk: orpheusVm.disk ? `${Math.round((orpheusVm.disk as number) / 1024 / 1024 / 1024)}GB` : undefined,
    };
  } catch {
    return { exists: false, status: 'Unknown' };
  }
}

export function startVM(): void {
  console.log('Starting Lima VM...');
  execSync(`limactl start ${VM_NAME}`, { stdio: 'inherit' });
}

export function stopVM(): void {
  console.log('Stopping Lima VM...');
  execSync(`limactl stop ${VM_NAME}`, { stdio: 'inherit' });
}

export function deleteVM(): void {
  console.log('Deleting Lima VM...');
  execSync(`limactl delete ${VM_NAME}`, { stdio: 'inherit' });
}

export function sshVM(): void {
  const child = spawn('limactl', ['shell', VM_NAME], {
    stdio: 'inherit',
  });

  child.on('exit', (code) => {
    process.exit(code ?? 0);
  });
}
