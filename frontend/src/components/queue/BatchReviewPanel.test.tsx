import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { BatchReviewPanel } from './BatchReviewPanel';

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });

describe('BatchReviewPanel', () => {
  it('renders the approve heading, button with correct text and aria-label', () => {
    render(
      <MantineProvider>
        <BatchReviewPanel selectedIds={['a', 'b']} onComplete={vi.fn()} />
      </MantineProvider>,
    );

    expect(screen.getByRole('heading', { level: 2, name: 'Approve scans' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Approve 2 selected scans' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Approve 2 selected scans' })).toHaveAttribute(
      'aria-label',
      'Approve 2 selected scans',
    );
  });

  it('shows error message on simulated failure', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockRejectedValue(new Error('Network error'));
    vi.stubGlobal('fetch', fetchMock);
    const onComplete = vi.fn();

    render(
      <MantineProvider>
        <BatchReviewPanel selectedIds={['a', 'b']} onComplete={onComplete} />
      </MantineProvider>,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Approve 2 selected scans' }));

    await waitFor(() =>
      expect(screen.getByText('Unable to approve selected scans.')).toBeInTheDocument(),
    );
  });
});
