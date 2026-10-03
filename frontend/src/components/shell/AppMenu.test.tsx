import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ProviderInfo } from '../../types';
import { AppMenu } from './AppMenu';

const provider = (overrides: Partial<ProviderInfo> = {}): ProviderInfo => ({
  id: 'kroger',
  displayName: 'Kroger',
  capabilities: {
    auth: 'oauth2_authorization_code',
    delivery: 'server_push',
    confirmation: 'per_request',
    mutation: 'add_only',
    identity: 'derived',
  },
  connectionState: 'disconnected',
  credentialsConfigured: true,
  ...overrides,
});

const jsonResponse = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });

const renderMenu = (rows: ProviderInfo[], onCredentialsChanged: () => void = () => undefined) => {
  vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
    const url = String(input);
    if (url === '/api/providers') return Promise.resolve(jsonResponse(rows));
    if (url === '/api/build') return Promise.resolve(jsonResponse({ commit: 'abc123def456' }));
    return Promise.reject(new Error(`Unexpected request: ${url}`));
  }));
  return render(
    <MantineProvider>
      <MemoryRouter initialEntries={['/shopping']}>
        <Routes>
          <Route path="/shopping" element={<AppMenu onCredentialsChanged={onCredentialsChanged} />} />
          <Route path="/diagnostics" element={<h1>Diagnostics</h1>} />
        </Routes>
      </MemoryRouter>
    </MantineProvider>,
  );
};

const openMenu = async () => {
  fireEvent.click(screen.getByRole('button', { name: 'Menu' }));
  return screen.findByRole('menuitem', { name: 'Edit Kroger credentials' });
};

describe('AppMenu', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('keeps credentials, diagnostics, and the build id in the menu', async () => {
    renderMenu([provider({ connectionState: 'connected' })]);
    const credentials = await openMenu();
    expect(credentials).toBeEnabled();
    expect(screen.getByRole('menuitem', { name: 'Disconnect Kroger' })).toBeEnabled();
    expect(screen.getByRole('menuitem', { name: 'Diagnostics' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /setup/i })).not.toBeInTheDocument();
    expect(await screen.findByRole('note', { name: 'build abc123def456' })).toBeVisible();
    expect(screen.queryByLabelText('Client ID')).not.toBeInTheDocument();
  });

  it('opens the credentials dialog for a store that is not connected yet', async () => {
    renderMenu([provider({ credentialsConfigured: false, connectionState: 'disconnected' })]);
    const credentials = await openMenu();
    expect(screen.queryByRole('menuitem', { name: 'Disconnect Kroger' })).not.toBeInTheDocument();
    fireEvent.click(credentials);
    expect(await screen.findByRole('dialog', { name: 'Kroger credentials' })).toBeInTheDocument();
    expect(screen.getByLabelText('Client ID')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Menu' })).toBeInTheDocument();
  });

  it('disconnects a connected store from the menu', async () => {
    const onChanged = vi.fn();
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === '/api/providers') return Promise.resolve(jsonResponse([provider({ connectionState: 'connected' })]));
      if (url === '/api/providers/kroger/connection' && init?.method === 'DELETE') {
        return Promise.resolve(jsonResponse({}));
      }
      if (url === '/api/build') return Promise.resolve(jsonResponse({ commit: 'abc123def456' }));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    render(
      <MantineProvider>
        <MemoryRouter>
          <AppMenu onCredentialsChanged={onChanged} />
        </MemoryRouter>
      </MantineProvider>,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Menu' }));
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Disconnect Kroger' }));

    await waitFor(() => expect(onChanged).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith('/api/providers/kroger/connection', expect.objectContaining({ method: 'DELETE' }));
    await waitFor(() => expect(screen.queryByRole('menuitem', { name: 'Disconnect Kroger' })).not.toBeInTheDocument());
  });

  it('reports a disconnect failure and leaves the menu open', async () => {
    const onChanged = vi.fn();
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === '/api/providers') return Promise.resolve(jsonResponse([provider({ connectionState: 'connected' })]));
      if (url === '/api/providers/kroger/connection' && init?.method === 'DELETE') {
        return Promise.resolve(jsonResponse({ error: 'Kroger could not be disconnected' }, 500));
      }
      if (url === '/api/build') return Promise.resolve(jsonResponse({ commit: 'abc123def456' }));
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    }));
    render(
      <MantineProvider>
        <MemoryRouter>
          <AppMenu onCredentialsChanged={onChanged} />
        </MemoryRouter>
      </MantineProvider>,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Menu' }));
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Disconnect Kroger' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('Kroger could not be disconnected');
    expect(onChanged).not.toHaveBeenCalled();
    expect(screen.getByRole('menuitem', { name: 'Disconnect Kroger' })).toBeInTheDocument();
  });

  it('opens diagnostics from the menu', async () => {
    renderMenu([provider()]);
    await openMenu();
    fireEvent.click(screen.getByRole('menuitem', { name: 'Diagnostics' }));
    expect(await screen.findByRole('heading', { name: 'Diagnostics' })).toBeInTheDocument();
  });
});
