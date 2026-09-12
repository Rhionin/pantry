import { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Loader, SimpleGrid, Stack, Text, TextInput, Title } from '@mantine/core';
import { getInventoryList } from '../../api/client';
import type { InventoryItem } from '../../types';
import { filterInventoryItems } from '../../utils/inventoryFilter';
import { SuggestionPanel } from '../suggestions/SuggestionPanel';
import { ItemInstanceList } from './ItemInstanceList';
import { ItemRow } from './ItemRow';
import { mergeInventoryEvent } from './inventoryUtils';

interface InventorySectionProps {
  heading: string;
  items: InventoryItem[];
  selectedItemId: string | null;
  onSelect: (itemId: string) => void;
}

const InventorySection = ({ heading, items, selectedItemId, onSelect }: InventorySectionProps) => (
  <Stack component="section" aria-label={heading} gap="xs">
    <Title order={2} size="h4">{heading}</Title>
    <SimpleGrid cols={{ base: 1, xs: 2, sm: 3, md: 4 }} spacing="xs">
      {items.map((inventoryItem) => (
        <ItemRow
          key={inventoryItem.item.id}
          inventoryItem={inventoryItem}
          selected={selectedItemId === inventoryItem.item.id}
          controlsId={`inventory-item-${inventoryItem.item.id}`}
          onSelect={() => onSelect(inventoryItem.item.id)}
        />
      ))}
    </SimpleGrid>
  </Stack>
);

export const InventoryPage = () => {
  const [inventoryItems, setInventoryItems] = useState<InventoryItem[]>([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedItemId, setSelectedItemId] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const loadInventory = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      setInventoryItems(await getInventoryList());
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to load inventory.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void Promise.resolve().then(loadInventory);
  }, [loadInventory]);

  useEffect(() => {
    const eventSource = new EventSource('/api/events');
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
  const selectedItem = inventoryItems.find((item) => item.item.id === selectedItemId);

  const selectItem = (itemId: string) => {
    setSelectedItemId((current) => current === itemId ? null : itemId);
  };

  return (
    <Stack gap="sm">
      <Title order={1} size="h3">Inventory</Title>
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
          />
        </Alert>
      )}
      {otherItems.length > 0 && (
        <InventorySection
          heading="Inventory items"
          items={otherItems}
          selectedItemId={selectedItemId}
          onSelect={selectItem}
        />
      )}
      {selectedItem !== undefined && (
        <Stack gap="sm">
          <ItemInstanceList
            itemId={selectedItem.item.id}
            productName={selectedItem.item.product.name}
            onInventoryChanged={() => void loadInventory()}
          />
          <SuggestionPanel
            itemId={selectedItem.item.id}
            productName={selectedItem.item.product.name}
            onTargetQuantitySaved={() => void loadInventory()}
          />
        </Stack>
      )}
    </Stack>
  );
};
