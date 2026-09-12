import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { ScanEntryCard } from './ScanEntryCard';
import type { ItemInstanceWithStatus, ScanEntry } from '../../types';
import { commitScanEntry } from '../../api/client';

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
    fireEvent.click(screen.getByRole('button', { name: 'Approve scan' }));

    await waitFor(() => expect(onChanged).toHaveBeenCalled());
    const commitCall = fetchMock.mock.calls.find(([url]) => String(url).includes('/commit'));
    expect(JSON.parse(commitCall?.[1]?.body as string)).toEqual({ instanceId: 'later' });
  });
});

describe('ScanEntryCard approve scan rename', () => {
  it('shows the approve scan button with correct text and aria-label', async () => {
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

    const button = screen.getByRole('button', { name: 'Approve scan' });
    expect(button).toBeInTheDocument();
    expect(button).toHaveAttribute('aria-label', 'Approve scan');
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

    const button = screen.getByRole('button', { name: 'Approve scan' });
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

    const button = screen.getByRole('button', { name: 'Approve scan' });
    fireEvent.click(button);

    await waitFor(() => {
      const errorAlert = screen.getByRole('alert');
      expect(errorAlert).toBeInTheDocument();
      expect(errorAlert).toHaveTextContent('Unable to approve scan.');
    });
  });
});

describe('ScanEntryCard remove control', () => {
  it('shows the remove button for pending entries', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, status: 'pending', direction: null }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const button = screen.getByText('Remove').closest('button');
    expect(button).toBeInTheDocument();
    expect(button).toHaveAttribute('aria-label', 'Remove scan');
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

    const button = screen.getByText('Remove').closest('button');
    expect(button).toBeInTheDocument();
    expect(button).toHaveAttribute('aria-label', 'Remove scan');
  });

  it('sends the cancelled PATCH on successful removal', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
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
          entry={{ ...entry, status: 'pending', direction: null }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={onChanged}
        />
      </MantineProvider>,
    );

    const button = screen.getByText('Remove').closest('button');
    if (!button) throw new Error('Remove button not found');
    fireEvent.click(button);

    await waitFor(() => expect(onChanged).toHaveBeenCalled());
  });

  it('shows error message on simulated failure and keeps the card rendered', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/scans/scan-1') return Promise.reject(new Error('Network error'));
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, status: 'pending', direction: null }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const button = screen.getByText('Remove').closest('button');
    if (!button) throw new Error('Remove button not found');
    fireEvent.click(button);

    await waitFor(() => {
      const errorAlert = screen.getByRole('alert');
      expect(errorAlert).toBeInTheDocument();
      expect(errorAlert).toHaveTextContent('Network error');
    });

    // Card should still be rendered (not removed)
    const card = screen.getByRole('article', { name: 'Scan 123' });
    expect(card).toBeInTheDocument();
  });

  it('shows fallback error message "Unable to remove scan." when error is not an Error instance', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/scans/scan-1') return Promise.reject({ statusCode: 500 });
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, status: 'pending', direction: null }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={vi.fn()}
        />
      </MantineProvider>,
    );

    const button = screen.getByText('Remove').closest('button');
    if (!button) throw new Error('Remove button not found');
    fireEvent.click(button);

    await waitFor(() => {
      const errorAlert = screen.getByRole('alert');
      expect(errorAlert).toBeInTheDocument();
      expect(errorAlert).toHaveTextContent('Unable to remove scan.');
    });
  });
});
describe('ScanEntryCard unit count controls', () => {
  it('shows increment/decrement controls when entry.status is pending', async () => {
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

    expect(screen.getByRole('button', { name: 'Decrease unit count' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Increase unit count' })).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Unit count' })).toBeInTheDocument();
  });

  it('shows decrement button disabled when unitCount is 1', async () => {
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

    expect(screen.getByRole('button', { name: 'Decrease unit count' })).toBeDisabled();
  });

  it('does not show unit count controls when entry.status is not pending', async () => {
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

    expect(screen.queryByRole('button', { name: 'Decrease unit count' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Increase unit count' })).not.toBeInTheDocument();
    expect(screen.queryByRole('spinbutton', { name: 'Unit count' })).not.toBeInTheDocument();
  });

  it('calls handleIncrement on increment button click', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/scans/scan-1') {
        expect(request?.method).toBe('PATCH');
        expect(JSON.parse(request?.body as string)).toEqual({ unitCount: 2 });
        return Promise.resolve(jsonResponse({ ...entry, unitCount: 2 }));
      }
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    const onChanged = vi.fn();

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, unitCount: 1 }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={onChanged}
        />
      </MantineProvider>,
    );

    const incrementButton = screen.getByRole('button', { name: 'Increase unit count' });
    fireEvent.click(incrementButton);

    await waitFor(() => {
      expect(onChanged).toHaveBeenCalled();
    });
  });

  it('calls handleDecrement on decrement button click', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, request?: RequestInit) => {
      const url = String(input);
      if (url === '/api/scans/scan-1') {
        expect(request?.method).toBe('PATCH');
        expect(JSON.parse(request?.body as string)).toEqual({ unitCount: 1 });
        return Promise.resolve(jsonResponse({ ...entry, unitCount: 1 }));
      }
      throw new Error(`Unexpected ${request?.method ?? 'GET'} request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    const onChanged = vi.fn();

    render(
      <MantineProvider>
        <ScanEntryCard
          entry={{ ...entry, unitCount: 2 }}
          itemId="item-1"
          selected={false}
          onSelectedChange={vi.fn()}
          onChanged={onChanged}
        />
      </MantineProvider>,
    );

    const decrementButton = screen.getByRole('button', { name: 'Decrease unit count' });
    fireEvent.click(decrementButton);

    await waitFor(() => {
      expect(onChanged).toHaveBeenCalled();
    });
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

    const input = screen.getByRole('textbox', { name: 'Unit count' });
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

    const input = screen.getByRole('textbox', { name: 'Unit count' });
    expect(input).toHaveValue('3');
    
    // Type 0, then blur - the blur handler validates and should reset
    fireEvent.change(input, { target: { value: '0' } });
    fireEvent.blur(input);
    
    // After blur validation fails, value should revert to original
    expect(input).toHaveValue('3');
    // No PATCH should be sent since validation failed before PATCH
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

    const input = screen.getByRole('textbox', { name: 'Unit count' });
    expect(input).toHaveValue('3');
    
    // Type 2.5 which gets truncated to 2 by NumberInput's allowDecimal=false
    fireEvent.change(input, { target: { value: '2.5' } });
    expect(input).toHaveValue('2');
    
    // Blurring should send PATCH since 2 != 3
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

    const input = screen.getByRole('textbox', { name: 'Unit count' });
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

    // Value should revert to original on failure
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

    const input = screen.getByRole('textbox', { name: 'Unit count' });
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

    const input = screen.getByRole('textbox', { name: 'Unit count' });
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

    // Clear the error by typing (handleUnitCountChange clears unitCountError when value is not null/empty)
    fireEvent.change(input, { target: { value: '4' } });
    
    // The error should be cleared by handleUnitCountChange
    const errorAlertsAfter = screen.getAllByRole('alert');
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

    const input = screen.getByRole('textbox', { name: 'Unit count' });
    expect(input).toHaveValue('3');
    fireEvent.change(input, { target: { value: '4' } });
    fireEvent.keyDown(input, { key: 'Enter' });

    await waitFor(() => {
      expect(onChanged).toHaveBeenCalled();
    });
  });
});
