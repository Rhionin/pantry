import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Badge,
  Button,
  Group,
  Loader,
  NativeSelect,
  NumberInput,
  Stack,
  Table,
  Text,
  Title,
} from '@mantine/core';
import {
  addShoppingListItem,
  getInventoryList,
  getShoppingList,
  markShoppingListItemPurchased,
  removeShoppingListItem,
} from '../../api/client';
import type { InventoryItem, ShoppingListEntry } from '../../types';
import { CartExportButton } from './CartExportButton';

const requestErrorMessage = (error: unknown, fallback: string) =>
  error instanceof Error ? error.message : fallback;

export const ShoppingListPage = () => {
  const [entries, setEntries] = useState<ShoppingListEntry[]>([]);
  const [inventory, setInventory] = useState<InventoryItem[]>([]);
  const [selectedItemId, setSelectedItemId] = useState('');
  const [quantity, setQuantity] = useState<number | string>(1);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const loadShoppingList = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const [shoppingEntries, inventoryItems] = await Promise.all([
        getShoppingList(),
        getInventoryList(),
      ]);
      setEntries(shoppingEntries);
      setInventory(inventoryItems);
    } catch (requestError) {
      setError(requestErrorMessage(requestError, 'Unable to load the shopping list.'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void Promise.resolve().then(loadShoppingList);
  }, [loadShoppingList]);

  const inventoryByItemId = useMemo(
    () => new Map(inventory.map((inventoryItem) => [inventoryItem.item.id, inventoryItem])),
    [inventory],
  );
  const manualQuantity = typeof quantity === 'number' ? quantity : Number(quantity);
  const manualQuantityIsValid = quantity !== '' && Number.isInteger(manualQuantity) && manualQuantity >= 1;

  const addManualItem = async () => {
    if (selectedItemId === '' || !manualQuantityIsValid) return;
    setSaving(true);
    setError('');
    try {
      await addShoppingListItem(selectedItemId, manualQuantity);
      setSelectedItemId('');
      setQuantity(1);
      await loadShoppingList();
    } catch (requestError) {
      setError(requestErrorMessage(requestError, 'Unable to add the shopping list item.'));
    } finally {
      setSaving(false);
    }
  };

  const updateEntry = async (entry: ShoppingListEntry, action: 'purchase' | 'remove') => {
    if (entry.id === '') return;
    setError('');
    try {
      if (action === 'purchase') await markShoppingListItemPurchased(entry.id);
      else await removeShoppingListItem(entry.id);
      await loadShoppingList();
    } catch (requestError) {
      setError(requestErrorMessage(requestError, `Unable to ${action} the shopping list item.`));
    }
  };

  return (
    <Stack gap="sm">
      <Group justify="space-between">
        <Title order={1} size="h3">Shopping list</Title>
        <CartExportButton disabled={entries.length === 0} />
      </Group>
      <Stack component="form" gap="xs" onSubmit={(event) => {
        event.preventDefault();
        void addManualItem();
      }}>
        <Title order={2} size="h5">Add an item</Title>
        <Group align="end" gap="xs" wrap="wrap">
          <NativeSelect
            size="xs"
            label="Pantry item"
            value={selectedItemId}
            onChange={(event) => setSelectedItemId(event.currentTarget.value)}
            data={[
              { value: '', label: 'Choose an item' },
              ...inventory.map((inventoryItem) => ({
                value: inventoryItem.item.id,
                label: inventoryItem.item.product.name,
              })),
            ]}
          />
          <NumberInput
            size="xs"
            label="Quantity"
            min={1}
            step={1}
            allowDecimal={false}
            value={quantity}
            onChange={setQuantity}
            w={100}
          />
          <Button
            size="xs"
            type="submit"
            loading={saving}
            disabled={selectedItemId === '' || !manualQuantityIsValid}
          >
            Add to shopping list
          </Button>
        </Group>
      </Stack>
      {loading && <Loader aria-label="Loading shopping list" />}
      {error !== '' && (
        <Alert color="red" py="xs">
          {error}
        </Alert>
      )}
      {!loading && error === '' && entries.length === 0 && (
        <Text c="dimmed">Your shopping list is empty.</Text>
      )}
      {!loading && entries.length > 0 && (
        <Table.ScrollContainer minWidth={680}>
          <Table aria-label="Shopping list entries">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Item</Table.Th>
                <Table.Th>Quantity</Table.Th>
                <Table.Th>Source</Table.Th>
                <Table.Th>Actions</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {entries.map((entry) => {
                const inventoryItem = inventoryByItemId.get(entry.itemId);
                const productName = inventoryItem?.item.product.name ?? `Item ${entry.itemId}`;
                const unit = inventoryItem?.item.product.unitOfMeasure ?? 'units';
                const needsTarget = entry.source === 'manual' && inventoryItem?.item.targetQuantity === null;
                return (
                  <Table.Tr key={entry.id === '' ? `auto-${entry.itemId}` : entry.id}>
                    <Table.Td>
                      <Stack gap={2}>
                        <Text fw={600}>{productName}</Text>
                        {needsTarget && <Text size="sm" c="dimmed">Set a target quantity for automatic restocking.</Text>}
                      </Stack>
                    </Table.Td>
                    <Table.Td>{entry.quantity} {unit}</Table.Td>
                    <Table.Td><Badge variant="light">{entry.source === 'auto' ? 'Derived' : 'Manual'}</Badge></Table.Td>
                    <Table.Td>
                      {entry.id === '' ? (
                        <Text size="sm" c="dimmed">Updates when inventory changes</Text>
                      ) : (
                        <Group gap="xs">
                          <Button size="xs" onClick={() => void updateEntry(entry, 'purchase')}>
                            Mark {productName} purchased
                          </Button>
                          <Button
                            size="xs"
                            color="red"
                            variant="light"
                            onClick={() => void updateEntry(entry, 'remove')}
                          >
                            Remove {productName}
                          </Button>
                        </Group>
                      )}
                    </Table.Td>
                  </Table.Tr>
                );
              })}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      )}
    </Stack>
  );
};
