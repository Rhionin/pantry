import { useState } from 'react';
import { Button, Group, Select, Stack, Text, TextInput } from '@mantine/core';
import { addGroupMembers, ApiError, createGroup } from '../../api/client';
import type { GroupTarget, ProductGroup, TargetConflictMember } from '../../types';
import { conflictLine } from '../groups/copy';

export interface GroupSelectBarProps {
  productIds: string[];
  groups: ProductGroup[];
  onDone: () => void;
}

export const GroupSelectBar = ({ productIds, groups, onDone }: GroupSelectBarProps) => {
  const [name, setName] = useState('');
  const [existingId, setExistingId] = useState<string | null>(groups[0]?.id ?? null);
  const [conflict, setConflict] = useState<TargetConflictMember[] | null>(null);
  const [retryKind, setRetryKind] = useState<'new' | 'existing'>('new');
  const [error, setError] = useState('');

  const run = async (kind: 'new' | 'existing', target?: GroupTarget) => {
    if (kind === 'new' && name.trim() === '') return;
    if (kind === 'existing' && !existingId) return;
    setError('');
    setRetryKind(kind);
    try {
      if (kind === 'new') {
        await createGroup(name.trim(), productIds, target);
      } else if (existingId) {
        await addGroupMembers(existingId, productIds, '', target);
      }
      setConflict(null);
      onDone();
    } catch (requestError) {
      if (requestError instanceof ApiError && requestError.code === 'target_decision_required') {
        setConflict(requestError.members);
        return;
      }
      setError(requestError instanceof Error ? requestError.message : 'Unable to group these products.');
    }
  };

  return (
    <Stack className="group-select-bar page-cluster" gap="xs" p="sm">
      <Text size="sm">{productIds.length} selected</Text>
      {conflict && (
        <Stack gap={4}>
          {conflict.map((member) => <Text key={member.productId} size="sm">{conflictLine(member)}</Text>)}
          <Group gap="xs">
            <Button size="xs" variant="light" onClick={() => void run(retryKind, { clear: true })}>Use the account window</Button>
            <Button size="xs" variant="light" onClick={() => void run(retryKind, { quantity: 24, dimension: 'mass' })}>Use ounces</Button>
            <Button size="xs" variant="light" onClick={() => void run(retryKind, { windowMonths: 3 })}>Use months</Button>
          </Group>
        </Stack>
      )}
      <Group gap="xs" align="flex-end" wrap="wrap">
        <TextInput label="New group" placeholder="Cut green beans" value={name} onChange={(event) => setName(event.currentTarget.value)} />
        <Button onClick={() => void run('new')} disabled={name.trim() === '' || productIds.length === 0}>Group these</Button>
      </Group>
      {groups.length > 0 && (
        <Group gap="xs" align="flex-end" wrap="wrap">
          <Select
            label="Existing group"
            data={groups.map((group) => ({ value: group.id, label: group.name }))}
            value={existingId}
            onChange={setExistingId}
          />
          <Button variant="light" onClick={() => void run('existing')} disabled={!existingId || productIds.length === 0}>Add to group</Button>
        </Group>
      )}
      {error !== '' && <Text size="sm" c="red">{error}</Text>}
    </Stack>
  );
};
