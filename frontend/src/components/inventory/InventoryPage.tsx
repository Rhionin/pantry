import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { trackEventSource } from '../../telemetry/client';
import { Alert, Button, Drawer, Group, Loader, Stack, Text, TextInput, Title, UnstyledButton } from '@mantine/core';
import {
  createGroup, getInventoryList, getProduct, getSupplySettings, listGroups, listProducts, listSuggestions, rescanSuggestions,
} from '../../api/client';
import type { GroupSuggestion, InventoryItem, Product, ProductGroup } from '../../types';
import { InboxPage } from '../groups/InboxPage';
import { ProductEditor } from '../product/ProductEditor';
import { AddToGroupSheet } from './AddToGroupSheet';
import { GroupRow } from './GroupRow';
import { GroupSelectBar } from './GroupSelectBar';
import { ItemInstanceList } from './ItemInstanceList';
import { ItemRow } from './ItemRow';
import { OpeningBanner } from './OpeningBanner';
import { mergeInventoryEvent } from './inventoryUtils';
import {
  asCatalog, asSuggestions, buildShelf, filterShelf, groupsForShelf, parseShelfFilter, type ShelfFilter, type ShelfMember,
} from './shelf';
import '../groups/groups.css';
import './shelf.css';

const ProductBarcodeLine = ({ productId }: { productId: string }) => {
  const [barcodes, setBarcodes] = useState<string[]>([]);

  useEffect(() => {
    let active = true;
    void getProduct(productId)
      .then((product) => {
        if (active) setBarcodes(product.barcodes ?? []);
      })
      .catch(() => {
        if (active) setBarcodes([]);
      });
    return () => {
      active = false;
    };
  }, [productId]);

  if (barcodes.length === 0) return null;

  return (
    <Text size="xs" c="dimmed" className="copyable-barcode">
      Barcode: {barcodes.join(', ')}
    </Text>
  );
};

const chips: { id: ShelfFilter; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'low', label: 'Running low' },
  { id: 'groups', label: 'Groups' },
  { id: 'ungrouped', label: 'Ungrouped' },
];

