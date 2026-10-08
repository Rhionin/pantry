import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Badge,
  Button,
  Checkbox,
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
  clearBrandPreference,
  clearItemDeal,
  fillShoppingCart,
  getInventoryList,
  getShoppingConsiderations,
  getShoppingList,
  listProviders,
  markShoppingListItemPurchased,
  removeShoppingListItem,
  saveBrandPreference,
  swapShoppingLine,
  saveItemDeal,
  setShoppingListAdjustment,
} from '../../api/client';
import type { InventoryItem, ProviderInfo, ShoppingConsideration, ShoppingConsiderations, ShoppingListEntry } from '../../types';
import { ruleLabel } from '../groups/copy';
import { useCredentialsRevision } from '../../credentialsRefresh';
import { ProviderPanel } from './ProviderPanel';
import { ProvisionButton } from './ProvisionButton';

const requestErrorMessage = (error: unknown, fallback: string) =>
  error instanceof Error ? error.message : fallback;

const emptyNotes: ShoppingConsiderations = {
  retailerDeals: 'unavailable',
  retailerDetail: '',
  considerations: [],
};

const formatCents = (cents: number) => `$${(cents / 100).toFixed(2)}`;

const offerSentence = (note: ShoppingConsideration) => {
  const offer = note.offer;
  if (offer === null) return '';
  const usual = note.members.find((member) => member.itemId === note.chosenItemId)?.name ?? 'the usual brand';
  const noteLabel = offer.label !== '' && offer.label !== 'On sale' ? ` (${offer.label})` : '';
  if (offer.priceCents !== null && offer.usualPriceCents !== null) {
    return `${offer.name} is ${formatCents(offer.priceCents)}${noteLabel}, compared with ${formatCents(offer.usualPriceCents)} for ${usual}.`;
  }
  if (offer.priceCents !== null) {
    return `${offer.name} is on sale at ${formatCents(offer.priceCents)}${noteLabel}. This list buys ${usual}.`;
  }
  return `${offer.name} is on sale${noteLabel}. This list buys ${usual}.`;
};

