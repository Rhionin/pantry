import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { ScanEntryCard } from './ScanEntryCard';
import type { ItemInstanceWithStatus, ScanEntry } from '../../types';

const entry: ScanEntry = {
  id: 'scan-1',
  userId: 'default-user',
  barcode: '123',
  scannedAt: '2026-03-20T10:00:00Z',
  direction: 'stock_out',
  unitCount: 1,
  expiresAt: null,
  status: 'pending',
  productId: 'product-1',
  product: { id: 'product-1', name: 'Milk', category: 'Dairy', unitOfMeasure: 'carton' },
  committedAt: null,
  createdAt: '2026-03-20T10:00:00Z',
};

const instances: ItemInstanceWithStatus[] = [
  { id: 'later', itemId: 'item-1', stockInAt: '2026-03-02T00:00:00Z', expiresAt: '2026-05-01T00:00:00Z', removedAt: null, removalReason: null, createdAt: '2026-03-02T00:00:00Z', expiryStatus: 'ok' },
  { id: 'oldest', itemId: 'item-1', stockInAt: '2026-03-01T00:00:00Z', expiresAt: '2026-04-01T00:00:00Z', removedAt: null, removalReason: null, createdAt: '2026-03-01T00:00:00Z', expiryStatus: 'ok' },
  { id: 'undated', itemId: 'item-1', stockInAt: '2026-03-03T00:00:00Z', expiresAt: null, removedAt: null, removalReason: null, createdAt: '2026-03-03T00:00:00Z', expiryStatus: 'ok' },
];

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });

describe('ScanEntryCard stock-out review', () => {
  it('orders instances oldest first and commits the specifically selected instance', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/inventory/item-1/instances') return Promise.resolve(jsonResponse(instances));
      if (url === '/api/scans/scan-1/commit') return Promise.resolve(jsonResponse({ ...entry, status: 'committed' }));
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    const onChanged = vi.fn();
    render(
      <MantineProvider>
        <ScanEntryCard
          entry={entry}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={onChanged}
        />
      </MantineProvider>,
    );

    const options = await screen.findAllByRole('radio');
    expect(options.map((option) => option.getAttribute('value'))).toEqual(['', 'oldest', 'later', 'undated']);
    fireEvent.click(options[2]);
    fireEvent.click(screen.getByRole('button', { name: 'Approve' }));

    await waitFor(() => expect(onChanged).toHaveBeenCalled());
    const commitCall = fetchMock.mock.calls.find(([url]) => String(url).includes('/commit'));
    expect(JSON.parse(commitCall?.[1]?.body as string)).toEqual({ instanceId: 'later' });
  });
});

describe('ScanEntryCard approve scan rename', () => {
  it('shows the approve button with correct text and aria-label', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/inventory/item-1/instances') return Promise.resolve(jsonResponse(instances));
      if (url === '/api/scans/scan-1/commit') return Promise.resolve(jsonResponse({ ...entry, status: 'committed' }));
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, direction: 'stock_out' }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const button = screen.getByRole('button', { name: 'Approve' });
    expect(button).toBeInTheDocument();
  });

  it('shows error message on simulated failure', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/inventory/item-1/instances') return Promise.resolve(jsonResponse(instances));
      if (url === '/api/scans/scan-1/commit') return Promise.reject(new Error('Network error'));
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, direction: 'stock_out' }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const button = screen.getByRole('button', { name: 'Approve' });
    fireEvent.click(button);

    await waitFor(() => {
      const errorAlert = screen.getByRole('alert');
      expect(errorAlert).toBeInTheDocument();
      expect(errorAlert).toHaveTextContent('Network error');
    });
  });

  it('shows fallback error message "Unable to approve scan." when error is not an Error instance', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/inventory/item-1/instances') return Promise.resolve(jsonResponse(instances));
      if (url === '/api/scans/scan-1/commit') return Promise.reject({ statusCode: 500 });
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, direction: 'stock_out' }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const button = screen.getByRole('button', { name: 'Approve' });
    fireEvent.click(button);

    await waitFor(() => {
      const errorAlert = screen.getByRole('alert');
      expect(errorAlert).toBeInTheDocument();
      expect(errorAlert).toHaveTextContent('Unable to approve scan.');
    });
  });
});

