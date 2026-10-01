import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { BuildStamp } from './BuildStamp';

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });

const renderStamp = () =>
  render(
    <MantineProvider>
      <BuildStamp />
    </MantineProvider>,
  );

describe('BuildStamp', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('shows the commit, timestamp, and subject returned by the build endpoint', async () => {
    const commit = '0123456789abcdef0123456789abcdef01234567';
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        jsonResponse({
          commit,
          committedAt: '2026-10-01T04:32:19Z',
          subject: 'Show the running commit in a quiet footer',
        }),
      ),
    );

    renderStamp();

    const note = await screen.findByRole('note', {
      name: `Show the running commit in a quiet footer, 2026-10-01 04:32 UTC, build ${commit}`,
    });
    expect(note).toHaveTextContent('Show the running commit in a quiet footer');
    expect(note).toHaveTextContent(`2026-10-01 04:32 UTC · ${commit}`);
  });

  it('shows only the commit when time and subject are absent', async () => {
    const commit = '0123456789abcdef0123456789abcdef01234567';
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({ commit })));

    renderStamp();

    const note = await screen.findByRole('note', { name: `build ${commit}` });
    expect(note).toHaveTextContent(`build ${commit}`);
  });

  it('stays quiet when the build endpoint fails', async () => {
    const fetchMock = vi.fn().mockRejectedValue(new Error('offline'));
    vi.stubGlobal('fetch', fetchMock);

    renderStamp();

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(screen.queryByRole('note')).not.toBeInTheDocument();
  });

  it('stays quiet when the commit is empty', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ commit: '' }));
    vi.stubGlobal('fetch', fetchMock);

    renderStamp();

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    await act(async () => {
      await Promise.resolve();
    });
    expect(screen.queryByRole('note')).not.toBeInTheDocument();
  });
});
