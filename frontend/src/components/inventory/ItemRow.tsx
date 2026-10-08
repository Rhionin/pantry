import { memo, useRef, type ReactNode } from 'react';
import { Avatar, Badge, Button, Card, Checkbox, Divider, Group, Stack, Text, Title } from '@mantine/core';
import type { InventoryItem } from '../../types';
import { ProvenanceBadge } from '../product/ProvenanceBadge';
import { visibleCategory } from './inventoryUtils';

export interface ItemRowProps {
  inventoryItem: InventoryItem;
  selected: boolean;
  controlsId: string;
  onSelect: () => void;
  selecting?: boolean;
  checked?: boolean;
  onChecked?: (checked: boolean) => void;
  onLongPress?: () => void;
  children?: ReactNode;
}

export const ItemRow = memo(({
  inventoryItem,
  selected,
  controlsId,
  onSelect,
  selecting = false,
  checked = false,
  onChecked,
  onLongPress,
  children,
}: ItemRowProps) => {
  const { item, instanceCount, nearExpiryCount, expiredCount } = inventoryItem;
  const category = visibleCategory(item.product.category);
  const hold = useRef<number | null>(null);

  const startHold = () => {
    if (!onLongPress) return;
    hold.current = window.setTimeout(() => onLongPress(), 500);
  };
  const clearHold = () => {
    if (hold.current !== null) {
      window.clearTimeout(hold.current);
      hold.current = null;
    }
  };

  return (
    <Card
      component="article"
      withBorder
      padding="sm"
      onPointerDown={startHold}
      onPointerUp={clearHold}
      onPointerLeave={clearHold}
      onPointerCancel={clearHold}
    >
      <Stack gap="xs">
        <Group gap="xs" wrap="nowrap" align="flex-start">
          {selecting && (
            <Checkbox
              aria-label={`Select ${item.product.name}`}
              checked={checked}
              onChange={(event) => onChecked?.(event.currentTarget.checked)}
              mt={4}
            />
          )}
          <Avatar src={item.product.imageUrl} name={item.product.name} radius="sm" size="lg" />
          <Stack gap={2} style={{ flex: 1, minWidth: 0 }}>
            <Title order={3} size="h5">{item.product.name}</Title>
            {category !== null && <Text size="sm" c="dimmed">{category}</Text>}
            <ProvenanceBadge quiet externalSource={item.product.externalSource} />
            <Group justify="space-between" align="center" gap="xs" wrap="wrap">
              <Group gap={6} wrap="wrap">
                <Text size="sm">{instanceCount} {item.product.unitOfMeasure}</Text>
                {nearExpiryCount > 0 && (
                  <Badge size="sm" color="yellow">{nearExpiryCount} near expiry</Badge>
                )}
                {expiredCount > 0 && <Badge size="sm" color="red">{expiredCount} expired</Badge>}
              </Group>
              <Button
                variant="subtle"
                color="gray"
                size="compact-sm"
                px={4}
                style={{ flex: '0 0 auto' }}
                aria-expanded={selected}
                aria-controls={controlsId}
                onClick={onSelect}
              >
                {selected ? 'Hide instances' : 'View instances'}
                <span aria-hidden="true">{selected ? ' ▴' : ' ▾'}</span>
              </Button>
            </Group>
          </Stack>
        </Group>
        {selected && children != null && (
          <>
            <Divider />
            {children}
          </>
        )}
      </Stack>
    </Card>
  );
});

ItemRow.displayName = 'ItemRow';
