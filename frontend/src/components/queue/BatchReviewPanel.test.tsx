import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { BatchReviewPanel } from './BatchReviewPanel';

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });

describe('BatchReviewPanel', () => {
  it('commits all selected scans with the shared direction and expiration date', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      jsonResponse({ updatedCount: 2, committedIds: ['a', 'b'] }),
    );
    vi.stubGlobal('fetch', fetchMock);
    const onComplete = vi.fn();
    render(
      <MantineProvider>
        <BatchReviewPanel selectedIds={['a', 'b']} onComplete={onComplete} />
      </MantineProvider>,
    );

    fireEvent.change(screen.getByLabelText('Expiration date'), { target: { value: '2026-04-15' } });
    fireEvent.click(screen.getByRole('button', { name: 'Commit 2 selected' }));

    await waitFor(() => expect(onComplete).toHaveBeenCalled());
    const [, request] = fetchMock.mock.calls[0];
    expect(JSON.parse(request?.body as string)).toEqual({
      scanEntryIds: ['a', 'b'],
      direction: 'stock_in',
      expiresAt: '2026-04-15T00:00:00.000Z',
      commit: true,
    });
  });
});
