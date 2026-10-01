import { fireEvent, render, screen } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { describe, expect, it, vi } from 'vitest';
import type { ProviderInfo } from '../../types';
import { ProviderPanel } from './ProviderPanel';

const provider = (overrides: Partial<ProviderInfo>): ProviderInfo => ({
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

const renderPanel = (providers: ProviderInfo[]) => render(
  <MantineProvider>
    <ProviderPanel providers={providers} onChanged={() => undefined} />
  </MantineProvider>,
);

describe('ProviderPanel', () => {
  it('offers Connect when the provider is configured and disconnected', () => {
    renderPanel([provider({ connectionState: 'disconnected' })]);
    expect(screen.getByRole('button', { name: 'Connect' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Disconnect' })).not.toBeInTheDocument();
  });

  it('explains an expired connection and offers Reconnect', () => {
    renderPanel([provider({ connectionState: 'reauth_required' })]);
    expect(screen.getByText(/connection expired/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Reconnect' })).toBeInTheDocument();
  });

  it('offers Disconnect and a new-cart control when connected', () => {
    renderPanel([provider({ connectionState: 'connected' })]);
    expect(screen.getByRole('button', { name: 'Disconnect' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Start a new cart' })).toBeInTheDocument();
  });

  it('marks an unconfigured provider and hides the connection control', () => {
    renderPanel([provider({ credentialsConfigured: false, connectionState: 'disconnected' })]);
    expect(screen.getByText('unconfigured')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Connect' })).not.toBeInTheDocument();
  });

  it('shows no connection control when a connection is not required', () => {
    renderPanel([provider({ connectionState: 'not_required' })]);
    expect(screen.queryByRole('button', { name: 'Connect' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Disconnect' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Start a new cart' })).toBeInTheDocument();
  });

  it('saves credentials without showing a stored secret', async () => {
    const onChanged = vi.fn();
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === '/api/providers/kroger/credentials' && init?.method === 'PUT') {
        expect(init.body).toBe(JSON.stringify({
          clientId: 'ui-client',
          clientSecret: 'typed-once',
          redirectUri: 'https://pantry.example/cb',
          modality: 'PICKUP',
        }));
        return Promise.resolve(new Response(JSON.stringify({
          clientId: 'ui-client',
          redirectUri: 'https://pantry.example/cb',
          modality: 'PICKUP',
          secretSet: true,
          source: 'saved',
        }), { status: 200, headers: { 'Content-Type': 'application/json' } }));
      }
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);
    render(
      <MantineProvider>
        <ProviderPanel
          providers={[provider({
            credentialsConfigured: false,
            credentials: {
              clientId: '',
              redirectUri: '',
              modality: 'PICKUP',
              secretSet: true,
              source: 'none',
            },
          })]}
          onChanged={onChanged}
        />
      </MantineProvider>,
    );
    expect(screen.getByLabelText('Client secret')).toHaveValue('');
    expect(screen.getByText('A client secret is saved and is not shown.')).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('Client ID'), { target: { value: 'ui-client' } });
    fireEvent.change(screen.getByLabelText('Client secret'), { target: { value: 'typed-once' } });
    fireEvent.change(screen.getByLabelText('Redirect URI'), { target: { value: 'https://pantry.example/cb' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save credentials' }));
    await screen.findByRole('button', { name: 'Save credentials' });
    expect(onChanged).toHaveBeenCalled();
  });

  it('starts the authorization redirect', async () => {
    const assign = vi.fn();
    vi.stubGlobal('location', { assign });
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(new Response(JSON.stringify({
      authorizationUrl: 'https://api.kroger.com/oauth?state=abc',
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))));
    renderPanel([provider({ connectionState: 'disconnected' })]);
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }));
    await screen.findByRole('button', { name: 'Connect' });
    expect(assign).toHaveBeenCalledWith('https://api.kroger.com/oauth?state=abc');
  });
});
