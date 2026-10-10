import type { ComponentProps } from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import type { ProductGroup } from '../../types';
import { GroupMembershipAction } from './GroupMembershipAction';

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });

const group = (extra: Partial<ProductGroup> = {}): ProductGroup => ({
  id: 'g1',
  name: 'Peanut butter',
  rule: 'same_as_ran_out',
  ruleConfirmed: false,
  members: [],
  runningLow: false,
  ...extra,
});

const renderAction = (props: Partial<ComponentProps<typeof GroupMembershipAction>> = {}) => {
  const onChanged = vi.fn();
  render(
    <MantineProvider>
      <MemoryRouter>
        <GroupMembershipAction
          productId="oat"
          productName="Oat milk"
          onChanged={onChanged}
          {...props}
        />
      </MemoryRouter>
    </MantineProvider>,
  );
  return onChanged;
};

describe('GroupMembershipAction', () => {
  it('adds an ungrouped product to the chosen group', async () => {
    const posts: unknown[] = [];
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? 'GET';
      if (url === '/api/groups' && method === 'GET') {
        return Promise.resolve(json([
          group(),
          group({ id: 'soup', name: 'Soup' }),
        ]));
      }
      posts.push(JSON.parse(String(init?.body)));
      return Promise.resolve(json(group({ id: 'soup', name: 'Soup' })));
    }));

    const onChanged = renderAction();
    fireEvent.click(screen.getByRole('button', { name: 'Add Oat milk to a group' }));
    expect(await screen.findByRole('radio', { name: 'Peanut butter' })).toBeChecked();
    fireEvent.click(screen.getByRole('radio', { name: 'Soup' }));
    fireEvent.click(screen.getByRole('button', { name: 'Add to group' }));

    await waitFor(() => expect(posts).toEqual([{ productIds: ['oat'] }]));
    expect(onChanged).toHaveBeenCalledOnce();
    expect(screen.queryByRole('dialog', { name: 'Add to group' })).not.toBeInTheDocument();
  });

  it('asks which supply setting the group should keep', async () => {
    const posts: unknown[] = [];
    const conflict = {
      error: 'Choose what this group should keep on hand.',
      code: 'target_decision_required',
      members: [{ productId: 'oat', name: 'Oat milk', quantity: 12, dimension: 'mass' as const }],
    };
    vi.stubGlobal('fetch', vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
      const method = init?.method ?? 'GET';
      if (method === 'GET') return Promise.resolve(json([group({ windowMonths: 6 })]));
      const body = JSON.parse(String(init?.body)) as { target?: unknown };
      posts.push(body);
      if (!body.target) return Promise.resolve(json(conflict, 409));
      return Promise.resolve(json(group({ windowMonths: 6 })));
    }));

    renderAction();
    fireEvent.click(screen.getByRole('button', { name: 'Add Oat milk to a group' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Add to group' }));
    expect(await screen.findByRole('button', { name: "Keep the group's 6 months" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Use 12 ounces (from Oat milk)' }));
    await waitFor(() => expect(posts).toHaveLength(2));
    expect(posts[1]).toEqual({ productIds: ['oat'], target: { quantity: 12, dimension: 'mass' } });
  });

  it('moves a product that is already in a group', async () => {
    const posts: unknown[] = [];
    vi.stubGlobal('fetch', vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
      const method = init?.method ?? 'GET';
      if (method === 'GET') {
        return Promise.resolve(json([
          group({ id: 'pb', name: 'Peanut butter' }),
          group({ id: 'soup', name: 'Soup' }),
        ]));
      }
      posts.push(JSON.parse(String(init?.body)));
      return Promise.resolve(json(group({ id: 'soup', name: 'Soup' })));
    }));

    const onChanged = renderAction({
      productId: 'peanut',
      productName: 'Creamy peanut butter',
      currentGroup: { id: 'pb', name: 'Peanut butter' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Move Creamy peanut butter to another group' }));
    expect(await screen.findByText('Already in Peanut butter.')).toBeInTheDocument();
    expect(screen.queryByRole('radio', { name: 'Peanut butter' })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('radio', { name: 'Soup' }));
    fireEvent.click(screen.getByRole('button', { name: 'Move to this group' }));
    await waitFor(() => expect(posts).toEqual([{ productIds: ['peanut'], fromGroupId: 'pb' }]));
    expect(onChanged).toHaveBeenCalledOnce();
  });

  it('says when there is nowhere to move and when no groups exist', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(json([]))));
    const { unmount } = render(
      <MantineProvider>
        <MemoryRouter>
          <GroupMembershipAction
            productId="oat"
            productName="Oat milk"
            currentGroup={{ id: 'pb', name: 'Peanut butter' }}
            onChanged={() => {}}
          />
        </MemoryRouter>
      </MantineProvider>,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Move Oat milk to another group' }));
    expect(await screen.findByText('Already in Peanut butter.')).toBeInTheDocument();
    expect(screen.getByText('No other groups.')).toBeInTheDocument();
    unmount();

    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(json([]))));
    renderAction();
    fireEvent.click(screen.getByRole('button', { name: 'Add Oat milk to a group' }));
    expect(await screen.findByText('No groups yet.')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Product groups' })).toHaveAttribute('href', '/groups');
  });

  it('filters a long group list', async () => {
    const rows = Array.from({ length: 9 }, (_, index) => group({
      id: `g${index}`,
      name: index === 8 ? 'Zucchini' : `Pantry ${index}`,
    }));
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(json(rows))));
    renderAction();
    fireEvent.click(screen.getByRole('button', { name: 'Add Oat milk to a group' }));
    const search = await screen.findByLabelText('Search groups');
    fireEvent.change(search, { target: { value: 'zuc' } });
    expect(screen.getByRole('radio', { name: 'Zucchini' })).toBeInTheDocument();
    expect(screen.queryByRole('radio', { name: 'Pantry 0' })).not.toBeInTheDocument();
    fireEvent.change(search, { target: { value: 'missing' } });
    expect(screen.getByText('No matching groups.')).toBeInTheDocument();
  });
});