describe('ScanEntryCard remove control', () => {
  it('shows the remove button for pending entries with direction', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, status: 'pending', direction: 'stock_out' }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const button = screen.getByRole('button', { name: 'Remove' });
    expect(button).toBeInTheDocument();
  });

  it('shows the remove button for flagged entries', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, status: 'flagged', direction: null }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const button = screen.getByRole('button', { name: 'Remove' });
    expect(button).toBeInTheDocument();
  });

  it('sends the cancelled PATCH on successful removal', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/inventory/item-1/instances') return Promise.resolve(jsonResponse(instances));
      if (url === '/api/scans/scan-1') {
        expect(request?.method).toBe('PATCH');
        expect(JSON.parse(request?.body as string)).toEqual({ status: 'cancelled' });
        return Promise.resolve(jsonResponse({ ...entry, status: 'cancelled' }));
      }
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    const onChanged = vi.fn();

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, status: 'pending', direction: 'stock_out' }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={onChanged}
        />
      </MantineProvider>,
    );

    const button = screen.getByRole('button', { name: 'Remove' });
    fireEvent.click(button);

    await waitFor(() => expect(onChanged).toHaveBeenCalled());
  });

  it('shows error message on simulated failure and keeps the card rendered', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/inventory/item-1/instances') return Promise.resolve(jsonResponse(instances));
      if (url === '/api/scans/scan-1') return Promise.reject(new Error('Network error'));
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, status: 'pending', direction: 'stock_out' }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const button = screen.getByRole('button', { name: 'Remove' });
    fireEvent.click(button);

    await waitFor(() => {
      const errorAlert = screen.getByRole('alert');
      expect(errorAlert).toBeInTheDocument();
      expect(errorAlert).toHaveTextContent('Network error');
    });

    const card = screen.getByRole('article', { name: 'Scan 123' });
    expect(card).toBeInTheDocument();
  });

  it('shows fallback error message "Unable to remove scan." when error is not an Error instance', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/inventory/item-1/instances') return Promise.resolve(jsonResponse(instances));
      if (url === '/api/scans/scan-1') return Promise.reject({ statusCode: 500 });
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, status: 'pending', direction: 'stock_out' }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const button = screen.getByRole('button', { name: 'Remove' });
    fireEvent.click(button);

    await waitFor(() => {
      const errorAlert = screen.getByRole('alert');
      expect(errorAlert).toBeInTheDocument();
      expect(errorAlert).toHaveTextContent('Unable to remove scan.');
    });
  });
});

describe('ScanEntryCard unit count controls', () => {
  it('shows unit count input when entry.status is pending', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, unitCount: 3 }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    expect(screen.getByLabelText('Unit count')).toBeInTheDocument();
  });

  it('does not show unit count input when entry.status is not pending', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, status: 'flagged' }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    expect(screen.queryByLabelText('Unit count')).not.toBeInTheDocument();
  });
});

