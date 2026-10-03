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
  return screen.findByRole('menuitem', { name: 'Manage Kroger connection' });
};

describe('AppMenu', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('keeps the connection action, diagnostics, and a labeled build section in the menu', async () => {
    renderMenu([provider({ connectionState: 'connected' })]);
    const connection = await openMenu();
    expect(connection).toBeEnabled();
    expect(screen.queryByRole('menuitem', { name: 'Edit Kroger credentials' })).not.toBeInTheDocument();
    expect(screen.queryByRole('menuitem', { name: 'Disconnect Kroger' })).not.toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: 'Diagnostics' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /setup/i })).not.toBeInTheDocument();
    const note = await screen.findByRole('note', { name: 'build abc123def456' });
    expect(note).toBeVisible();
    expect(screen.getByText('Build')).toBeVisible();
    expect(screen.queryByRole('menuitem', { name: 'build abc123def456' })).not.toBeInTheDocument();
    expect(screen.queryByRole('menuitem', { name: 'Build' })).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Client ID')).not.toBeInTheDocument();
  });

  it('opens credentials from the connection item without leaving the page', async () => {
    renderMenu([provider({ credentialsConfigured: false, connectionState: 'disconnected' })]);
    const connection = await openMenu();
    expect(screen.queryByRole('menuitem', { name: 'Disconnect Kroger' })).not.toBeInTheDocument();
    fireEvent.click(connection);
    expect(await screen.findByRole('dialog', { name: 'Kroger connection' })).toBeInTheDocument();
    expect(screen.getByRole('form', { name: 'Kroger credentials' })).toBeInTheDocument();
    expect(screen.getByLabelText('Client ID')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Disconnect Kroger' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Menu' })).toBeInTheDocument();
  });

  it('reaches disconnect from the connection item only while connected', async () => {
    renderMenu([provider({ connectionState: 'connected' })]);
    fireEvent.click(await openMenu());
    expect(await screen.findByRole('dialog', { name: 'Kroger connection' })).toBeInTheDocument();
    expect(screen.getByLabelText('Client ID')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Disconnect Kroger' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Menu' })).toBeInTheDocument();
  });

  it('disconnects a connected store from the connection dialog', async () => {
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
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Manage Kroger connection' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Disconnect Kroger' }));

    await waitFor(() => expect(onChanged).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith('/api/providers/kroger/connection', expect.objectContaining({ method: 'DELETE' }));
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Disconnect Kroger' })).not.toBeInTheDocument());
    expect(screen.queryByRole('dialog', { name: 'Kroger connection' })).not.toBeInTheDocument();
  });

  it('reports a disconnect failure and leaves the connection dialog open', async () => {
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
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Manage Kroger connection' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Disconnect Kroger' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('Kroger could not be disconnected');
    expect(onChanged).not.toHaveBeenCalled();
    expect(screen.getByRole('dialog', { name: 'Kroger connection' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Disconnect Kroger' })).toBeInTheDocument();
    expect(screen.getByLabelText('Client ID')).toBeInTheDocument();
  });

  it('opens diagnostics from the menu', async () => {
    renderMenu([provider()]);
    await openMenu();
    fireEvent.click(screen.getByRole('menuitem', { name: 'Diagnostics' }));
    expect(await screen.findByRole('heading', { name: 'Diagnostics' })).toBeInTheDocument();
  });
});