export const ShoppingListPage = () => {
  const [entries, setEntries] = useState<ShoppingListEntry[]>([]);
  const [providers, setProviders] = useState<ProviderInfo[]>([]);
  const [inventory, setInventory] = useState<InventoryItem[]>([]);
  const [notes, setNotes] = useState<ShoppingConsiderations>(emptyNotes);
  const [accepted, setAccepted] = useState<Record<string, string>>({});
  const [selectedItemId, setSelectedItemId] = useState('');
  const [quantity, setQuantity] = useState<number | string>(1);
  const [saleItemId, setSaleItemId] = useState('');
  const [salePrice, setSalePrice] = useState<number | string>('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [filling, setFilling] = useState(false);
  const [decisionsOpen, setDecisionsOpen] = useState(false);
  const [error, setError] = useState('');

  const refresh = useCallback(async () => {
    const providerRows = await listProviders();
    const target = providerRows.find((row) => row.credentialsConfigured) ?? providerRows[0];
    const [shoppingEntries, inventoryItems] = await Promise.all([
      getShoppingList(target?.id),
      getInventoryList(),
    ]);
    setProviders(providerRows);
    setEntries(shoppingEntries);
    setInventory(inventoryItems);
    try {
      setNotes(await getShoppingConsiderations());
    } catch {
      setNotes(emptyNotes);
    }
  }, []);

  const loadShoppingList = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      await refresh();
    } catch (requestError) {
      setError(requestErrorMessage(requestError, 'Unable to load the shopping list.'));
    } finally {
      setLoading(false);
    }
  }, [refresh]);

  useEffect(() => {
    void Promise.resolve().then(loadShoppingList);
  }, [loadShoppingList]);

  const credentialsRevision = useCredentialsRevision();
  useEffect(() => {
    if (credentialsRevision === 0) return;
    void Promise.resolve().then(loadShoppingList);
  }, [credentialsRevision, loadShoppingList]);

  const targetProvider = providers.find((row) => row.credentialsConfigured) ?? providers[0] ?? null;

  const changeAdjustment = async (entry: ShoppingListEntry, value: number | string) => {
    if (entry.id === '' || targetProvider === null) return;
    const nextQuantity = typeof value === 'number' ? value : Number(value);
    if (!Number.isInteger(nextQuantity) || nextQuantity < 0 || nextQuantity > 999) return;
    setError('');
    try {
      await setShoppingListAdjustment(entry.id, entry.provider || targetProvider.id, nextQuantity);
      await loadShoppingList();
    } catch (requestError) {
      setError(requestErrorMessage(requestError, 'Unable to save the quantity adjustment.'));
    }
  };

  // A taken deal only applies while that offer is still the one on the line.
  // Deriving it here drops a stale choice when the notes refresh, without
  // writing state from an effect.
  const acceptedDeals = useMemo(() => {
    const next: Record<string, string> = {};
    for (const [lineId, useId] of Object.entries(accepted)) {
      const note = notes.considerations.find((item) => item.lineItemId === lineId);
      if (note?.offer?.itemId === useId) next[lineId] = useId;
    }
    return next;
  }, [accepted, notes]);

  const inventoryByItemId = useMemo(
    () => new Map(inventory.map((inventoryItem) => [inventoryItem.item.id, inventoryItem])),
    [inventory],
  );
  const notesByLine = useMemo(
    () => new Map(notes.considerations.map((note) => [note.lineItemId, note])),
    [notes],
  );
  const saleBrands = useMemo(() => {
    const byId = new Map<string, string>();
    for (const note of notes.considerations) {
      for (const member of note.members) {
        byId.set(member.itemId, member.name);
      }
    }
    return [...byId.entries()].map(([value, label]) => ({ value, label }));
  }, [notes]);
  const selectedSale = useMemo(() => {
    for (const note of notes.considerations) {
      const member = note.members.find((item) => item.itemId === saleItemId);
      if (member) return member;
    }
    return undefined;
  }, [notes, saleItemId]);
  const offers = notes.considerations.filter((note) => note.offer !== null);
  const showDecisions = decisionsOpen || entries.length > 0;
  const listAction = entries.length > 0 ? 'Update the list' : 'Build the list';
  const manualQuantity = typeof quantity === 'number' ? quantity : Number(quantity);
  const manualQuantityIsValid = quantity !== '' && Number.isInteger(manualQuantity) && manualQuantity >= 1;
  const salePriceNumber = typeof salePrice === 'number' ? salePrice : Number(salePrice);
  const salePriceIsValid = salePrice !== '' && Number.isInteger(salePriceNumber) && salePriceNumber >= 0;

  const fillCart = async () => {
    setFilling(true);
    setError('');
    try {
      await fillShoppingCart();
      setDecisionsOpen(true);
      await refresh();
    } catch (requestError) {
      setError(requestErrorMessage(requestError, 'Unable to build the list.'));
    } finally {
      setFilling(false);
    }
  };

  const addManualItem = async () => {
    if (selectedItemId === '' || !manualQuantityIsValid) return;
    setSaving(true);
    setError('');
    try {
      await addShoppingListItem(selectedItemId, manualQuantity);
      setSelectedItemId('');
      setQuantity(1);
      await refresh();
    } catch (requestError) {
      setError(requestErrorMessage(requestError, 'Unable to add the shopping list item.'));
    } finally {
      setSaving(false);
    }
  };

  const swapLine = async (entry: ShoppingListEntry, itemId: string) => {
    if (entry.id === '' || itemId === entry.itemId) return;
    setError('');
    try {
      await swapShoppingLine(entry.id, itemId);
      await refresh();
    } catch (requestError) {
      setError(requestErrorMessage(requestError, 'Unable to change the product on this line.'));
    }
  };

  const updateEntry = async (entry: ShoppingListEntry, action: 'purchase' | 'remove') => {
    if (entry.id === '') return;
    setError('');
    try {
      if (action === 'purchase') await markShoppingListItemPurchased(entry.id);
      else await removeShoppingListItem(entry.id);
      await refresh();
    } catch (requestError) {
      setError(requestErrorMessage(requestError, `Unable to ${action} the shopping list item.`));
    }
  };

  const saveBrand = async (note: ShoppingConsideration, itemId: string, ignorePrice: boolean) => {
    setError('');
    try {
      if (itemId === '') {
        await clearBrandPreference(note.members[0]?.itemId ?? note.lineItemId);
      } else {
        await saveBrandPreference(itemId, ignorePrice);
      }
      await refresh();
    } catch (requestError) {
      setError(requestErrorMessage(requestError, 'Unable to save the brand preference.'));
    }
  };

  const noteSale = async () => {
    if (saleItemId === '' || !salePriceIsValid) return;
    setError('');
    try {
      await saveItemDeal(saleItemId, salePriceNumber, 'Noted sale');
      setSalePrice('');
      await refresh();
    } catch (requestError) {
      setError(requestErrorMessage(requestError, 'Unable to note the sale.'));
    }
  };

  const clearSale = async () => {
    if (saleItemId === '') return;
    setError('');
    try {
      await clearItemDeal(saleItemId);
      await refresh();
    } catch (requestError) {
      setError(requestErrorMessage(requestError, 'Unable to clear the sale.'));
    }
  };

  const toggleDeal = (note: ShoppingConsideration) => {
    const offer = note.offer;
    if (offer === null) return;
    setAccepted((current) => {
      const next = { ...current };
      if (next[note.lineItemId] === offer.itemId) {
        delete next[note.lineItemId];
      } else {
        next[note.lineItemId] = offer.itemId;
      }
      return next;
    });
  };

  return (
    <Stack gap="sm">
      <div className="shopping-toolbar">
        <Title order={1} size="h3" className="shopping-toolbar-title">Shopping plan</Title>
        <div className="shopping-toolbar-actions">
          <Button
            className="shopping-fill"
            loading={filling}
            disabled={loading}
            onClick={() => void fillCart()}
          >
            {listAction}
          </Button>
          <ProvisionButton
            provider={targetProvider}
            entries={entries}
            useItemIds={acceptedDeals}
            explainEmpty={!loading}
            onFinished={() => void loadShoppingList()}
          />
        </div>
      </div>
      <ProviderPanel providers={providers} onChanged={() => void loadShoppingList()} />
      {!loading && showDecisions && notes.considerations.length > 0 && (
        <Alert variant="light" color="teal" title={offers.length > 0 ? 'A sale to consider' : 'Brand notes'}>
          <Stack gap="xs">
            {offers.map((note) => {
              const offer = note.offer;
              if (offer === null) return null;
              const usual = note.members.find((member) => member.itemId === note.chosenItemId)?.name ?? 'the usual brand';
              const taken = acceptedDeals[note.lineItemId] === offer.itemId;
              return (
                <Group key={note.lineItemId} justify="space-between" align="center" wrap="wrap">
                  <Text size="sm">{offerSentence(note)}</Text>
                  <Button size="xs" variant={taken ? 'filled' : 'light'} onClick={() => toggleDeal(note)}>
                    {taken ? `Keep ${usual}` : `Take the deal on ${offer.name}`}
                  </Button>
                </Group>
              );
            })}
            <Text size="xs" c="dimmed">Send to Kroger sends the usual brand until you take a deal.</Text>
            {notes.retailerDetail !== '' && <Text size="xs" c="dimmed">{notes.retailerDetail}</Text>}
            <Group align="end" gap="xs" wrap="wrap">
              <NativeSelect
                size="xs"
                label="Brand on sale"
                value={saleItemId}
                onChange={(event) => setSaleItemId(event.currentTarget.value)}
                data={[{ value: '', label: 'Choose a brand' }, ...saleBrands]}
              />
              <NumberInput
                size="xs"
                label="Sale price in cents"
                min={0}
                step={1}
                allowDecimal={false}
                value={salePrice}
                onChange={setSalePrice}
                w={160}
              />
              <Button size="xs" variant="light" disabled={saleItemId === '' || !salePriceIsValid} onClick={() => void noteSale()}>
                Note sale
              </Button>
              <Button
                size="xs"
                variant="default"
                disabled={saleItemId === '' || selectedSale?.onSale !== true}
                onClick={() => void clearSale()}
              >
                Clear sale
              </Button>
            </Group>
          </Stack>
        </Alert>
      )}
      {showDecisions && <Stack component="form" gap="xs" onSubmit={(event) => {
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
      </Stack>}
      {loading && <Loader aria-label="Loading shopping plan" />}
      {error !== '' && (
        <Alert color="red" py="xs">
          {error}
        </Alert>
      )}
      {!loading && showDecisions && entries.length > 0 && (
        <div className="shopping-entries-scroll">
          <Table className="shopping-entries" aria-label="Shopping plan entries">
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
                const title = entry.group ? entry.group.name : productName;
                const unit = inventoryItem?.item.product.unitOfMeasure ?? 'units';
                const note = entry.group ? undefined : notesByLine.get(entry.itemId);
                return (
                  <Table.Tr key={entry.id === '' ? `auto-${entry.itemId}` : entry.id}>
                    <Table.Td>
                      <Stack gap={2}>
                        <Text fw={600}>{title}</Text>
                        {entry.group && <Text size="sm">{productName}</Text>}
                        {entry.group && (
                          <Badge variant="light" color={entry.group.ruleConfirmed ? undefined : 'yellow'} w="fit-content">
                            {entry.group.ruleConfirmed ? ruleLabel(entry.group.rule) : 'Pick a rule'}
                          </Badge>
                        )}
                        {entry.note !== undefined && entry.note !== '' && (
                          <Text size="sm" c="dimmed">{entry.note}</Text>
                        )}
                        {entry.group && entry.group.members.length > 1 && (
                          <NativeSelect
                            size="xs"
                            label="This trip, buy"
                            value={entry.itemId}
                            data={entry.group.members.map((member) => ({ value: member.itemId, label: member.name }))}
                            onChange={(event) => void swapLine(entry, event.currentTarget.value)}
                          />
                        )}
                        {note && (
                          <Group gap="xs" align="end" wrap="wrap">
                            <NativeSelect
                              size="xs"
                              label={`Preferred brand for ${note.genericName}`}
                              value={note.preferredItemId}
                              onChange={(event) => {
                                const itemId = event.currentTarget.value;
                                void saveBrand(note, itemId, itemId === '' ? false : note.ignorePrice);
                              }}
                              data={[
                                { value: '', label: 'No preference' },
                                ...note.members.map((member) => ({ value: member.itemId, label: member.name })),
                              ]}
                            />
                            <Checkbox
                              size="xs"
                              mt="lg"
                              label={`Always buy this brand of ${note.genericName}`}
                              checked={note.ignorePrice}
                              disabled={note.preferredItemId === ''}
                              onChange={(event) => {
                                if (note.preferredItemId === '') return;
                                void saveBrand(note, note.preferredItemId, event.currentTarget.checked);
                              }}
                            />
                          </Group>
                        )}
                      </Stack>
                    </Table.Td>
                    <Table.Td data-label="Quantity">
                      <Stack gap={4}>
                        <Text>{entry.quantity} {unit}</Text>
                        {entry.adjustment !== undefined && entry.computedQuantity !== undefined && (
                          <Text size="sm" c="dimmed">Computed {entry.computedQuantity} {unit}</Text>
                        )}
                        {entry.id !== '' && targetProvider !== null && (
                          <NumberInput
                            size="xs"
                            aria-label={`Provision quantity for ${productName}`}
                            min={0}
                            max={999}
                            step={1}
                            allowDecimal={false}
                            defaultValue={entry.quantity}
                            key={`${entry.id}-${entry.quantity}`}
                            onBlur={(event) => void changeAdjustment(entry, event.currentTarget.value)}
                            w={120}
                          />
                        )}
                        {entry.basis && (
                          <Text size="xs" c="dimmed">
                            {`${entry.basis.instanceCount} on hand`}
                          </Text>
                        )}
                      </Stack>
                    </Table.Td>
                    <Table.Td data-label="Source"><Badge variant="light">{entry.source === 'auto' ? 'Derived' : 'Manual'}</Badge></Table.Td>
                    <Table.Td data-label="Actions">
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
        </div>
      )}
    </Stack>
  );
};
