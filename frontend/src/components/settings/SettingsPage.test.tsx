import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { SettingsPage } from './SettingsPage';

const jsonResponse = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });

const settings = { months: 3, opening: false, wipePhrase: 'WIPE INVENTORY' };

describe('SettingsPage', () => {
  afterEach(() => {
    localStorage.clear();
    vi.unstubAllGlobals();
  });

  it('saves a supply length between 1 and 12 months', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? 'GET';
      if (url === '/api/settings/supply' && method === 'PUT') {
        return Promise.resolve(jsonResponse({ ...settings, months: 6 }));
      }
      if (url === '/api/settings/supply') {
        return Promise.resolve(jsonResponse(settings));
      }
      throw new Error(`Unexpected request: ${method} ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><SettingsPage /></MantineProvider>);
    const months = await screen.findByLabelText('Months of supply');
    fireEvent.change(months, { target: { value: '6' } });
    fireEvent.blur(months);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      '/api/settings/supply',
      expect.objectContaining({ method: 'PUT', body: JSON.stringify({ months: 6 }) }),
    ));
  });

  it('requires the exact confirmation phrase before wiping inventory', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? 'GET';
      if (url === '/api/inventory/wipe' && method === 'POST') {
        return Promise.resolve(jsonResponse({}));
      }
      if (url === '/api/settings/supply') {
        return Promise.resolve(jsonResponse(settings));
      }
      throw new Error(`Unexpected request: ${method} ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><SettingsPage /></MantineProvider>);
    await screen.findByLabelText('Months of supply');

    fireEvent.click(screen.getByRole('button', { name: 'Wipe inventory' }));
    const confirmButton = await screen.findByRole('button', { name: 'Confirm wipe' });
    expect(confirmButton).toBeDisabled();

    fireEvent.change(screen.getByLabelText('Type WIPE INVENTORY to confirm'), {
      target: { value: 'wipe inventory' },
    });
    expect(confirmButton).toBeDisabled();

    fireEvent.change(screen.getByLabelText('Type WIPE INVENTORY to confirm'), {
      target: { value: 'WIPE INVENTORY' },
    });
    expect(confirmButton).toBeEnabled();
    fireEvent.click(confirmButton);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/inventory/wipe', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ confirmation: 'WIPE INVENTORY' }),
    })));
  });

  it('keeps the page up when a wipe is rejected', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? 'GET';
      if (url === '/api/inventory/wipe' && method === 'POST') {
        return Promise.resolve(jsonResponse({ error: 'Type WIPE INVENTORY to confirm wiping the inventory.' }, 400));
      }
      if (url === '/api/settings/supply') {
        return Promise.resolve(jsonResponse(settings));
      }
      throw new Error(`Unexpected request: ${method} ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(<MantineProvider><SettingsPage /></MantineProvider>);
    await screen.findByLabelText('Months of supply');

    fireEvent.click(screen.getByRole('button', { name: 'Wipe inventory' }));
    fireEvent.change(await screen.findByLabelText('Type WIPE INVENTORY to confirm'), {
      target: { value: 'WIPE INVENTORY' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Confirm wipe' }));

    expect(await screen.findByText('Type WIPE INVENTORY to confirm wiping the inventory.')).toBeInTheDocument();
    expect(screen.getByLabelText('Months of supply')).toBeInTheDocument();
  });

  it('shows scan alerts, on by default, and the note when this browser cannot notify', async () => {
    vi.stubGlobal('Notification', undefined);
    vi.stubGlobal('fetch', (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === '/api/settings/supply') return Promise.resolve(jsonResponse(settings));
      if (url === '/api/settings/group-rule') return Promise.resolve(jsonResponse({ rule: 'same_as_ran_out' }));
      throw new Error(`Unexpected request: ${url}`);
    });

    render(<MantineProvider><SettingsPage /></MantineProvider>);

    expect(await screen.findByRole('switch', { name: 'Scan alerts' })).toBeChecked();
    expect(screen.getByRole('status')).toHaveTextContent(
      'This browser cannot show notifications from an open tab.',
    );
  });
});