export const InventoryPage = () => {
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const filter = parseShelfFilter(params.get('filter'));
  const reviewOpen = params.get('review') === '1' || params.get('suggestion') !== null;
  const suggestionId = params.get('suggestion') ?? undefined;
  const [inventoryItems, setInventoryItems] = useState<InventoryItem[]>([]);
  const [groups, setGroups] = useState<ProductGroup[]>([]);
  const [suggestions, setSuggestions] = useState<GroupSuggestion[]>([]);
  const [catalog, setCatalog] = useState<Product[]>([]);
  const [householdMonths, setHouseholdMonths] = useState<number | undefined>();
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedItemId, setSelectedItemId] = useState<string | null>(null);
  const [expandedGroupId, setExpandedGroupId] = useState<string | null>(null);
  const [selecting, setSelecting] = useState(false);
  const [checked, setChecked] = useState<string[]>([]);
  const [sheet, setSheet] = useState<{ productId: string; name: string; fromGroupId?: string } | null>(null);
  const [loading, setLoading] = useState(true);
  const [scanning, setScanning] = useState(false);
  const [notice, setNotice] = useState('');
  const [error, setError] = useState('');
  const [opening, setOpening] = useState(false);
  const itemsRef = useRef<InventoryItem[]>([]);
  const rescanned = useRef(false);
  useEffect(() => {
    itemsRef.current = inventoryItems;
  }, [inventoryItems]);

  const loadInventory = useCallback(async (quiet = false) => {
    if (!quiet) setLoading(true);
    setError('');
    try {
      const [items, settings, listed, cards, products] = await Promise.all([
        getInventoryList(),
        getSupplySettings().catch(() => null),
        listGroups().catch(() => [] as ProductGroup[]),
        listSuggestions().catch(() => [] as GroupSuggestion[]),
        listProducts().catch(() => [] as Product[]),
      ]);
      setInventoryItems(items);
      setOpening(settings?.opening === true);
      setHouseholdMonths(settings?.months);
      setGroups(groupsForShelf(listed, items));
      setSuggestions(asSuggestions(cards));
      setCatalog(asCatalog(products));
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to load inventory.');
    } finally {
      if (!quiet) setLoading(false);
    }
  }, []);

  useEffect(() => {
    void Promise.resolve().then(() => loadInventory());
  }, [loadInventory]);

  useEffect(() => {
    const eventSource = new EventSource('/api/events');
    trackEventSource(eventSource);
    eventSource.addEventListener('inventory', (message) => {
      const inventoryItem = JSON.parse((message as MessageEvent).data) as InventoryItem;
      const current = itemsRef.current;
      const touchesGroup = inventoryItem.group != null || current.some((item) =>
        item.group != null && item.item.productId === inventoryItem.item.productId);
      if (touchesGroup) {
        void loadInventory(true);
        return;
      }
      setInventoryItems((rows) => mergeInventoryEvent(rows, inventoryItem));
    });
    return () => eventSource.close();
  }, [loadInventory]);

  const rescan = useCallback(async () => {
    setScanning(true);
    setNotice('');
    setError('');
    try {
      const created = await rescanSuggestions();
      if (created.length === 0) setNotice('No new groups found');
      await loadInventory(true);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to look for groups.');
    } finally {
      setScanning(false);
    }
  }, [loadInventory]);

  useEffect(() => {
    if (params.get('rescan') !== '1' || rescanned.current) return;
    rescanned.current = true;
    const next = new URLSearchParams(params);
    next.delete('rescan');
    setParams(next, { replace: true });
    void rescan();
  }, [params, rescan, setParams]);

  const rows = useMemo(
    () => filterShelf(buildShelf({
      items: inventoryItems,
      groups,
      suggestions,
      catalog,
      householdMonths,
    }), searchQuery, filter),
    [inventoryItems, groups, suggestions, catalog, householdMonths, searchQuery, filter],
  );

  const setFilter = (next: ShelfFilter) => {
    const updated = new URLSearchParams(params);
    if (next === 'all') updated.delete('filter');
    else updated.set('filter', next);
    setParams(updated, { replace: true });
  };

  const closeReview = () => {
    const updated = new URLSearchParams(params);
    updated.delete('review');
    updated.delete('suggestion');
    setParams(updated, { replace: true });
  };

  const openSuggestion = (id: string) => {
    const updated = new URLSearchParams(params);
    updated.set('suggestion', id);
    setParams(updated, { replace: true });
  };

  const toggleChecked = (productId: string, next: boolean) => {
    setChecked((current) => next ? [...new Set([...current, productId])] : current.filter((id) => id !== productId));
  };

  const startSelecting = (productId?: string) => {
    setSelecting(true);
    if (productId) toggleChecked(productId, true);
  };

  const selectItem = (itemId: string) => {
    setSelectedItemId((current) => current === itemId ? null : itemId);
  };

  const shiftOnHand = (itemId: string, delta: number) => {
    setInventoryItems((current) => current.map((entry) => (
      entry.item.id === itemId
        ? { ...entry, instanceCount: Math.max(0, entry.instanceCount + delta) }
        : entry
    )));
  };

  const startGroup = async (productId: string, name: string) => {
    setError('');
    try {
      const created = await createGroup(name, [productId]);
      navigate(`/groups/${created.id}`);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to create the group.');
    }
  };

  const renderExpanded = (inventoryItem: InventoryItem): ReactNode => (
    <Stack gap="sm">
      <ProductBarcodeLine productId={inventoryItem.item.productId} />
      <ItemInstanceList
        itemId={inventoryItem.item.id}
        productName={inventoryItem.item.product.name}
        onHand={inventoryItem.instanceCount}
        onHandChange={(delta) => shiftOnHand(inventoryItem.item.id, delta)}
        onInventoryChanged={() => { void loadInventory(true); }}
      />
      <ProductEditor
        productId={inventoryItem.item.productId}
        onSaved={() => void loadInventory()}
      />
    </Stack>
  );

  const empty = !loading && error === '' && inventoryItems.length === 0 && groups.length === 0;

  return (
    <Stack gap="sm" className="page-wide bin-page shelf-page">
      <Title order={1} size="h3" className="page-cluster">Inventory</Title>
      {opening && <OpeningBanner onComplete={() => setOpening(false)} />}
      <Group className="page-cluster" gap="xs" align="flex-end" wrap="nowrap">
        <TextInput
          size="xs"
          label="Search products or groups"
          placeholder="Search products or groups"
          value={searchQuery}
          onChange={(event) => setSearchQuery(event.currentTarget.value)}
          style={{ flex: 1 }}
        />
        <Button size="xs" variant={selecting ? 'filled' : 'light'} onClick={() => {
          if (selecting) {
            setSelecting(false);
            setChecked([]);
            return;
          }
          startSelecting();
        }}
        >
          {selecting ? 'Done' : 'Select'}
        </Button>
      </Group>
      <div className="shelf-filters" role="group" aria-label="Filter inventory">
        {chips.map((chip) => (
          <UnstyledButton
            key={chip.id}
            className="bin-chip"
            aria-pressed={filter === chip.id}
            onClick={() => setFilter(chip.id)}
          >
            {chip.label}
          </UnstyledButton>
        ))}
      </div>
      {filter === 'groups' && (
        <Button size="sm" variant="default" className="suggestion-rescan" loading={scanning} onClick={() => void rescan()}>
          Look for more groups
        </Button>
      )}
      {notice !== '' && <p className="shelf-notice">{notice}</p>}
      {loading && <Loader aria-label="Loading inventory" />}
      {error !== '' && <Alert color="red" py="xs">{error}</Alert>}
      {empty && <Text c="dimmed">Your inventory is empty.</Text>}
      {!loading && error === '' && !empty && rows.length === 0 && (
        <Text c="dimmed">No products or groups match.</Text>
      )}
      {rows.length > 0 && (
        <div className="shelf-list">
          {rows.map((row) => {
            if (row.kind === 'group') {
              return (
                <GroupRow
                  key={row.id}
                  row={row}
                  expanded={expandedGroupId === row.id}
                  onToggle={() => setExpandedGroupId((current) => current === row.id ? null : row.id)}
                  onMove={(member: ShelfMember) => setSheet({
                    productId: member.productId,
                    name: member.name,
                    fromGroupId: row.id,
                  })}
                  onChanged={() => { void loadInventory(true); }}
                  onHandChange={shiftOnHand}
                />
              );
            }
            const inventoryItem = row.item;
            const selected = selectedItemId === inventoryItem.item.id;
            return (
              <ItemRow
                key={row.id}
                inventoryItem={inventoryItem}
                hand={row.hand}
                suggestion={row.suggestion}
                selected={selected}
                controlsId={`inventory-item-${inventoryItem.item.id}`}
                onSelect={() => selectItem(inventoryItem.item.id)}
                onAdd={() => setSheet({ productId: inventoryItem.item.productId, name: inventoryItem.item.product.name })}
                onStartGroup={() => void startGroup(inventoryItem.item.productId, inventoryItem.item.product.name)}
                onOpenSuggestion={row.suggestion ? () => openSuggestion(row.suggestion?.id ?? '') : undefined}
                selecting={selecting}
                checked={checked.includes(inventoryItem.item.productId)}
                onChecked={(next) => toggleChecked(inventoryItem.item.productId, next)}
                onLongPress={() => startSelecting(inventoryItem.item.productId)}
              >
                {selected ? renderExpanded(inventoryItem) : null}
              </ItemRow>
            );
          })}
        </div>
      )}
      {selecting && checked.length > 0 && (
        <GroupSelectBar
          productIds={checked}
          groups={groups}
          onDone={() => {
            setSelecting(false);
            setChecked([]);
            void loadInventory(true);
          }}
        />
      )}
      {sheet && (
        <AddToGroupSheet
          opened
          productId={sheet.productId}
          productName={sheet.name}
          fromGroupId={sheet.fromGroupId}
          groups={groups}
          suggestions={suggestions}
          onClose={() => setSheet(null)}
          onChanged={() => {
            setSheet(null);
            void loadInventory(true);
          }}
        />
      )}
      <Drawer
        opened={reviewOpen}
        onClose={closeReview}
        position="right"
        size="md"
        title="Suggestions"
        classNames={{ content: 'bin-page' }}
        closeButtonProps={{ 'aria-label': 'Close' }}
      >
        <InboxPage embedded focusId={suggestionId} />
      </Drawer>
    </Stack>
  );
};
