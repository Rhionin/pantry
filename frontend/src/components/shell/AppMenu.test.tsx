import { fireEvent, render, screen } from '@testing-library/react';
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

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });

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
    expect(screen.getByRole('menuitem', { name: 'Diagnostics' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /setup/i })).not.toBeInTheDocument();
    expect(await screen.findByRole('note', { name: 'build abc123def456' })).toBeVisible();
    expect(screen.queryByLabelText('Client ID')).not.toBeInTheDocument();
  });

  it('opens the credentials dialog for a store that is not connected yet', async () => {
    renderMenu([provider({ credentialsConfigured: false, connectionState: 'disconnected' })]);
    fireEvent.click(await openMenu());
    expect(await screen.findByRole('dialog', { name: 'Kroger credentials' })).toBeInTheDocument();
    expect(screen.getByLabelText('Client ID')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Menu' })).toBeInTheDocument();
  });

  it('opens diagnostics from the menu', async () => {
    renderMenu([provider()]);
    await openMenu();
    fireEvent.click(screen.getByRole('menuitem', { name: 'Diagnostics' }));
    expect(await screen.findByRole('heading', { name: 'Diagnostics' })).toBeInTheDocument();
  });
});
