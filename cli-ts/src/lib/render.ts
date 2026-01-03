//@RENDER_HELPER
import { render } from 'ink';
import type { ReactElement } from 'react';

export function renderApp(element: ReactElement): void {
  const { waitUntilExit } = render(element);
  waitUntilExit().catch(() => {
    process.exit(1);
  });
}

export async function renderAndWait(element: ReactElement): Promise<void> {
  const { waitUntilExit } = render(element);
  await waitUntilExit();
}
