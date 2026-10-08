import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { Alert, Anchor, Badge, Button, Group, Loader, Paper, Stack, Text, TextInput, Title } from '@mantine/core';
import { createGroup, listGroups, listSuggestions } from '../../api/client';
import type { GroupSuggestion, ProductGroup } from '../../types';
import {
  matchesChip, matchesSearch, onHandCount, ruleLabel, suggestionBanner, targetLabel, type GroupChip,
} from './copy';

const chips: { id: GroupChip; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'low', label: 'Running low' },
  { id: 'unconfirmed', label: 'Pick a rule' },
  { id: 'same_as_ran_out', label: 'Same as what ran out' },
  { id: 'favorite', label: 'Always my favorite' },
  { id: 'best_deal', label: 'Best deal' },
];

export const GroupsPage = () => {
  const [groups, setGroups] = useState<ProductGroup[]>([]);
  const [suggestions, setSuggestions] = useState<GroupSuggestion[]>([]);
  const [search, setSearch] = useState('');
  const [chip, setChip] = useState<GroupChip>('all');
  const [name, setName] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    setError('');
    try {
      const [groupRows, cards] = await Promise.all([listGroups(), listSuggestions()]);
      setGroups(groupRows);
      setSuggestions(cards);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to load groups.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);

  const create = async () => {
    const trimmed = name.trim();
    if (trimmed === '') return;
    setSaving(true);
    setError('');
    try {
      await createGroup(trimmed);
      setName('');
      await load();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to create the group.');
    } finally {
      setSaving(false);
    }
  };

  const shown = groups.filter((group) => matchesSearch(group, search) && matchesChip(group, chip));
  const banner = suggestionBanner(suggestions);

  return (
    <Stack gap="sm" className="page-wide">
      <Stack gap="sm" className="page-cluster">
        <Title order={1} size="h3">Product groups</Title>
        {loading && <Loader aria-label="Loading groups" />}
        {error !== '' && <Alert color="red" py="xs">{error}</Alert>}
        {banner !== '' && (
          <Alert variant="light" color="blue" title="Suggestions">
            <Anchor component={Link} to="/groups/suggestions">{banner}</Anchor>
          </Alert>
        )}
        <TextInput
          label="Search groups"
          value={search}
          onChange={(event) => setSearch(event.currentTarget.value)}
        />
        <Group gap="xs">
          {chips.map((item) => (
            <Button
              key={item.id}
              size="xs"
              variant={chip === item.id ? 'filled' : 'light'}
              aria-pressed={chip === item.id}
              onClick={() => setChip(item.id)}
            >
              {item.label}
            </Button>
          ))}
        </Group>
        <Group align="end" gap="xs">
          <TextInput
            label="New group"
            value={name}
            onChange={(event) => setName(event.currentTarget.value)}
            style={{ flex: 1 }}
          />
          <Button size="xs" loading={saving} disabled={name.trim() === ''} onClick={() => void create()}>
            + New
          </Button>
        </Group>
        {!loading && shown.length === 0 && <Text c="dimmed">No groups yet.</Text>}
      </Stack>
      {shown.length > 0 && (
        <div className="card-grid">
          {shown.map((group) => (
            <Paper key={group.id} component={Link} to={`/groups/${group.id}`} withBorder p="sm" className="group-row">
              <Group justify="flex-start" align="flex-start" wrap="wrap" gap="sm">
                <Stack gap={2}>
                  <Text fw={600}>{group.name}</Text>
                  <Text size="sm">{onHandCount(group.members)} on hand · {targetLabel(group)}</Text>
                </Stack>
                {group.ruleConfirmed
                  ? <Badge variant="light">{ruleLabel(group.rule)}</Badge>
                  : <Badge color="yellow">Pick a rule</Badge>}
              </Group>
            </Paper>
          ))}
        </div>
      )}
    </Stack>
  );
};