describe('ScanEntryCard direct numeric input', () => {
  it('sends expected PATCH for a valid confirmed value', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/scans/scan-1') {
        expect(request?.method).toBe('PATCH');
        expect(JSON.parse(request?.body as string)).toEqual({ unitCount: 5 });
        return Promise.resolve(jsonResponse({ ...entry, unitCount: 5 }));
      }
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    const onChanged = vi.fn();

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, unitCount: 3 }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={onChanged}
        />
      </MantineProvider>,
    );

    const input = screen.getByLabelText('Unit count') as HTMLInputElement;
    expect(input).toHaveValue('3');
    fireEvent.change(input, { target: { value: '5' } });
    fireEvent.blur(input);

    await waitFor(() => {
      expect(onChanged).toHaveBeenCalled();
    });
  });

  it('rejects invalid value (< 1) client-side with displayed count unchanged', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, unitCount: 3 }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const input = screen.getByLabelText('Unit count') as HTMLInputElement;
    expect(input).toHaveValue('3');
    
    fireEvent.change(input, { target: { value: '0' } });
    fireEvent.blur(input);
    
    expect(input).toHaveValue('3');
    const patchCalls = fetchMock.mock.calls.filter(([url]) => url.includes('/scans/scan-1'));
    expect(patchCalls.length).toBe(0);
  });

  it('rejects invalid value (non-integer) client-side with displayed count unchanged', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, unitCount: 3 }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const input = screen.getByLabelText('Unit count') as HTMLInputElement;
    expect(input).toHaveValue('3');
    
    fireEvent.change(input, { target: { value: '2.5' } });
    expect(input).toHaveValue('2');
    
    fireEvent.blur(input);
    
    const patchCalls = fetchMock.mock.calls.filter(([url]) => url.includes('/scans/scan-1'));
    expect(patchCalls.length).toBe(1);
    expect(JSON.parse(patchCalls[0][1]?.body as string)).toEqual({ unitCount: 2 });
  });

  it('shows error and reverts to previously confirmed count on failed PATCH', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/scans/scan-1') {
        return Promise.reject(new Error('Network error'));
      }
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, unitCount: 3 }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const input = screen.getByLabelText('Unit count') as HTMLInputElement;
    expect(input).toHaveValue('3');
    fireEvent.change(input, { target: { value: '5' } });
    fireEvent.blur(input);

    await waitFor(() => {
      const errorAlerts = screen.getAllByRole('alert');
      const unitCountError = errorAlerts.find(alert => 
        alert.textContent?.includes('Unable to update unit count.')
      );
      expect(unitCountError).toBeInTheDocument();
    });

    expect(input).toHaveValue('3');
  });

  it('shows fallback error message when error is not an Error instance', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/scans/scan-1') {
        return Promise.reject({ statusCode: 500 });
      }
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, unitCount: 3 }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const input = screen.getByLabelText('Unit count') as HTMLInputElement;
    expect(input).toHaveValue('3');
    fireEvent.change(input, { target: { value: '5' } });
    fireEvent.blur(input);

    await waitFor(() => {
      const errorAlerts = screen.getAllByRole('alert');
      const unitCountError = errorAlerts.find(alert => 
        alert.textContent?.includes('Unable to update unit count.')
      );
      expect(unitCountError).toBeInTheDocument();
    });
  });

  it('clears error when user starts typing after error', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/scans/scan-1') {
        return Promise.reject(new Error('Network error'));
      }
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, unitCount: 3 }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const input = screen.getByLabelText('Unit count') as HTMLInputElement;
    expect(input).toHaveValue('3');
    fireEvent.change(input, { target: { value: '5' } });
    fireEvent.blur(input);

    await waitFor(() => {
      const errorAlerts = screen.getAllByRole('alert');
      const unitCountError = errorAlerts.find(alert => 
        alert.textContent?.includes('Unable to update unit count.')
      );
      expect(unitCountError).toBeInTheDocument();
    });

    fireEvent.change(input, { target: { value: '4' } });
    
    const errorAlertsAfter = screen.queryAllByRole('alert');
    const unitCountErrorAfter = errorAlertsAfter.find(alert => 
      alert.textContent?.includes('Unable to update unit count.')
    );
    expect(unitCountErrorAfter).toBeUndefined();
  });

  it('sends PATCH on Enter key press', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/scans/scan-1') {
        expect(request?.method).toBe('PATCH');
        expect(JSON.parse(request?.body as string)).toEqual({ unitCount: 4 });
        return Promise.resolve(jsonResponse({ ...entry, unitCount: 4 }));
      }
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    const onChanged = vi.fn();

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, unitCount: 3 }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={onChanged}
        />
      </MantineProvider>,
    );

    const input = screen.getByLabelText('Unit count');
    expect(input).toHaveValue('3');
    fireEvent.change(input, { target: { value: '4' } });
    fireEvent.keyDown(input, { key: 'Enter' });

    await waitFor(() => {
      expect(onChanged).toHaveBeenCalled();
    });
  });
});

describe('ScanEntryCard change indicator', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('shows change indicator on mount with animated class by default', () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={entry}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const card = screen.getByRole('article', { name: 'Scan 123' });
    expect(card).toHaveClass('scan-entry-card--changed-animated');
  });

  it('hides change indicator after 600ms', () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={entry}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const card = screen.getByRole('article', { name: 'Scan 123' });
    expect(card).toHaveClass('scan-entry-card--changed-animated');

    act(() => {
      vi.advanceTimersByTime(600);
    });

    expect(card).not.toHaveClass('scan-entry-card--changed-animated');
    expect(card).not.toHaveClass('scan-entry-card--changed-static');
  });

  it('shows change indicator when unitCount changes with animated class', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    const { rerender } = render(
      <MantineProvider>
        <ScanEntryCard
          entry={entry}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    act(() => {
      vi.advanceTimersByTime(600);
    });

    const card = screen.getByRole('article', { name: 'Scan 123' });
    expect(card).not.toHaveClass('scan-entry-card--changed-animated');

    rerender(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, unitCount: 5 }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    expect(card).toHaveClass('scan-entry-card--changed-animated');
  });

  it('shows change indicator with static class when prefers-reduced-motion is set', () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({
        matches: true,
        media: '(prefers-reduced-motion: reduce)',
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    );

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={entry}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const card = screen.getByRole('article', { name: 'Scan 123' });
    expect(card).toHaveClass('scan-entry-card--changed-static');
    expect(card).not.toHaveClass('scan-entry-card--changed-animated');
  });

  it('hides static change indicator after 600ms', () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({
        matches: true,
        media: '(prefers-reduced-motion: reduce)',
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    );

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={entry}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const card = screen.getByRole('article', { name: 'Scan 123' });
    expect(card).toHaveClass('scan-entry-card--changed-static');

    act(() => {
      vi.advanceTimersByTime(600);
    });

    expect(card).not.toHaveClass('scan-entry-card--changed-static');
    expect(card).not.toHaveClass('scan-entry-card--changed-animated');
  });
});

