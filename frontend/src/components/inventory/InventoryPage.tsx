import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { trackEventSource } from '../../telemetry/client';
import { Alert, Loader, SimpleGrid, Stack, Text, TextInput, Title } from '@mantine/core';
import { getInventoryList, getSupplySettings } from '../../api/client';
import type { InventoryItem } from '../../types';
import { filterInventoryItems } from '../../utils/inventoryFilter';
import { ProductEditor } from '../product/ProductEditor';
import { ItemInstanceList } from './ItemInstanceList';
import { ItemRow } from './ItemRow';
import { mergeInventoryEvent } from './inventoryUtils';
import { OpeningBanner } from './OpeningBanner';

interface InventorySectionProps {
  heading: string;
  items: InventoryItem[];
  selectedItemId: string | null;
  onSelect: (itemId: string) => void;
  renderExpanded: (item: InventoryItem) => ReactNode;
}

const InventorySection = ({
  heading,
  items,
  selectedItemId,
  onSelect,
  renderExpanded,
}: InventorySectionProps) => (
  <Stack component="section" aria-label={heading} gap="xs">
    <Title order={2} size="h4">{heading}</Title>
    <SimpleGrid cols={{ base: 1, xs: 2, sm: 3, md: 4 }} spacing="xs" style={{ alignItems: 'start' }}>
      {items.map((inventoryItem) => {
        const selected = selectedItemId === inventoryItem.item.id;
        return (
          <ItemRow
            key={inventoryItem.item.id}
            inventoryItem={inventoryItem}
            selected={selected}
            controlsId={`inventory-item-${inventoryItem.item.id}`}
            onSelect={() => onSelect(inventoryItem.item.id)}
          >
            {selected ? renderExpanded(inventoryItem) : null}
          </ItemRow>
        );
      })}
    </SimpleGrid>
  </Stack>
);

export const InventoryPage = () => {
  const [inventoryItems, setInventoryItems] = useState<InventoryItem[]>([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedItemId, setSelectedItemId] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [opening, setOpening] = useState(false);

  const loadInventory = useCallback(async (quiet = false) => {
    if (!quiet) {
      setLoading(true);
    }
    setError('');
    try {
      const [items, settings] = await Promise.all([
        getInventoryList(),
        getSupplySettings().catch(() => null),
      ]);
      setInventoryItems(items);
      setOpening(settings?.opening === true);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to load inventory.');
    } finally {
      if (!quiet) {
        setLoading(false);
      }
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
      setInventoryItems((current) => mergeInventoryEvent(current, inventoryItem));
    });
    return () => eventSource.close();
  }, []);

  const filteredItems = useMemo(
    () => filterInventoryItems(inventoryItems, searchQuery),
    [inventoryItems, searchQuery],
  );
  const needsAttentionItems = filteredItems.filter((item) => item.needsAttention);
  const otherItems = filteredItems.filter((item) => !item.needsAttention);

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

  const renderExpanded = (inventoryItem: InventoryItem) => (
    <Stack gap="sm">
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

  return (
    <Stack gap="sm">
      <Title order={1} size="h3">Inventory</Title>
      {opening && <OpeningBanner onComplete={() => setOpening(false)} />}
      <TextInput
        size="xs"
        label="Search inventory"
        placeholder="Search by product name or category"
        value={searchQuery}
        onChange={(event) => setSearchQuery(event.currentTarget.value)}
      />
      {loading && <Loader aria-label="Loading inventory" />}
      {error !== '' && (
        <Alert color="red" py="xs">
          {error}
        </Alert>
      )}
      {!loading && error === '' && inventoryItems.length === 0 && (
        <Text c="dimmed">Your inventory is empty.</Text>
      )}
      {!loading && error === '' && inventoryItems.length > 0 && filteredItems.length === 0 && (
        <Text c="dimmed">No inventory items match your search.</Text>
      )}
      {needsAttentionItems.length > 0 && (
        <Alert color="yellow" title="Items expiring soon or already expired">
          <InventorySection
            heading="Needs Attention"
            items={needsAttentionItems}
            selectedItemId={selectedItemId}
            onSelect={selectItem}
            renderExpanded={renderExpanded}
          />
        </Alert>
      )}
      {otherItems.length > 0 && (
        <InventorySection
          heading="Inventory items"
          items={otherItems}
          selectedItemId={selectedItemId}
          onSelect={selectItem}
          renderExpanded={renderExpanded}
        />
      )}
    </Stack>
  );
};
