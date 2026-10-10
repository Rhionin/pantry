import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { Anchor, Button, Loader, Modal, Radio, Stack, Text, TextInput } from '@mantine/core';
import { addGroupMembers, ApiError, listGroups } from '../../api/client';
import type { GroupTarget, ProductGroup, TargetConflictMember } from '../../types';
import { TargetConflictPrompt } from '../groups/TargetConflictPrompt';

export interface GroupMembershipActionProps {
  productId: string;
  productName: string;
  currentGroup?: { id: string; name: string } | null;
  onChanged: () => void;
}

const closeButton = { 'aria-label': 'Close' };

export const GroupMembershipAction = ({
  productId,
  productName,
  currentGroup = null,
  onChanged,
}: GroupMembershipActionProps) => {
  const moving = currentGroup !== null;
  const [opened, setOpened] = useState(false);
  const [groups, setGroups] = useState<ProductGroup[]>([]);
  const [loading, setLoading] = useState(false);
  const [query, setQuery] = useState('');
  const [chosenId, setChosenId] = useState<string | null>(null);
  const [conflict, setConflict] = useState<TargetConflictMember[] | null>(null);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!opened) return undefined;
    let active = true;
    void listGroups()
      .then((rows) => {
        if (!active) return;
        setGroups(rows);
        const available = rows.filter((group) => group.id !== currentGroup?.id);
        setChosenId((current) => (
          current !== null && available.some((group) => group.id === current) ? current : available[0]?.id ?? null
        ));
      })
      .catch((requestError: unknown) => {
        if (!active) return;
        setGroups([]);
        setChosenId(null);
        setError(requestError instanceof Error ? requestError.message : 'Unable to load groups.');
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [opened, currentGroup?.id]);

  const available = useMemo(
    () => groups.filter((group) => group.id !== currentGroup?.id),
    [groups, currentGroup?.id],
  );
  const shown = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (q === '') return available;
    return available.filter((group) => group.name.toLowerCase().includes(q));
  }, [available, query]);
  const chosen = available.find((group) => group.id === chosenId) ?? null;
  const chosenVisible = chosen !== null && shown.some((group) => group.id === chosen.id);

  const open = () => {
    setError('');
    setConflict(null);
    setQuery('');
    setLoading(true);
    setOpened(true);
  };

  const submit = async (target?: GroupTarget) => {
    if (!chosen || saving) return;
    setSaving(true);
    setError('');
    try {
      await addGroupMembers(chosen.id, [productId], currentGroup?.id ?? '', target);
      setConflict(null);
      setOpened(false);
      onChanged();
    } catch (requestError) {
      if (requestError instanceof ApiError && requestError.code === 'target_decision_required') {
        setConflict(requestError.members);
        return;
      }
      setError(requestError instanceof Error ? requestError.message : 'Unable to update that group.');
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <Button
        size="compact-sm"
        variant="light"
        style={{ flex: '0 0 auto' }}
        aria-label={moving ? `Move ${productName} to another group` : `Add ${productName} to a group`}
        onPointerDown={(event) => event.stopPropagation()}
        onClick={open}
      >
        {moving ? 'Move to' : 'Add to group'}
      </Button>
      <Modal
        opened={opened}
        onClose={() => setOpened(false)}
        title={moving ? 'Move to another group' : 'Add to group'}
        closeButtonProps={closeButton}
        transitionProps={{ duration: 0 }}
      >
        <Stack gap="sm">
          <Text size="sm" fw={600}>{productName}</Text>
          {moving && <Text size="sm">Already in {currentGroup.name}.</Text>}
          {loading && <Loader size="sm" aria-label="Loading groups" />}
          {!loading && available.length === 0 && (
            moving ? (
              <Text size="sm" c="dimmed">No other groups.</Text>
            ) : (
              <Stack gap={4}>
                <Text size="sm" c="dimmed">No groups yet.</Text>
                <Anchor component={Link} to="/groups">Product groups</Anchor>
              </Stack>
            )
          )}
          {!loading && available.length > 8 && (
            <TextInput
              label="Search groups"
              placeholder="Name"
              value={query}
              onChange={(event) => setQuery(event.currentTarget.value)}
            />
          )}
          {!loading && shown.length > 0 && (
            <Radio.Group
              label="Group"
              value={chosenVisible ? chosenId ?? '' : ''}
              onChange={(value) => {
                setChosenId(value);
                setConflict(null);
              }}
            >
              <Stack gap="sm" mah={240} style={{ overflowY: 'auto' }}>
                {shown.map((group) => (
                  <Radio key={group.id} value={group.id} label={group.name} />
                ))}
              </Stack>
            </Radio.Group>
          )}
          {!loading && available.length > 0 && shown.length === 0 && (
            <Text size="sm" c="dimmed">No matching groups.</Text>
          )}
          {conflict && chosen && (
            <TargetConflictPrompt
              group={chosen}
              members={conflict}
              onChoose={(target) => void submit(target)}
              onCancel={() => setConflict(null)}
            />
          )}
          {!loading && available.length > 0 && conflict === null && (
            <Button
              loading={saving}
              disabled={!chosenVisible}
              onClick={() => void submit()}
            >
              {moving ? 'Move to this group' : 'Add to group'}
            </Button>
          )}
          {error !== '' && <Text size="sm" c="red">{error}</Text>}
        </Stack>
      </Modal>
    </>
  );
};
