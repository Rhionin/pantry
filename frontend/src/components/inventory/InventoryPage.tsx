import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { trackEventSource } from '../../telemetry/client';
import { Alert, Button, Group, Loader, Stack, Text, TextInput, Title } from '@mantine/core';
import { getInventoryList, getProduct, getSupplySettings, listGroups } from '../../api/client';
import type { InventoryItem, ProductGroup } from '../../types';
import { filterInventoryItems } from '../../utils/inventoryFilter';
import { ProductEditor } from '../product/ProductEditor';
import { GroupRow } from './GroupRow';
import { GroupSelectBar } from './GroupSelectBar';
import { ItemInstanceList } from './ItemInstanceList';
import { ItemRow } from './ItemRow';
import { clusterInventory, mergeInventoryEvent } from './inventoryUtils';
import { OpeningBanner } from './OpeningBanner';
import { RuleSheet } from './RuleSheet';

interface InventorySectionProps {
  heading: string;
  items: InventoryItem[];
  selectedItemId: string | null;
  expandedGroupId: string | null;
  selecting: boolean;
  checked: string[];
  onSelect: (itemId: string) => void;
  onToggleGroup: (groupId: string) => void;
  onEditRule: (groupId: string) => void;
  onChecked: (productId: string, next: boolean) => void;
  onLongPress: (productId: string) => void;
  renderExpanded: (item: InventoryItem) => ReactNode;
  onHandChange: (itemId: string, delta: number) => void;
  onInventoryChanged: () => void;
}

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

const InventorySection = ({
  heading,
  items,
  selectedItemId,
  expandedGroupId,
  selecting,
  checked,
  onSelect,
  onToggleGroup,
  onEditRule,
  onChecked,
  onLongPress,
  renderExpanded,
  onHandChange,
  onInventoryChanged,
}: InventorySectionProps) => (
  <Stack component="section" aria-label={heading} gap="xs">
    <Title order={2} size="h4" className="page-cluster">{heading}</Title>
    <div className="card-grid">
      {clusterInventory(items).map((cluster) => {
        const group = cluster.items[0]?.group;
        if (group) {
          return (
            <GroupRow
              key={cluster.key}
              group={group}
              items={cluster.items}
              expanded={expandedGroupId === group.id}
              onToggle={() => onToggleGroup(group.id)}
              onEditRule={() => onEditRule(group.id)}
              onHandChange={onHandChange}
              onInventoryChanged={onInventoryChanged}
            />
          );
        }
        const inventoryItem = cluster.items[0];
        if (!inventoryItem) return null;
        const selected = selectedItemId === inventoryItem.item.id;
        return (
          <ItemRow
            key={cluster.key}
            inventoryItem={inventoryItem}
            selected={selected}
            controlsId={`inventory-item-${inventoryItem.item.id}`}
            onSelect={() => onSelect(inventoryItem.item.id)}
            selecting={selecting}
            checked={checked.includes(inventoryItem.item.productId)}
            onChecked={(next) => onChecked(inventoryItem.item.productId, next)}
            onLongPress={() => onLongPress(inventoryItem.item.productId)}
          >
            {selected ? renderExpanded(inventoryItem) : null}
          </ItemRow>
        );
      })}
    </div>
  </Stack>
);

export const InventoryPage = () => {
  const [inventoryItems, setInventoryItems] = useState<InventoryItem[]>([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedItemId, setSelectedItemId] = useState<string | null>(null);
  const [expandedGroupId, setExpandedGroupId] = useState<string | null>(null);
  const [ruleGroupId, setRuleGroupId] = useState<string | null>(null);
  const [selecting, setSelecting] = useState(false);
  const [checked, setChecked] = useState<string[]>([]);
  const [existingGroups, setExistingGroups] = useState<ProductGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [opening, setOpening] = useState(false);
  const itemsRef = useRef<InventoryItem[]>([]);
  useEffect(() => {
    itemsRef.current = inventoryItems;
  }, [inventoryItems]);

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

  const filteredItems = useMemo(
    () => filterInventoryItems(inventoryItems, searchQuery),
    [inventoryItems, searchQuery],
  );
  const clustered = clusterInventory(filteredItems);
  const attentionIds = new Set(
    clustered
      .filter((cluster) => cluster.items.some((item) => item.needsAttention))
      .flatMap((cluster) => cluster.items.map((item) => item.item.id)),
  );
  const needsAttentionItems = filteredItems.filter((item) => attentionIds.has(item.item.id));
  const otherItems = filteredItems.filter((item) => !attentionIds.has(item.item.id));
  const ruleGroup = inventoryItems.find((item) => item.group?.id === ruleGroupId)?.group;

  const toggleChecked = (productId: string, next: boolean) => {
    setChecked((current) => next ? [...new Set([...current, productId])] : current.filter((id) => id !== productId));
  };

  const startSelecting = (productId?: string) => {
    setSelecting(true);
    if (productId) toggleChecked(productId, true);
    void listGroups().then(setExistingGroups).catch(() => setExistingGroups([]));
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

  const renderExpanded = (inventoryItem: InventoryItem) => (
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

  return (
    <Stack gap="sm" className="page-wide">
      <Title order={1} size="h3" className="page-cluster">Inventory</Title>
      {opening && <OpeningBanner onComplete={() => setOpening(false)} />}
      <Group className="page-cluster" gap="xs" align="flex-end">
        <TextInput
          size="xs"
          label="Search inventory"
          placeholder="Search by product name or category"
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
            expandedGroupId={expandedGroupId}
            selecting={selecting}
            checked={checked}
            onSelect={selectItem}
            onToggleGroup={(groupId) => setExpandedGroupId((current) => current === groupId ? null : groupId)}
            onEditRule={setRuleGroupId}
            onChecked={toggleChecked}
            onLongPress={startSelecting}
            renderExpanded={renderExpanded}
            onHandChange={shiftOnHand}
            onInventoryChanged={() => { void loadInventory(true); }}
          />
        </Alert>
      )}
      {otherItems.length > 0 && (
        <InventorySection
          heading="Inventory items"
          items={otherItems}
          selectedItemId={selectedItemId}
          expandedGroupId={expandedGroupId}
          selecting={selecting}
          checked={checked}
          onSelect={selectItem}
          onToggleGroup={(groupId) => setExpandedGroupId((current) => current === groupId ? null : groupId)}
          onEditRule={setRuleGroupId}
          onChecked={toggleChecked}
          onLongPress={startSelecting}
          renderExpanded={renderExpanded}
          onHandChange={shiftOnHand}
          onInventoryChanged={() => { void loadInventory(true); }}
        />
      )}
      {selecting && checked.length > 0 && (
        <GroupSelectBar
          productIds={checked}
          groups={existingGroups}
          onDone={() => {
            setSelecting(false);
            setChecked([]);
            void loadInventory(true);
          }}
        />
      )}
      {ruleGroup && (
        <RuleSheet
          group={ruleGroup}
          opened={ruleGroupId !== null}
          onClose={() => setRuleGroupId(null)}
          onSaved={() => { void loadInventory(true); }}
        />
      )}
    </Stack>
  );
};
