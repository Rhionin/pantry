import { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Badge, Button, Group, Loader, Paper, Stack, Text, Title } from '@mantine/core';
import { listItemInstances } from '../../api/client';
import type { ExpiryStatus, ItemInstanceWithStatus } from '../../types';
import { computeExpiryStatus } from '../../utils/expiry';
import { AddInstanceModal } from './AddInstanceModal';

export interface ItemInstanceListProps {
  itemId: string;
  productName: string;
  onInventoryChanged: () => void;
}

const sortUseOldestFirst = (
  instances: ItemInstanceWithStatus[],
): ItemInstanceWithStatus[] =>
  [...instances].sort((left, right) => {
    if (left.expiresAt === null) return right.expiresAt === null ? 0 : 1;
    if (right.expiresAt === null) return -1;
    return new Date(left.expiresAt).getTime() - new Date(right.expiresAt).getTime();
  });

const formatDate = (value: string): string =>
  new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeZone: 'UTC' }).format(new Date(value));

const expiryBadge = (status: ExpiryStatus) => {
  if (status === 'expired') return <Badge color="red">Expired</Badge>;
  if (status === 'near_expiry') return <Badge color="yellow">Near expiry</Badge>;
  return null;
};

export const ItemInstanceList = ({
  itemId,
  productName,
  onInventoryChanged,
}: ItemInstanceListProps) => {
  const [instances, setInstances] = useState<ItemInstanceWithStatus[]>([]);
  const [evaluatedAt, setEvaluatedAt] = useState(() => new Date());
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [addModalOpened, setAddModalOpened] = useState(false);

  const loadInstances = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const result = await listItemInstances(itemId);
      setInstances(sortUseOldestFirst(result));
      setEvaluatedAt(new Date());
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to load instances.');
    } finally {
      setLoading(false);
    }
  }, [itemId]);

  useEffect(() => {
    void Promise.resolve().then(loadInstances);
  }, [loadInstances]);

  const instanceStatuses = useMemo(
    () => new Map(instances.map((instance) => [
      instance.id,
      computeExpiryStatus(instance.expiresAt, evaluatedAt),
    ])),
    [evaluatedAt, instances],
  );

  return (
    <Stack id={`inventory-item-${itemId}`} component="section" aria-labelledby={`instances-${itemId}`}>
      <Group justify="space-between">
        <Title id={`instances-${itemId}`} order={3}>{productName} instances</Title>
        <Button onClick={() => setAddModalOpened(true)}>Add instance</Button>
      </Group>
      {loading && <Loader aria-label="Loading item instances" />}
      {error !== '' && <Alert color="red">{error}</Alert>}
      {!loading && error === '' && instances.length === 0 && (
        <Text c="dimmed">No item instances.</Text>
      )}
      {!loading && error === '' && instances.length > 0 && (
        <Stack component="ol" gap="sm">
          {instances.map((instance) => (
            <Paper component="li" key={instance.id} withBorder p="sm">
              <Group justify="space-between" align="flex-start">
                <Stack gap={2}>
                  <Text>Stocked in {formatDate(instance.stockInAt)}</Text>
                  <Text>
                    {instance.expiresAt === null
                      ? 'No expiration date'
                      : `Expires ${formatDate(instance.expiresAt)}`}
                  </Text>
                </Stack>
                {expiryBadge(instanceStatuses.get(instance.id) ?? 'ok')}
              </Group>
            </Paper>
          ))}
        </Stack>
      )}
      <AddInstanceModal
        itemId={itemId}
        opened={addModalOpened}
        onClose={() => setAddModalOpened(false)}
        onAdded={() => {
          void loadInstances();
          onInventoryChanged();
        }}
      />
    </Stack>
  );
};