describe('ScanEntryCard batch selection checkbox', () => {
  it('renders the checkbox for pending entries', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={entry}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const checkbox = screen.getByRole('checkbox', { name: 'Select scan for batch approval' });
    expect(checkbox).toBeInTheDocument();
  });

  it('does not render visible label text for the checkbox', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={entry}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    screen.getByRole('checkbox', { name: 'Select scan for batch approval' });

    expect(screen.queryByText('Select scan for batch approval')).not.toBeInTheDocument();
    expect(screen.queryByText(/Select scan/)).not.toBeInTheDocument();
  });

  it('sets aria-label describing batch approval purpose', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={entry}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const checkbox = screen.getByRole('checkbox', { name: 'Select scan for batch approval' });
    expect(checkbox).toHaveAttribute('aria-label', 'Select scan for batch approval');
    expect(checkbox.getAttribute('aria-label')).toContain('batch');
  });

  it('toggles checkbox from unchecked to checked and calls onSelectedChange', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    const onSelectedChange = vi.fn();

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={entry}
          itemId="item-1"
          selected={false}
          onSelectedChange={onSelectedChange}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const checkbox = screen.getByRole('checkbox', { name: 'Select scan for batch approval' });
    expect(checkbox).not.toBeChecked();

    fireEvent.click(checkbox);

    expect(onSelectedChange).toHaveBeenCalledWith(true);
  });

  it('toggles checkbox from checked to unchecked and calls onSelectedChange', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    const onSelectedChange = vi.fn();

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={entry}
          itemId="item-1"
          selected={true}
          onSelectedChange={onSelectedChange}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const checkbox = screen.getByRole('checkbox', { name: 'Select scan for batch approval' });
    expect(checkbox).toBeChecked();

    fireEvent.click(checkbox);

    expect(onSelectedChange).toHaveBeenCalledWith(false);
  });

  it('does not render checkbox for flagged entries', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, status: 'flagged' }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    expect(screen.queryByRole('checkbox', { name: 'Select scan for batch approval' })).not.toBeInTheDocument();
  });

  it('reflects the selected prop state in the checkbox', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    const { rerender } = render(
      <MantineProvider>
        <ScanEntryCard
          entry={entry}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const checkbox = screen.getByRole('checkbox', { name: 'Select scan for batch approval' });
    expect(checkbox).not.toBeChecked();

    rerender(
      <MantineProvider>
        <ScanEntryCard
          entry={entry}
          itemId="item-1"
          selected={true}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    expect(checkbox).toBeChecked();
  });
});

