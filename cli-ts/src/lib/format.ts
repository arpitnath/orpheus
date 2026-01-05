//@FORMAT_UTILS
// Centralized formatting utilities for CLI output

//@COLORS
export const c = {
  green: '\x1b[32m',
  red: '\x1b[31m',
  yellow: '\x1b[33m',
  cyan: '\x1b[36m',
  blue: '\x1b[34m',
  magenta: '\x1b[35m',
  dim: '\x1b[2m',
  bold: '\x1b[1m',
  reset: '\x1b[0m',
};

//@SYMBOLS
export const sym = {
  check: '\u2713',      // ✓
  cross: '\u2717',      // ✗
  bullet: '\u2022',     // •
  arrow: '\u2192',      // →
  line: '\u2500',       // ─
  doubleLine: '\u2550', // ═
  corner: {
    tl: '\u250c',       // ┌
    tr: '\u2510',       // ┐
    bl: '\u2514',       // └
    br: '\u2518',       // ┘
  },
  vertical: '\u2502',   // │
  tee: {
    left: '\u251c',     // ├
    right: '\u2524',    // ┤
  },
  circle: {
    empty: '\u25cb',    // ○
    filled: '\u25cf',   // ●
  },
};

//@OUTPUT_HELPERS
export const ok = (msg: string) => `${c.green}${sym.check}${c.reset} ${msg}`;
export const err = (msg: string) => `${c.red}${sym.cross}${c.reset} ${msg}`;
export const warn = (msg: string) => `${c.yellow}!${c.reset} ${msg}`;
export const info = (msg: string) => `${c.cyan}${sym.bullet}${c.reset} ${msg}`;

//@LABEL_VALUE
export const label = (key: string, value: string, keyWidth = 12) =>
  `${c.dim}${key.padEnd(keyWidth)}${c.reset}${value}`;

//@TABLE
export function table(headers: string[], rows: string[][], widths: number[]): string {
  const headerLine = headers.map((h, i) => h.padEnd(widths[i])).join('');
  const separator = sym.line.repeat(widths.reduce((a, b) => a + b, 0));
  const bodyLines = rows.map(row =>
    row.map((cell, i) => cell.padEnd(widths[i])).join('')
  );
  return [headerLine, separator, ...bodyLines].join('\n');
}

//@BOX
export function box(title: string, content: string): string {
  const lines = content.split('\n');
  const maxLen = Math.max(title.length, ...lines.map(l => stripAnsi(l).length));
  const width = maxLen + 2;

  const top = `${sym.corner.tl}${sym.line.repeat(width)}${sym.corner.tr}`;
  const titleLine = `${sym.vertical} ${c.bold}${title.padEnd(maxLen)}${c.reset} ${sym.vertical}`;
  const sep = `${sym.tee.left}${sym.line.repeat(width)}${sym.tee.right}`;
  const body = lines.map(l => {
    const stripped = stripAnsi(l);
    const padding = maxLen - stripped.length;
    return `${sym.vertical} ${l}${' '.repeat(padding)} ${sym.vertical}`;
  }).join('\n');
  const bottom = `${sym.corner.bl}${sym.line.repeat(width)}${sym.corner.br}`;

  return [top, titleLine, sep, body, bottom].join('\n');
}

//@STRIP_ANSI
function stripAnsi(str: string): string {
  // eslint-disable-next-line no-control-regex
  return str.replace(/\x1b\[[0-9;]*m/g, '');
}

//@STATUS_DOT
export const statusDot = (status: string): string => {
  switch (status.toLowerCase()) {
    case 'running':
    case 'healthy':
    case 'ok':
      return `${c.green}${sym.circle.filled}${c.reset}`;
    case 'idle':
    case 'pending':
      return `${c.yellow}${sym.circle.filled}${c.reset}`;
    case 'error':
    case 'failed':
    case 'unhealthy':
      return `${c.red}${sym.circle.filled}${c.reset}`;
    default:
      return `${c.dim}${sym.circle.empty}${c.reset}`;
  }
};

//@TIME_FORMATTING
export function formatUptime(seconds: number): string {
  if (seconds < 0 || !Number.isFinite(seconds)) return '0s';

  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = Math.floor(seconds % 60);

  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  }
  if (minutes > 0) {
    return `${minutes}m ${secs}s`;
  }
  return `${secs}s`;
}

//@BYTE_FORMATTING
export function formatBytes(bytes: number): string {
  if (bytes < 0 || !Number.isFinite(bytes)) return '0 B';

  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let unitIndex = 0;
  let size = bytes;

  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex++;
  }

  return `${size.toFixed(unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`;
}

//@RELATIVE_TIME
export function formatRelativeTime(date: string | Date): string {
  const now = new Date();
  const then = typeof date === 'string' ? new Date(date) : date;
  const diffMs = now.getTime() - then.getTime();

  if (diffMs < 0) return 'in the future';

  const seconds = Math.floor(diffMs / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);

  if (days > 0) return `${days} day${days > 1 ? 's' : ''} ago`;
  if (hours > 0) return `${hours} hour${hours > 1 ? 's' : ''} ago`;
  if (minutes > 0) return `${minutes} minute${minutes > 1 ? 's' : ''} ago`;
  return 'just now';
}

//@STRING_UTILS
export function truncate(str: string, maxLen: number): string {
  if (!str || str.length <= maxLen) return str;
  return str.slice(0, maxLen - 3) + '...';
}
