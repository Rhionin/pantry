import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { describe, expect, it, vi } from 'vitest';
import { SuggestionPanel } from './SuggestionPanel';

const jsonResponse = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });

const renderPanel = (onTargetQuantitySaved = vi.fn()) => {
  render(
    <MantineProvider>
      <SuggestionPanel
        itemId="item-1"
        productName="Whole Milk"
        onTargetQuantitySaved={onTargetQuantitySaved}
      />
    </MantineProvider>,
  );
  return onTargetQuantitySaved;
};

describe('SuggestionPanel', () => {
  it('loads a suggestion only when requested and accepts it', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === '/api/suggestions/item-1') {
        return Promise.resolve(jsonResponse({
          itemId: 'item-1',
          suggestedQuantity: 4,
          reasoning: 'You use one carton every four days.',
          consumptionEventCount: 5,
          dataInsufficient: false,
        }));
      }
      if (url === '/api/items/item-1/target-quantity') {
        expect(init?.method).toBe('POST');
        expect(init?.body).toBe(JSON.stringify({ targetQuantity: 4 }));
        return Promise.resolve(jsonResponse({ itemId: 'item-1', targetQuantity: 4 }));
      }
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    const onSaved = renderPanel();

    expect(fetchMock).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Get suggestion' }));

    expect(await screen.findByText('Suggested target: 4')).toBeInTheDocument();
    expect(screen.getByText('You use one carton every four days.')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Accept' }));

    expect(await screen.findByText('Target quantity set to 4.')).toBeInTheDocument();
    expect(onSaved).toHaveBeenCalledOnce();
  });

  it('prompts for and saves a manual target when data is insufficient', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === '/api/suggestions/item-1') {
        return Promise.resolve(jsonResponse({
          itemId: 'item-1',
          suggestedQuantity: 0,
          reasoning: '',
          consumptionEventCount: 2,
          dataInsufficient: true,
        }));
      }
      if (url === '/api/items/item-1/target-quantity') {
        expect(init?.body).toBe(JSON.stringify({ targetQuantity: 6 }));
        return Promise.resolve(jsonResponse({ itemId: 'item-1', targetQuantity: 6 }));
      }
      throw new Error(`Unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    renderPanel();

    fireEvent.click(screen.getByRole('button', { name: 'Get suggestion' }));
    expect(await screen.findByText('Not enough consumption history')).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText('Manual target quantity'), { target: { value: '6' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save manual target' }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      '/api/items/item-1/target-quantity',
      expect.objectContaining({ method: 'POST', body: JSON.stringify({ targetQuantity: 6 }) }),
    ));
  });
});
