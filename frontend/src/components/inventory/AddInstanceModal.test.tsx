import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { describe, expect, it, vi } from 'vitest';
import { AddInstanceModal } from './AddInstanceModal';

const jsonResponse = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });

describe('AddInstanceModal', () => {
  it('posts the selected expiration date, closes, and requests a refresh', async () => {
    const fetchMock = vi.fn(() => Promise.resolve(jsonResponse({
      id: 'instance-1',
      itemId: 'item-1',
    }, 201)));
    vi.stubGlobal('fetch', fetchMock);
    const onClose = vi.fn();
    const onAdded = vi.fn();

    render(
      <MantineProvider>
        <AddInstanceModal
          itemId="item-1"
          opened
          onClose={onClose}
          onAdded={onAdded}
        />
      </MantineProvider>,
    );

    const submitButton = screen.getByRole('button', { name: 'Add instance' });
    expect(submitButton).toBeDisabled();
    fireEvent.change(screen.getByLabelText(/Expiration date/), {
      target: { value: '2026-04-15' },
    });
    fireEvent.click(submitButton);

    await waitFor(() => expect(onClose).toHaveBeenCalledOnce());
    expect(onAdded).toHaveBeenCalledOnce();
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/inventory/item-1/instances',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ expiresAt: '2026-04-15T00:00:00.000Z' }),
      }),
    );
  });

  it('keeps the modal open and displays the API error when creation fails', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(jsonResponse(
      { error: 'Expiration date is invalid.' },
      400,
    ))));
    const onClose = vi.fn();

    render(
      <MantineProvider>
        <AddInstanceModal
          itemId="item-1"
          opened
          onClose={onClose}
          onAdded={vi.fn()}
        />
      </MantineProvider>,
    );

    fireEvent.change(screen.getByLabelText(/Expiration date/), {
      target: { value: '2026-04-15' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Add instance' }));

    expect(await screen.findByText('Expiration date is invalid.')).toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
  });
});
