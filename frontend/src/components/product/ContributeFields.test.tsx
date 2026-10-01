import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { ContributeFields, type ContributeChoice } from './ContributeFields';

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

const Harness = ({
  allowProductOptIn = true,
  onChange,
}: {
  allowProductOptIn?: boolean;
  onChange: (choice: ContributeChoice) => void;
}) => (
  <MantineProvider theme={{ respectReducedMotion: true }}>
    <ContributeFields allowProductOptIn={allowProductOptIn} onChange={onChange} />
  </MantineProvider>
);

describe('ContributeFields', () => {
  it('stays off and hides the per-product checkbox until sharing is turned on', async () => {
    const onChange = vi.fn();
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(jsonResponse({ enabled: false, configured: false })),
    );

    render(<Harness onChange={onChange} />);

    const share = await screen.findByRole('switch', { name: 'Let me contribute products I type in' });
    expect(share).not.toBeChecked();
    expect(screen.queryByRole('checkbox', { name: 'Contribute this product' })).not.toBeInTheDocument();
    await waitFor(() => expect(onChange).toHaveBeenCalledWith({
      contribute: false,
      contributeTo: 'openfoodfacts',
    }));
  });

  it('keeps the product unchecked after the household switch is turned on', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/settings/contribution' && request?.method === 'PUT') {
        return Promise.resolve(jsonResponse({ enabled: true, configured: false }));
      }
      if (url === '/api/settings/contribution') {
        return Promise.resolve(jsonResponse({ enabled: false, configured: false }));
      }
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);

    fireEvent.click(await screen.findByRole('switch', { name: 'Let me contribute products I type in' }));

    const productOptIn = await screen.findByRole('checkbox', { name: 'Contribute this product' });
    expect(productOptIn).not.toBeChecked();
    expect(screen.getByText(/not signed in to the open databases/)).toBeInTheDocument();
    await waitFor(() => expect(onChange).toHaveBeenLastCalledWith({
      contribute: false,
      contributeTo: 'openfoodfacts',
    }));
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/settings/contribution',
      expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({ enabled: true }),
      }),
    );
  });

  it('reports a contribution only after the product checkbox is checked', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(jsonResponse({ enabled: true, configured: true })),
    );
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);

    const productOptIn = await screen.findByRole('checkbox', { name: 'Contribute this product' });
    expect(productOptIn).not.toBeChecked();
    fireEvent.click(productOptIn);
    fireEvent.change(screen.getByLabelText('Open database'), { target: { value: 'openproductsfacts' } });

    await waitFor(() => expect(onChange).toHaveBeenLastCalledWith({
      contribute: true,
      contributeTo: 'openproductsfacts',
    }));
  });

  it('does not offer to contribute a product that already came from upstream', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(jsonResponse({ enabled: true, configured: false })),
    );
    render(<Harness allowProductOptIn={false} onChange={vi.fn()} />);

    expect(await screen.findByText(/edits stay in your pantry/)).toBeInTheDocument();
    expect(screen.queryByRole('checkbox', { name: 'Contribute this product' })).not.toBeInTheDocument();
  });
});
