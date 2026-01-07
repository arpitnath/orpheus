import { useEffect } from 'react';
import { useStdin } from 'ink';

/**
 * Check if raw mode is supported and return a boolean.
 */
export function useIsInteractive(): boolean {
  const { isRawModeSupported } = useStdin();
  return isRawModeSupported;
}

/**
 * Hook to handle manual refresh via 'r' key press.
 * Only active when raw mode is supported (interactive terminal).
 * @param refetch - Function to call when 'r' is pressed
 */
export function useRefresh(refetch: () => void): void {
  const { isRawModeSupported, stdin, setRawMode } = useStdin();

  useEffect(() => {
    if (!isRawModeSupported || !stdin) return;

    setRawMode(true);

    const handleData = (data: Buffer) => {
      const char = data.toString();
      if (char === 'r' || char === 'R') {
        refetch();
      }
    };

    stdin.on('data', handleData);

    return () => {
      stdin.off('data', handleData);
      setRawMode(false);
    };
  }, [isRawModeSupported, stdin, setRawMode, refetch]);
}

/**
 * Hook to handle quit via 'q' key press.
 * Only active when raw mode is supported (interactive terminal).
 * @param onQuit - Function to call when 'q' is pressed
 */
export function useQuit(onQuit: () => void): void {
  const { isRawModeSupported, stdin, setRawMode } = useStdin();

  useEffect(() => {
    if (!isRawModeSupported || !stdin) return;

    setRawMode(true);

    const handleData = (data: Buffer) => {
      const char = data.toString();
      if (char === 'q' || char === 'Q' || char === '\x1b') {
        onQuit();
      }
    };

    stdin.on('data', handleData);

    return () => {
      stdin.off('data', handleData);
      setRawMode(false);
    };
  }, [isRawModeSupported, stdin, setRawMode, onQuit]);
}

/**
 * Combined hook for refresh and quit functionality.
 * Only active when raw mode is supported (interactive terminal).
 * @param refetch - Function to call when 'r' is pressed
 * @param onQuit - Function to call when 'q' is pressed
 */
export function useRefreshAndQuit(refetch: () => void, onQuit: () => void): void {
  const { isRawModeSupported, stdin, setRawMode } = useStdin();

  useEffect(() => {
    if (!isRawModeSupported || !stdin) return;

    setRawMode(true);

    const handleData = (data: Buffer) => {
      const char = data.toString();
      if (char === 'r' || char === 'R') {
        refetch();
      }
      if (char === 'q' || char === 'Q' || char === '\x1b') {
        onQuit();
      }
    };

    stdin.on('data', handleData);

    return () => {
      stdin.off('data', handleData);
      setRawMode(false);
    };
  }, [isRawModeSupported, stdin, setRawMode, refetch, onQuit]);
}
