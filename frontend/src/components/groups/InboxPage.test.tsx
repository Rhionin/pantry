import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import type { GroupSuggestion } from '../../types';
import { InboxPage } from './InboxPage';

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });

const card: GroupSuggestion = {
  id: 'sug',
  kind: 'looks_alike',
  title: 'Peanut butter',
  status: 'open',
  members: [
    { productId: 'plain', name: 'Great Value Peanut Butter', included: true, caution: '' },
    { productId: 'crunchy', name: 'Great Value Crunchy Peanut Butter', included: false, caution: 'Crunchy. Probably not the same.' },
  ],
};

const renderInbox = (handler: (url: string, init?: RequestInit) => Response | Promise<Response>) => {
  vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, init?: RequestInit) => Promise.resolve(handler(String(input), init))));
  return render(
    <MantineProvider>
      <MemoryRouter>
        <InboxPage />
      </MemoryRouter>
    </MantineProvider>,
  );
};

describe('InboxPage', () => {
  it('leaves the caution row unchecked, groups the checked products, and dismisses the card', async () => {
    const calls: { url: string; body?: string }[] = [];
    let open = [card];
    renderInbox((url, init) => {
      calls.push({ url, body: init?.body as string | undefined });
      if (url === '/api/group-suggestions' && (init?.method ?? 'GET') === 'GET') return json(open);
      if (url.endsWith('/accept')) {
        open = [];
        return json({ id: 'g', name: 'Peanut butter', rule: 'same_as_ran_out', ruleConfirmed: false, members: [], runningLow: false });
      }
      if (url.endsWith('/dismiss')) {
        open = [];
        return new Response(null, { status: 204 });
      }
      throw new Error(`Unexpected ${init?.method} ${url}`);
    });

    expect(await screen.findByLabelText('Great Value Crunchy Peanut Butter')).not.toBeChecked();
    expect(screen.getByText('Crunchy. Probably not the same.')).toBeInTheDocument();
    expect(screen.getByLabelText('Great Value Peanut Butter')).toBeChecked();

    fireEvent.click(screen.getByRole('button', { name: 'Group' }));
    await waitFor(() => expect(calls.some((call) => call.url.endsWith('/accept'))).toBe(true));
    const accept = calls.find((call) => call.url.endsWith('/accept'));
    expect(JSON.parse(accept?.body ?? '{}')).toEqual({ productIds: ['plain'] });
    expect(await screen.findByText('Nothing to review.')).toBeInTheDocument();
  });

  it('dismisses the card and waits when members already have a target', async () => {
    const calls: string[] = [];
    renderInbox((url, init) => {
      calls.push(`${init?.method ?? 'GET'} ${url}`);
      if (url === '/api/group-suggestions') return json([card]);
      if (url.endsWith('/accept')) {
        return json({
          error: 'Choose what this group should keep on hand. These products already have their own supply setting.',
          code: 'target_decision_required',
          members: [{ productId: 'plain', name: 'Great Value Peanut Butter', quantity: 4 }],
        }, 409);
      }
      if (url.endsWith('/dismiss')) return new Response(null, { status: 204 });
      throw new Error(`Unexpected ${url}`);
    });

    fireEvent.click(await screen.findByRole('button', { name: 'Group' }));
    expect(await screen.findByText('Great Value Peanut Butter: keep 4')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Group' })).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Not the same' }));
    await waitFor(() => expect(calls.some((call) => call.startsWith('POST') && call.endsWith('/dismiss'))).toBe(true));
  });
});
