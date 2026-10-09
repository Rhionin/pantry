import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import type { InventoryItem, Product, ProductGroup } from '../../types';
import { GroupDetailPage } from './GroupDetailPage';

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });

const member = (productId: string, name: string, onHand: number) => ({ productId, name, onHand });

const baseGroup = (): ProductGroup => ({
  id: 'g1',
  name: 'Gatorade powder',
  rule: 'same_as_ran_out',
  ruleConfirmed: false,
  members: [
    member('punch', 'Fruit punch thirst quencher powder', 2),
    member('thirst', 'Thirst Quencher Powder', 4),
  ],
  runningLow: false,
});

const product = (id: string, name: string): Product => ({
  id,
  name,
  category: 'Drinks',
  unitOfMeasure: 'canister',
  createdAt: '2026-01-01T00:00:00Z',
});

const inventoryRow = (id: string, name: string, count: number): InventoryItem => ({
  item: {
    id: `item-${id}`,
    userId: 'user-1',
    productId: id,
    product: { id, name, category: 'Drinks', unitOfMeasure: 'canister' },
    targetQuantity: null,
    createdAt: '2026-01-01T00:00:00Z',
  },
  instanceCount: count,
  nearExpiryCount: 0,
  expiredCount: 0,
  needsAttention: false,
});

const renderDetail = (
  handler: (url: string, init?: RequestInit) => Response | Promise<Response>,
) => {
  vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, init?: RequestInit) => Promise.resolve(handler(String(input), init))));
  return render(
    <MantineProvider>
      <MemoryRouter initialEntries={['/groups/g1']}>
        <Routes>
          <Route path="/groups/:id" element={<GroupDetailPage />} />
          <Route path="/groups" element={<p>Groups list</p>} />
        </Routes>
      </MemoryRouter>
    </MantineProvider>,
  );
};

