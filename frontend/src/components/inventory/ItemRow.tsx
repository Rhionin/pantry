import { memo } from 'react';
import { Badge, Button, Card, Group, Stack, Text, Title } from '@mantine/core';
import type { InventoryItem } from '../../types';

export interface ItemRowProps {
  inventoryItem: InventoryItem;
  selected: boolean;
  controlsId: string;
  onSelect: () => void;
}

export const ItemRow = memo(({
  inventoryItem,
  selected,
  controlsId,
  onSelect,
}: ItemRowProps) => {
  const { item, instanceCount, nearExpiryCount, expiredCount } = inventoryItem;

  return (
    <Card component="article" withBorder>
      <Group justify="space-between" align="flex-start" wrap="nowrap">
        <Stack gap={2}>
          <Title order={3}>{item.product.name}</Title>
          <Text c="dimmed">{item.product.category}</Text>
          <Text size="sm">{instanceCount} {item.product.unitOfMeasure}</Text>
          <Group gap="xs">
            {nearExpiryCount > 0 && (
              <Badge color="yellow">{nearExpiryCount} near expiry</Badge>
            )}
            {expiredCount > 0 && <Badge color="red">{expiredCount} expired</Badge>}
          </Group>
        </Stack>
        <Button
          variant={selected ? 'filled' : 'light'}
          aria-expanded={selected}
          aria-controls={controlsId}
          onClick={onSelect}
        >
          {selected ? 'Hide instances' : 'View instances'}
        </Button>
      </Group>
    </Card>
  );
});

ItemRow.displayName = 'ItemRow';
