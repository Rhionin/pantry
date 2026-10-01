import type { ComponentProps } from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { describe, expect, it, vi } from 'vitest';
import type { ProviderInfo, ShoppingListEntry } from '../../types';
import { ProvisionButton } from './ProvisionButton';

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
  connectionState: 'connected',
  credentialsConfigured: true,
  ...overrides,
});

const entry = (overrides: Partial<ShoppingListEntry> = {}): ShoppingListEntry => ({
  id: 'sli-1',
  itemId: 'item-1',
  quantity: 2,
  source: 'manual',
  purchasedAt: null,
  ...overrides,
});

const renderButton = (props: Partial<ComponentProps<typeof ProvisionButton>> = {}) => render(
  <MantineProvider>
    <ProvisionButton
      provider={props.provider === undefined ? provider() : props.provider}
      entries={props.entries ?? [entry()]}
      onFinished={props.onFinished ?? (() => undefined)}
    />
  </MantineProvider>,
);

describe('ProvisionButton', () => {
  it('is enabled for a connected configured provider with something to buy', () => {
    renderButton();
    expect(screen.getByRole('button', { name: 'Add to Kroger cart' })).toBeEnabled();
  });

  it('stays disabled when the provider is disconnected or unconfigured or the list is empty of quantity', () => {
    const { rerender } = renderButton({ provider: provider({ connectionState: 'disconnected' }) });
    expect(screen.getByRole('button', { name: 'Add to Kroger cart' })).toBeDisabled();

    rerender(
      <MantineProvider>
        <ProvisionButton provider={provider({ credentialsConfigured: false })} entries={[entry()]} onFinished={() => undefined} />
      </MantineProvider>,
    );
    expect(screen.getByRole('button', { name: 'Add to Kroger cart' })).toBeDisabled();
    expect(screen.getByText('Kroger is unconfigured.')).toBeInTheDocument();

    rerender(
      <MantineProvider>
        <ProvisionButton provider={provider()} entries={[entry({ quantity: 0 })]} onFinished={() => undefined} />
      </MantineProvider>,
    );
    expect(screen.getByRole('button', { name: 'Add to Kroger cart' })).toBeDisabled();
  });

  it('shows the confirmed count when nothing failed', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(new Response(JSON.stringify({
      provider: 'kroger',
      exported: 2,
      entries: [],
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))));
    renderButton();
    fireEvent.click(screen.getByRole('button', { name: 'Add to Kroger cart' }));
    expect(await screen.findByText('2 items sent to your cart.')).toBeInTheDocument();
  });

  it('shows an error with no confirmed count when the operation fails', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(new Response(JSON.stringify({
      error: 'Kroger is not connected',
    }), { status: 409, statusText: 'Conflict', headers: { 'Content-Type': 'application/json' } }))));
    renderButton();
    fireEvent.click(screen.getByRole('button', { name: 'Add to Kroger cart' }));
    expect(await screen.findByText('Kroger is not connected')).toBeInTheDocument();
    expect(screen.queryByText(/sent to your cart/)).not.toBeInTheDocument();
  });

  it('caps failed names and offers both resolutions for an unknown item', async () => {
    const failed = Array.from({ length: 52 }, (_, index) => `Failed ${index}`);
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(new Response(JSON.stringify({
      provider: 'kroger',
      exported: 0,
      failedItems: failed,
      unknownItems: ['Oats'],
      entries: [{ entryId: 'sli-oats', itemId: 'oats', name: 'Oats', quantity: 1, outcome: 'unknown' }],
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))));
    renderButton();
    fireEvent.click(screen.getByRole('button', { name: 'Add to Kroger cart' }));
    expect(await screen.findByText(/and 2 more/)).toBeInTheDocument();
    expect(screen.getByText(/These items remain on your shopping list/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'It reached Kroger' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'It did not' })).toBeInTheDocument();
    expect(screen.queryByText(/sent to your cart/)).not.toBeInTheDocument();
  });
});
