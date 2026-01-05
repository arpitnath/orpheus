import { useState, useEffect, useCallback } from 'react';
import { useStdin } from 'ink';

export interface UseKeyboardNavigationResult {
  selectedIndex: number;
  expandedIndex: number | null;
  isInteractive: boolean;
}

interface UseKeyboardNavigationOptions {
  itemCount: number;
  onEnter?: (index: number) => void;
  onRefresh?: () => void;
  onQuit?: () => void;
}

/**
 * Hook for keyboard navigation in lists.
 * Handles ↑/↓ for navigation, →/← for expand/collapse, Enter for action, r for refresh, q for quit.
 */
export function useKeyboardNavigation({
  itemCount,
  onEnter,
  onRefresh,
  onQuit,
}: UseKeyboardNavigationOptions): UseKeyboardNavigationResult {
  const { isRawModeSupported, stdin, setRawMode } = useStdin();
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [expandedIndex, setExpandedIndex] = useState<number | null>(null);

  // Keep selectedIndex in bounds when itemCount changes
  useEffect(() => {
    if (selectedIndex >= itemCount && itemCount > 0) {
      setSelectedIndex(itemCount - 1);
    }
  }, [itemCount, selectedIndex]);

  const handleKeypress = useCallback(
    (data: Buffer) => {
      const key = data.toString();

      // Handle arrow keys (escape sequences)
      if (key === '\x1b[A') {
        // Up arrow
        setSelectedIndex((prev) => Math.max(0, prev - 1));
        return;
      }
      if (key === '\x1b[B') {
        // Down arrow
        setSelectedIndex((prev) => Math.min(itemCount - 1, prev + 1));
        return;
      }
      if (key === '\x1b[C') {
        // Right arrow - expand
        setExpandedIndex(selectedIndex);
        return;
      }
      if (key === '\x1b[D') {
        // Left arrow - collapse
        setExpandedIndex(null);
        return;
      }

      // Handle single character keys
      const char = key.toLowerCase();

      if (char === '\r' || char === '\n') {
        // Enter
        onEnter?.(selectedIndex);
        return;
      }
      if (char === 'r') {
        onRefresh?.();
        return;
      }
      if (char === 'q' || key === '\x1b') {
        // q or Escape
        onQuit?.();
        return;
      }
    },
    [itemCount, selectedIndex, onEnter, onRefresh, onQuit]
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
    selectedIndex,
    expandedIndex,
    isInteractive: isRawModeSupported,
  };
}
