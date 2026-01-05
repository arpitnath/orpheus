import { useState, useEffect, useCallback } from 'react';
import { useStdin } from 'ink';

export interface UseTabNavigationResult {
  activeTab: number;
  isInteractive: boolean;
}

interface UseTabNavigationOptions {
  tabCount: number;
  onQuit: () => void;
}

/**
 * Hook for tab navigation in inspect views.
 * Handles Tab key to cycle tabs, Esc/q to quit.
 */
export function useTabNavigation({
  tabCount,
  onQuit,
}: UseTabNavigationOptions): UseTabNavigationResult {
  const { isRawModeSupported, stdin, setRawMode } = useStdin();
  const [activeTab, setActiveTab] = useState(0);

  const handleKeypress = useCallback(
    (data: Buffer) => {
      const key = data.toString();

      // Tab key - cycle to next tab
      if (key === '\t') {
        setActiveTab((prev) => (prev + 1) % tabCount);
        return;
      }

      // q or Escape - quit
      if (key.toLowerCase() === 'q' || key === '\x1b') {
        onQuit();
        return;
      }
    },
    [tabCount, onQuit]
  );

  useEffect(() => {
    if (!isRawModeSupported || !stdin) return;

    setRawMode(true);
    stdin.on('data', handleKeypress);

    return () => {
      stdin.off('data', handleKeypress);
      setRawMode(false);
    };
  }, [isRawModeSupported, stdin, setRawMode, handleKeypress]);

  return {
    activeTab,
    isInteractive: isRawModeSupported,
  };
}
