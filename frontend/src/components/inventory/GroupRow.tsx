import { useState } from 'react';
import { Badge, Button, Card, Divider, Group, Stack, Text, Title } from '@mantine/core';
import type { InventoryGroup, InventoryItem } from '../../types';
import { ProductEditor } from '../product/ProductEditor';
import { ruleLabel, targetLabel } from '../groups/copy';
import { ItemInstanceList } from './ItemInstanceList';

export interface GroupRowProps {
  group: InventoryGroup;
  items: InventoryItem[];
  expanded: boolean;
  onToggle: () => void;
  onEditRule: () => void;
  onHandChange: (itemId: string, delta: number) => void;
  onInventoryChanged: () => void;
}

export const GroupRow = ({
  group,
  items,
  expanded,
  onToggle,
  onEditRule,
  onHandChange,
  onInventoryChanged,
}: GroupRowProps) => {
  const [instancesFor, setInstancesFor] = useState<string | null>(null);
  return (
  <Card component="article" withBorder padding="sm">
    <Stack gap="xs">
      <Stack gap={2}>
        <Title order={3} size="h5">{group.name}</Title>
        <Text size="sm">{group.onHand} on hand · {targetLabel(group)} · {group.memberCount} products</Text>
        <Group gap="xs">
          {group.ruleConfirmed ? (
            <Badge variant="light">{ruleLabel(group.rule)}</Badge>
          ) : (
            <Badge color="yellow" variant="light">Pick a rule</Badge>
          )}
        </Group>
        {!group.ruleConfirmed && (
          <Text size="sm">Still using {ruleLabel(group.rule)} until you pick a rule.</Text>
        )}
      </Stack>
      <Group gap="xs">
        <Button size="compact-sm" variant="subtle" color="gray" aria-expanded={expanded} onClick={onToggle}>
          {expanded ? 'Hide products' : 'Show products'}
        </Button>
        <Button size="compact-sm" variant="light" onClick={onEditRule}>Edit rule</Button>
      </Group>
      {expanded && (
        <>
          <Divider />
          {group.members.map((member) => {
            const inventoryItem = items.find((item) => item.item.productId === member.productId);
            return (
              <Stack key={member.productId} gap={4}>
                <Text size="sm">{member.name} · {member.onHand} on hand</Text>
                {member.barcodes && member.barcodes.length > 0 && (
                  <Text size="xs" c="dimmed" className="copyable-barcode">Barcode: {member.barcodes.join(', ')}</Text>
                )}
                {inventoryItem && (
                  <Button
                    size="compact-xs"
                    variant="subtle"
                    onClick={() => setInstancesFor((current) => current === member.productId ? null : member.productId)}
                  >
                    Instances
                  </Button>
                )}
                {inventoryItem && instancesFor === member.productId && (
                  <ItemInstanceList
                    itemId={inventoryItem.item.id}
                    productName={member.name}
                    onHand={inventoryItem.instanceCount}
                    onHandChange={(delta) => onHandChange(inventoryItem.item.id, delta)}
                    onInventoryChanged={onInventoryChanged}
                  />
                )}
                <ProductEditor
                  productId={member.productId}
                  group={{ id: group.id, name: group.name }}
                  onSaved={onInventoryChanged}
                />
              </Stack>
            );
          })}
        </>
      )}
    </Stack>
  </Card>
  );
};
