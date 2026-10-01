import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { ProductEditor } from './ProductEditor';

beforeEach(() => {
  vi.stubGlobal(
    'matchMedia',
    (query: string) =>
      ({
        matches: /prefers-reduced-motion/.test(query),
        media: query,
        onchange: null,
        addListener: () => {},
        removeListener: () => {},
        addEventListener: () => {},
        removeEventListener: () => {},
        dispatchEvent: () => false,
      }) as unknown as MediaQueryList,
  );
});

const jsonResponse = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });

describe('ProductEditor', () => {
  it('saves an edit with a contribution only after both opt-ins', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/products/prod-1' && request?.method === undefined) {
        return Promise.resolve(jsonResponse({
          id: 'prod-1',
          name: 'House Sponge',
          category: 'Cleaning',
          unitOfMeasure: 'each',
          createdAt: '2026-03-20T10:00:00Z',
          source: 'user',
          barcodes: ['012345678905'],
        }));
      }
      if (url === '/api/settings/contribution' && request?.method === 'PUT') {
        return Promise.resolve(jsonResponse({ enabled: true, configured: false }));
      }
      if (url === '/api/settings/contribution') {
        return Promise.resolve(jsonResponse({ enabled: false, configured: false }));
      }
      if (url === '/api/products/prod-1' && request?.method === 'PUT') {
        return Promise.resolve(jsonResponse({
          id: 'prod-1',
          name: 'Scrub Sponge',
          category: 'Cleaning',
          unitOfMeasure: 'each',
          createdAt: '2026-03-20T10:00:00Z',
          contribution: {
            status: 'not_configured',
            detail: 'Saved in your pantry. Nothing was sent because this Pantry is not signed in to the open databases.',
          },
        }));
      }
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    const onSaved = vi.fn();

    render(
      <MantineProvider theme={{ respectReducedMotion: true }}>
        <ProductEditor productId="prod-1" onSaved={onSaved} />
      </MantineProvider>,
    );

    expect(fetchMock).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Edit product' }));
    expect(await screen.findByDisplayValue('House Sponge')).toBeInTheDocument();

    fireEvent.click(await screen.findByRole('switch', { name: 'Let me contribute products I type in' }));
    fireEvent.click(await screen.findByRole('checkbox', { name: 'Contribute this product' }));
    fireEvent.change(screen.getByRole('textbox', { name: /^Product name/ }), { target: { value: 'Scrub Sponge' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save product' }));

    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/products/prod-1',
      expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({
          name: 'Scrub Sponge',
          category: 'Cleaning',
          unitOfMeasure: 'each',
          contribute: true,
          contributeTo: 'openfoodfacts',
          barcode: '012345678905',
        }),
      }),
    );
  });
});