describe('ScanEntryCard expiration date input', () => {
  it('shows date input when entry.status is pending', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={entry}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    expect(screen.getByLabelText('Expiration date')).toBeInTheDocument();
  });

  it('defaults to empty string when entry.expiresAt is null', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, expiresAt: null }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const input = screen.getByLabelText('Expiration date') as HTMLInputElement;
    expect(input).toHaveValue('');
  });

  it('defaults to YYYY-MM-DD format when entry.expiresAt is set', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, expiresAt: '2026-05-15T10:00:00Z' }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const input = screen.getByLabelText('Expiration date') as HTMLInputElement;
    expect(input).toHaveValue('2026-05-15');
  });

  it('sends expected PATCH for a confirmed date value', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/scans/scan-1') {
        expect(request?.method).toBe('PATCH');
        const body = JSON.parse(request?.body as string);
        expect(body.expiresAt).toBe('2026-06-01T00:00:00.000Z');
        return Promise.resolve(jsonResponse({ ...entry, expiresAt: '2026-06-01T00:00:00.000Z' }));
      }
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    const onChanged = vi.fn();

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, expiresAt: null }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={onChanged}
        />
      </MantineProvider>,
    );

    const input = screen.getByLabelText('Expiration date') as HTMLInputElement;
    expect(input).toHaveValue('');
    fireEvent.change(input, { target: { value: '2026-06-01' } });
    fireEvent.blur(input);

    await waitFor(() => {
      expect(onChanged).toHaveBeenCalled();
    });
  });

  it('does not send PATCH when value has not changed', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, expiresAt: '2026-05-15T00:00:00Z' }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const input = screen.getByLabelText('Expiration date') as HTMLInputElement;
    expect(input).toHaveValue('2026-05-15');
    fireEvent.blur(input);

    const patchCalls = fetchMock.mock.calls.filter(([url]) => String(url).includes('/scans/scan-1'));
    expect(patchCalls.length).toBe(0);
  });

  it('shows error and resets to previously confirmed date on failed PATCH', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/scans/scan-1') {
        return Promise.reject(new Error('Network error'));
      }
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, expiresAt: '2026-05-15T00:00:00Z' }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const input = screen.getByLabelText('Expiration date') as HTMLInputElement;
    expect(input).toHaveValue('2026-05-15');
    fireEvent.change(input, { target: { value: '2026-06-01' } });
    fireEvent.blur(input);

    await waitFor(() => {
      const errorAlerts = screen.getAllByRole('alert');
      const expiryError = errorAlerts.find(alert =>
        alert.textContent?.includes('Unable to update expiration date.')
      );
      expect(expiryError).toBeInTheDocument();
    });

    expect(input).toHaveValue('2026-05-15');
  });

  it('shows error and resets empty to empty on failed PATCH when original expiresAt was null', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/scans/scan-1') {
        return Promise.reject(new Error('Network error'));
      }
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, expiresAt: null }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const input = screen.getByLabelText('Expiration date') as HTMLInputElement;
    expect(input).toHaveValue('');
    fireEvent.change(input, { target: { value: '2026-06-01' } });
    fireEvent.blur(input);

    await waitFor(() => {
      const errorAlerts = screen.getAllByRole('alert');
      const expiryError = errorAlerts.find(alert =>
        alert.textContent?.includes('Unable to update expiration date.')
      );
      expect(expiryError).toBeInTheDocument();
    });

    expect(input).toHaveValue('');
  });

  it('clears error when user starts typing after error', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/scans/scan-1') {
        return Promise.reject(new Error('Network error'));
      }
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, expiresAt: '2026-05-15T00:00:00Z' }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const input = screen.getByLabelText('Expiration date') as HTMLInputElement;
    fireEvent.change(input, { target: { value: '2026-06-01' } });
    fireEvent.blur(input);

    await waitFor(() => {
      const errorAlerts = screen.getAllByRole('alert');
      const expiryError = errorAlerts.find(alert =>
        alert.textContent?.includes('Unable to update expiration date.')
      );
      expect(expiryError).toBeInTheDocument();
    });

    fireEvent.change(input, { target: { value: '2026-06-02' } });

    await waitFor(() => {
      expect(screen.queryByText('Unable to update expiration date.')).not.toBeInTheDocument();
    });
  });

  it('does not show date input when entry.status is not pending', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, status: 'flagged' }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    expect(screen.queryByLabelText('Expiration date')).not.toBeInTheDocument();
  });

  it('shows fallback error message when error is not an Error instance', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/scans/scan-1') {
        return Promise.reject({ statusCode: 500 });
      }
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, expiresAt: '2026-05-15T00:00:00Z' }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const input = screen.getByLabelText('Expiration date') as HTMLInputElement;
    fireEvent.change(input, { target: { value: '2026-06-01' } });
    fireEvent.blur(input);

    await waitFor(() => {
      const errorAlerts = screen.getAllByRole('alert');
      const expiryError = errorAlerts.find(alert =>
        alert.textContent?.includes('Unable to update expiration date.')
      );
      expect(expiryError).toBeInTheDocument();
    });
  });
});

describe('ScanEntryCard provenance badge', () => {
  it('displays provenance badge when product has externalSource', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, product: { id: 'product-1', name: 'Milk', category: 'Dairy', unitOfMeasure: 'carton', externalSource: 'openfoodfacts' } }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const badge = screen.getByLabelText('Product data from Open Food Facts');
    expect(badge).toBeInTheDocument();
    expect(badge).toHaveTextContent('Open Food Facts');
  });

  it('does not display badge when product has no externalSource', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={entry}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const badge = screen.queryByLabelText(/Product data from/);
    expect(badge).not.toBeInTheDocument();
  });

  it('does not display badge when product is null', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, product: null }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const badge = screen.queryByLabelText(/Product data from/);
    expect(badge).not.toBeInTheDocument();
  });
});
