import { render, screen } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { afterEach, describe, expect, it, vi } from 'vitest';
import App from './App';

class FakeEventSource {
  addEventListener() {}
  close() {}
}

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });

describe('section tabs', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    window.history.pushState({}, '', '/');
  });

  it('opens History from the same tabs as Scan Queue, Inventory, and Shopping List', async () => {
    vi.stubGlobal('EventSource', FakeEventSource);
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/api/session')) return Promise.resolve(json({ required: false }));
      if (url.includes('/api/history')) return Promise.resolve(json({ moves: [] }));
      if (url.includes('/api/providers')) return Promise.resolve(json([]));
      if (url.includes('/api/group-suggestions')) return Promise.resolve(json([]));
      if (url.includes('/api/build')) return Promise.resolve(json({}));
      return Promise.resolve(json({}));
    }));
    window.history.pushState({}, '', '/history');

    render(
      <MantineProvider env="test">
        <App />
      </MantineProvider>,
    );

    const nav = await screen.findByRole('navigation', { name: 'Sections' });
    expect(nav).toHaveTextContent('Scan Queue');
    expect(nav).toHaveTextContent('Inventory');
    expect(nav).toHaveTextContent('Shopping List');
    expect(screen.getByRole('link', { name: 'History' })).toHaveAttribute('aria-current', 'page');
    expect(await screen.findByRole('heading', { name: 'History' })).toBeInTheDocument();
    expect(await screen.findByText(/No stock moves yet/)).toBeInTheDocument();
  });
});
