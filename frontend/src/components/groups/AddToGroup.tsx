import { useMemo, useState } from 'react';
import { Button, Checkbox, Group, Stack, Text, TextInput } from '@mantine/core';
import { addGroupMembers, ApiError } from '../../api/client';
import type { GroupTarget, TargetConflictMember } from '../../types';
import type { AddCandidate } from './candidates';
import { conflictLine } from './copy';

export interface AddToGroupProps {
  groupId: string;
  candidates: AddCandidate[];
  onAdded: () => void;
}

const candidateLabel = (candidate: AddCandidate) =>
  candidate.onHand === undefined ? candidate.name : `${candidate.name} · ${candidate.onHand} on hand`;

export const AddToGroup = ({ groupId, candidates, onAdded }: AddToGroupProps) => {
  const [query, setQuery] = useState('');
  const [selected, setSelected] = useState<string[]>([]);
  const [conflict, setConflict] = useState<TargetConflictMember[] | null>(null);
  const [error, setError] = useState('');
  const [adding, setAdding] = useState(false);

  const shown = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (q === '') return candidates;
    return candidates.filter((candidate) => candidate.name.toLowerCase().includes(q));
  }, [candidates, query]);

  const toggle = (productId: string) => {
    setSelected((current) => (
      current.includes(productId) ? current.filter((id) => id !== productId) : [...current, productId]
    ));
  };

  const add = async (target?: GroupTarget) => {
    if (selected.length === 0 || adding) return;
    setAdding(true);
    setError('');
    try {
      await addGroupMembers(groupId, selected, '', target);
      setConflict(null);
      setSelected([]);
      setQuery('');
      onAdded();
    } catch (requestError) {
      if (requestError instanceof ApiError && requestError.code === 'target_decision_required') {
        setConflict(requestError.members);
        return;
      }
      setError(requestError instanceof Error ? requestError.message : 'Unable to add those products.');
    } finally {
      setAdding(false);
    }
  };

  return (
    <Stack gap="xs">
      <Text fw={600}>Add to this group</Text>
      {candidates.length === 0 ? (
        <Text size="sm" c="dimmed">Every product already belongs to a group.</Text>
      ) : (
        <>
          <TextInput
            label="Search products"
            placeholder="Products not in a group"
            value={query}
            onChange={(event) => setQuery(event.currentTarget.value)}
          />
          <Stack gap={6} mah={240} style={{ overflowY: 'auto' }} role="group" aria-label="Products not in a group">
            {shown.length === 0 && <Text size="sm" c="dimmed">No matching products.</Text>}
            {shown.map((candidate) => (
              <Checkbox
                key={candidate.productId}
                label={candidateLabel(candidate)}
                checked={selected.includes(candidate.productId)}
                onChange={() => toggle(candidate.productId)}
              />
            ))}
          </Stack>
          {conflict && (
            <Stack gap={4}>
              {conflict.map((member) => <Text key={member.productId} size="sm">{conflictLine(member)}</Text>)}
              <Group gap="xs">
                <Button size="xs" variant="light" onClick={() => void add({ clear: true })}>Keep the account window</Button>
                <Button size="xs" variant="light" onClick={() => void add({ quantity: 24, dimension: 'mass' })}>Keep 24 ounces</Button>
                <Button size="xs" variant="light" onClick={() => void add({ windowMonths: 3 })}>Keep 3 months</Button>
              </Group>
            </Stack>
          )}
          <Group gap="xs" align="center">
            <Button size="xs" loading={adding} disabled={selected.length === 0} onClick={() => void add()}>
              Add to this group
            </Button>
            {selected.length > 0 && <Text size="sm">{selected.length} selected</Text>}
          </Group>
        </>
      )}
      {error !== '' && <Text size="sm" c="red">{error}</Text>}
    </Stack>
  );
};