describe('GroupDetailPage', () => {
  it('opens the rule sheet from the badge and shows the saved rule', async () => {
    let current = baseGroup();
    const calls: { url: string; method: string; body?: string }[] = [];
    renderDetail((url, init) => {
      const method = init?.method ?? 'GET';
      calls.push({ url, method, body: typeof init?.body === 'string' ? init.body : undefined });
      if (url === '/api/groups/g1' && method === 'GET') return json(current);
      if (url === '/api/groups' && method === 'GET') return json([current]);
      if (url === '/api/products' && method === 'GET') return json([product('glacier', 'Gatorade Glacier Freeze')]);
      if (url === '/api/inventory') return json([inventoryRow('glacier', 'Gatorade Glacier Freeze', 1)]);
      if (url.startsWith('/api/products/')) return json({ barcodes: ['052000340075'] });
      if (url === '/api/groups/preview') {
        return json({ productId: 'punch', because: 'Same as what ran out', buy: 1, explain: 'Buy the powder that ran out.' });
      }
      if (url === '/api/groups/g1/rule' && method === 'PUT') {
        const body = JSON.parse(String(init?.body)) as { rule: string; pinnedProductId: string };
        current = { ...current, rule: body.rule, pinnedProductId: body.pinnedProductId, ruleConfirmed: true };
        return json(current);
      }
      if (url === '/api/groups/g1/target' && method === 'PUT') return json(current);
      throw new Error(`Unexpected ${method} ${url}`);
    });

    fireEvent.click(await screen.findByRole('button', { name: 'Pick a rule' }));
    expect(await screen.findByRole('dialog', { name: 'Gatorade powder' })).toBeInTheDocument();
    expect(await screen.findByText('Next trip: Buy the powder that ran out.')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('textbox', { name: 'Rule' }));
    fireEvent.click(await screen.findByRole('option', { name: 'Always my favorite' }));
    fireEvent.click(screen.getByRole('textbox', { name: 'Always buy' }));
    fireEvent.click(await screen.findByRole('option', { name: 'Fruit punch thirst quencher powder' }));
    fireEvent.click(screen.getByRole('button', { name: 'Save rule' }));

    expect(await screen.findByRole('button', { name: 'Edit rule: Always my favorite' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Pick a rule' })).not.toBeInTheDocument();
    const saved = calls.find((call) => call.url === '/api/groups/g1/rule');
    expect(JSON.parse(saved?.body ?? '{}')).toEqual({
      rule: 'favorite',
      pinnedProductId: 'punch',
      confirm: true,
    });
  });

  it('adds an ungrouped product and asks which target to keep when supply settings differ', async () => {
    let current = baseGroup();
    let membersPosts = 0;
    renderDetail((url, init) => {
      const method = init?.method ?? 'GET';
      if (url === '/api/groups/g1' && method === 'GET') return json(current);
      if (url === '/api/groups' && method === 'GET') {
        return json([current, {
          id: 'g2',
          name: 'Milk',
          rule: 'favorite',
          ruleConfirmed: true,
          members: [member('milk', 'Whole Milk', 1)],
          runningLow: false,
        }]);
      }
      if (url === '/api/products' && method === 'GET') {
        return json([
          product('punch', 'Fruit punch thirst quencher powder'),
          product('glacier', 'Gatorade Glacier Freeze'),
          product('lemon', 'Lemonade powder'),
          product('milk', 'Whole Milk'),
        ]);
      }
      if (url === '/api/inventory') {
        return json([
          inventoryRow('punch', 'Fruit punch thirst quencher powder', 2),
          inventoryRow('glacier', 'Gatorade Glacier Freeze', 1),
          inventoryRow('milk', 'Whole Milk', 1),
        ]);
      }
      if (url.startsWith('/api/products/')) return json({ barcodes: [] });
      if (url === '/api/groups/g1/members' && method === 'POST') {
        membersPosts += 1;
        const body = JSON.parse(String(init?.body)) as { productIds: string[]; target?: { clear?: boolean } };
        if (membersPosts === 1) {
          return json({
            error: 'Choose what this group should keep on hand.',
            code: 'target_decision_required',
            members: [{ productId: 'glacier', name: 'Gatorade Glacier Freeze', quantity: 12 }],
          }, 409);
        }
        expect(body.productIds).toEqual(['glacier']);
        expect(body.target).toEqual({ clear: true });
        current = {
          ...current,
          members: [...current.members, member('glacier', 'Gatorade Glacier Freeze', 1)],
        };
        return json(current);
      }
      throw new Error(`Unexpected ${method} ${url}`);
    });

    expect(await screen.findByRole('checkbox', { name: 'Gatorade Glacier Freeze · 1 on hand' })).toBeInTheDocument();
    expect(screen.getByRole('checkbox', { name: 'Lemonade powder' })).toBeInTheDocument();
    expect(screen.queryByRole('checkbox', { name: /Fruit punch/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('checkbox', { name: /Whole Milk/ })).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText('Search products'), { target: { value: 'gatorade' } });
    expect(screen.queryByRole('checkbox', { name: 'Lemonade powder' })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('checkbox', { name: 'Gatorade Glacier Freeze · 1 on hand' }));
    fireEvent.click(screen.getByRole('button', { name: 'Add to this group' }));

    expect(await screen.findByText('These products have their own supply setting. Which should the whole group use?')).toBeInTheDocument();
    expect(screen.getByText('Gatorade Glacier Freeze: 12 ounces')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Keep the account window' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Keep 24 ounces' })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Keep the household default' }));

    expect(await screen.findByText('Gatorade Glacier Freeze')).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByRole('checkbox', { name: /Gatorade Glacier Freeze/ })).not.toBeInTheDocument());
    expect(membersPosts).toBe(2);
  });

  it('keeps a 6 month group target instead of clearing it', async () => {
    let current: ProductGroup = { ...baseGroup(), windowMonths: 6 };
    const bodies: { productIds: string[]; target?: { windowMonths?: number; clear?: boolean } }[] = [];
    renderDetail((url, init) => {
      const method = init?.method ?? 'GET';
      if (url === '/api/groups/g1' && method === 'GET') return json(current);
      if (url === '/api/groups' && method === 'GET') return json([current]);
      if (url === '/api/products' && method === 'GET') return json([product('glacier', 'Gatorade Glacier Freeze')]);
      if (url === '/api/inventory') return json([inventoryRow('glacier', 'Gatorade Glacier Freeze', 1)]);
      if (url.startsWith('/api/products/')) return json({ barcodes: [] });
      if (url === '/api/groups/g1/members' && method === 'POST') {
        const body = JSON.parse(String(init?.body)) as { productIds: string[]; target?: { windowMonths?: number } };
        bodies.push(body);
        if (!body.target) {
          return json({
            error: 'Choose what this group should keep on hand.',
            code: 'target_decision_required',
            members: [{ productId: 'glacier', name: 'Gatorade Glacier Freeze', quantity: 12, dimension: 'mass' }],
          }, 409);
        }
        current = {
          ...current,
          members: [...current.members, member('glacier', 'Gatorade Glacier Freeze', 1)],
        };
        return json(current);
      }
      throw new Error(`Unexpected ${method} ${url}`);
    });

    fireEvent.click(await screen.findByRole('checkbox', { name: 'Gatorade Glacier Freeze · 1 on hand' }));
    fireEvent.click(screen.getByRole('button', { name: 'Add to this group' }));

    expect(await screen.findByRole('button', { name: "Keep the group's 6 months" })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Use 12 ounces (from Gatorade Glacier Freeze)' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: "Keep the group's 6 months" }));

    await waitFor(() => expect(bodies).toHaveLength(2));
    expect(bodies[1]?.target).toEqual({ windowMonths: 6 });
  });

  it('sends a member quantity and dimension, and cancel sends nothing', async () => {
    const current = baseGroup();
    const posts: { productIds: string[]; target?: { quantity?: number; dimension?: string } }[] = [];
    renderDetail((url, init) => {
      const method = init?.method ?? 'GET';
      if (url === '/api/groups/g1' && method === 'GET') return json(current);
      if (url === '/api/groups' && method === 'GET') return json([current]);
      if (url === '/api/products' && method === 'GET') return json([product('glacier', 'Gatorade Glacier Freeze')]);
      if (url === '/api/inventory') return json([inventoryRow('glacier', 'Gatorade Glacier Freeze', 1)]);
      if (url.startsWith('/api/products/')) return json({ barcodes: [] });
      if (url === '/api/groups/g1/members' && method === 'POST') {
        const body = JSON.parse(String(init?.body)) as { productIds: string[]; target?: { quantity?: number; dimension?: string } };
        posts.push(body);
        if (!body.target) {
          return json({
            error: 'Choose what this group should keep on hand.',
            code: 'target_decision_required',
            members: [{ productId: 'glacier', name: 'Gatorade Glacier Freeze', quantity: 12, dimension: 'mass' }],
          }, 409);
        }
        return json({
          ...current,
          quantity: body.target.quantity,
          dimension: body.target.dimension,
          members: [...current.members, member('glacier', 'Gatorade Glacier Freeze', 1)],
        });
      }
      throw new Error(`Unexpected ${method} ${url}`);
    });

    const checkbox = await screen.findByRole('checkbox', { name: 'Gatorade Glacier Freeze · 1 on hand' });
    fireEvent.click(checkbox);
    fireEvent.click(screen.getByRole('button', { name: 'Add to this group' }));

    expect(await screen.findByText('Gatorade Glacier Freeze: 12 ounces')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Use 12 ounces (from Gatorade Glacier Freeze)' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(screen.queryByText('These products have their own supply setting. Which should the whole group use?')).not.toBeInTheDocument();
    expect(screen.getByRole('checkbox', { name: 'Gatorade Glacier Freeze · 1 on hand' })).toBeChecked();
    expect(posts).toHaveLength(1);
    expect(posts[0]?.target).toBeUndefined();

    fireEvent.click(screen.getByRole('button', { name: 'Add to this group' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Use 12 ounces (from Gatorade Glacier Freeze)' }));

    await waitFor(() => expect(posts).toHaveLength(3));
    expect(posts[0]?.target).toBeUndefined();
    expect(posts[1]?.target).toBeUndefined();
    expect(posts[2]?.target).toEqual({ quantity: 12, dimension: 'mass' });
  });

  it('still renames the group and moves a member', async () => {
    let current = baseGroup();
    const calls: { url: string; body?: string }[] = [];
    renderDetail((url, init) => {
      const method = init?.method ?? 'GET';
      if (method !== 'GET') calls.push({ url, body: typeof init?.body === 'string' ? init.body : undefined });
      if (url === '/api/groups/g1' && method === 'GET') return json(current);
      if (url === '/api/groups' && method === 'GET') {
        return json([current, {
          id: 'g2',
          name: 'Drink mixes',
          rule: 'same_as_ran_out',
          ruleConfirmed: false,
          members: [],
          runningLow: false,
        }]);
      }
      if (url === '/api/products' && method === 'GET') return json([]);
      if (url === '/api/inventory') return json([]);
      if (url.startsWith('/api/products/')) return json({ barcodes: ['052000340075'] });
      if (url === '/api/groups/g1' && method === 'PATCH') {
        current = { ...current, name: 'Sports powder' };
        return json(current);
      }
      if (url === '/api/groups/g2/members' && method === 'POST') {
        current = { ...current, members: current.members.filter((item) => item.productId !== 'punch') };
        return json(current);
      }
      throw new Error(`Unexpected ${method} ${url}`);
    });

    const name = await screen.findByLabelText('Name');
    fireEvent.change(name, { target: { value: 'Sports powder' } });
    fireEvent.click(screen.getByRole('button', { name: 'Rename' }));
    expect(await screen.findByRole('heading', { name: 'Sports powder' })).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText('Move Fruit punch thirst quencher powder'), { target: { value: 'g2' } });
    fireEvent.click(screen.getAllByRole('button', { name: 'Move' })[0]);
    await waitFor(() => expect(calls.some((call) => call.url === '/api/groups/g2/members')).toBe(true));
    const move = calls.find((call) => call.url === '/api/groups/g2/members');
    expect(JSON.parse(move?.body ?? '{}')).toEqual({ productIds: ['punch'], fromGroupId: 'g1' });
    await waitFor(() => expect(screen.queryByText('Fruit punch thirst quencher powder')).not.toBeInTheDocument());
  });
});
