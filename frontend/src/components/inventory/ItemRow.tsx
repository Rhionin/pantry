import { memo } from 'react';
import { Avatar, Badge, Button, Card, Group, Stack, Text, Title } from '@mantine/core';
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
    <Card component="article" withBorder padding="sm" h="100%">
      <Stack gap="xs" justify="space-between" h="100%">
        <Group gap="xs" wrap="nowrap" align="flex-start">
          <Avatar src={item.product.imageUrl} name={item.product.name} radius="sm" size="lg" />
          <Stack gap={2}>
            <Title order={3} size="h5">{item.product.name}</Title>
            <Text size="sm" c="dimmed">{item.product.category}</Text>
            <Text size="sm">{instanceCount} {item.product.unitOfMeasure}</Text>
          </Stack>
        </Group>
        <Group gap="xs">
          {nearExpiryCount > 0 && (
            <Badge size="sm" color="yellow">{nearExpiryCount} near expiry</Badge>
          )}
          {expiredCount > 0 && <Badge size="sm" color="red">{expiredCount} expired</Badge>}
        </Group>
        <Button
          size="xs"
          fullWidth
          variant={selected ? 'filled' : 'light'}
          aria-expanded={selected}
          aria-controls={controlsId}
          onClick={onSelect}
        >
          {selected ? 'Hide instances' : 'View instances'}
        </Button>
      </Stack>
    </Card>
  );
});

ItemRow.displayName = 'ItemRow';
