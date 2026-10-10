import { fireEvent, render, screen } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import type { ProductGroup } from '../../types';
import { GroupsPage } from './GroupsPage';

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });

const group = (overrides: Partial<ProductGroup>): ProductGroup => ({
  id: 'g1',
  name: 'Cut green beans',
  rule: 'same_as_ran_out',
  ruleConfirmed: false,
  members: [{ productId: 'gv', name: 'Great Value Cut Green Beans', onHand: 2 }],
  runningLow: false,
  ...overrides,
});

const groups: ProductGroup[] = [
  group({ id: 'beans', name: 'Cut green beans', runningLow: true }),
  group({
    id: 'milk',
    name: 'Milk',
    rule: 'favorite',
    ruleConfirmed: true,
    runningLow: false,
    members: [{ productId: 'm', name: 'Kroger Milk', onHand: 1 }],
  }),
];

const renderPage = () => {
  vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
    const url = String(input);
    if (url === '/api/groups') return Promise.resolve(jsonResponse(groups));
    if (url === '/api/group-suggestions') {
      return Promise.resolve(jsonResponse([
        { id: 's1', kind: 'looks_alike', title: 'Peanut butter', status: 'open', members: [] },
        { id: 's2', kind: 'from_old_plan', title: 'Soup', status: 'open', members: [] },
      ]));
    }
    throw new Error(`Unexpected request: ${url}`);
  }));
  return render(
    <MantineProvider>
      <MemoryRouter>
        <GroupsPage />
      </MemoryRouter>
    </MantineProvider>,
  );
};

describe('GroupsPage', () => {
  it('filters by chip and links to the suggestion split', async () => {
    renderPage();
    expect(await screen.findByText('Cut green beans')).toBeInTheDocument();
    expect(screen.getByText('Milk')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /2 suggestions to review/ })).toHaveTextContent('1 from the old shopping plan');
    expect(screen.getByRole('link', { name: /2 suggestions to review/ })).toHaveTextContent('1 look alike');

    fireEvent.click(screen.getByRole('button', { name: 'Running low' }));
    expect(screen.getByText('Cut green beans')).toBeInTheDocument();
    expect(screen.queryByText('Milk')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Pick a rule' }));
    expect(screen.getByText('Cut green beans')).toBeInTheDocument();
    expect(screen.queryByText('Milk')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Always my favorite' }));
    expect(screen.queryByText('Cut green beans')).not.toBeInTheDocument();
    expect(screen.getByText('Milk')).toBeInTheDocument();
  });

  it('opens the new group after creating it', async () => {
    const Pathname = () => {
      const { pathname } = useLocation();
      return <p>Path {pathname}</p>;
    };
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? 'GET';
      if (url === '/api/groups' && method === 'GET') return Promise.resolve(jsonResponse(groups));
      if (url === '/api/groups' && method === 'POST') {
        return Promise.resolve(jsonResponse(group({ id: 'g-soup', name: 'Soup', members: [] })));
      }
      if (url === '/api/group-suggestions') return Promise.resolve(jsonResponse([]));
      if (url === '/api/products') return Promise.resolve(jsonResponse([]));
      if (url === '/api/settings/supply') return Promise.resolve(jsonResponse({ months: 3 }));
      throw new Error(`Unexpected request: ${method} ${url}`);
    }));
    render(
      <MantineProvider>
        <MemoryRouter initialEntries={['/groups']}>
          <Pathname />
          <GroupsPage />
        </MemoryRouter>
      </MantineProvider>,
    );

    fireEvent.click(await screen.findByRole('button', { name: 'New group' }));
    fireEvent.change(await screen.findByRole('textbox', { name: 'Name' }), { target: { value: 'Soup' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create group' }));
    expect(await screen.findByText('Path /groups/g-soup')).toBeInTheDocument();
  });
});
