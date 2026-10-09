import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { Alert, Anchor, Button, Group, Loader, Modal, Stack, Text, TextInput, Title, UnstyledButton } from '@mantine/core';
import { createGroup, getSupplySettings, listGroups, listProducts, listSuggestions } from '../../api/client';
import type { GroupSuggestion, Product, ProductGroup } from '../../types';
import { BinMark } from './BinMark';
import {
  binView, listSummary, matchesChip, matchesSearch, memberOrder, memberPackage, suggestionBanner, type GroupChip,
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
  const [catalog, setCatalog] = useState<Product[]>([]);
  const [suggestions, setSuggestions] = useState<GroupSuggestion[]>([]);
  const [householdMonths, setHouseholdMonths] = useState<number | undefined>();
  const [search, setSearch] = useState('');
  const [chip, setChip] = useState<GroupChip>('all');
  const [name, setName] = useState('');
  const [creating, setCreating] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    setError('');
    try {
      const [groupRows, cards, products, settings] = await Promise.all([
        listGroups(),
        listSuggestions(),
        listProducts().catch(() => [] as Product[]),
        getSupplySettings().catch(() => null),
      ]);
      setGroups(groupRows);
      setSuggestions(cards);
      setCatalog(products);
      setHouseholdMonths(settings?.months);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to load groups.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);

  const productsById = useMemo(() => new Map(catalog.map((product) => [product.id, product])), [catalog]);

  const create = async () => {
    const trimmed = name.trim();
    if (trimmed === '') return;
    setSaving(true);
    setError('');
    try {
      await createGroup(trimmed);
      setName('');
      setCreating(false);
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
    <Stack gap="md" className="page-wide bin-page">
      <Stack gap="sm" className="page-cluster">
        <Group justify="space-between" align="center" wrap="wrap" className="bin-title-row">
          <Title order={1} size="h3">Product groups</Title>
          <Button color="dark" onClick={() => { setError(''); setCreating(true); }}>New group</Button>
        </Group>
        {loading && <Loader aria-label="Loading groups" />}
        {error !== '' && !creating && <Alert color="red" py="xs">{error}</Alert>}
        {banner !== '' && (
          <Anchor component={Link} to="/groups/suggestions" className="bin-action">{banner}</Anchor>
        )}
        <TextInput
          label="Search groups"
          placeholder="Name or product"
          value={search}
          onChange={(event) => setSearch(event.currentTarget.value)}
        />
        <div className="bin-filters">
          {chips.map((item) => (
            <UnstyledButton
              key={item.id}
              className="bin-chip"
              aria-pressed={chip === item.id}
              onClick={() => setChip(item.id)}
            >
              {item.label}
            </UnstyledButton>
          ))}
        </div>
        {!loading && groups.length === 0 && <Text c="dimmed">No groups yet.</Text>}
        {!loading && groups.length > 0 && shown.length === 0 && <Text c="dimmed">No groups match.</Text>}
      </Stack>
      {shown.length > 0 && (
        <ul className="bin-list">
          {shown.map((group) => {
            const members = memberOrder(group.members.map((member) => memberPackage(member, productsById.get(member.productId))));
            return (
              <li key={group.id}>
                <Link to={`/groups/${group.id}`} className="bin-row">
                  <BinMark view={binView(group, members)} members={members} size="mark" />
                  <span className="bin-row-copy">
                    <span className="bin-row-name">{group.name}</span>
                    <span className="bin-row-summary">{listSummary(group, members, householdMonths)}</span>
                  </span>
                </Link>
              </li>
            );
          })}
        </ul>
      )}
      <Modal
        opened={creating}
        onClose={() => setCreating(false)}
        title="New group"
        closeButtonProps={{ 'aria-label': 'Close' }}
      >
        <Stack gap="sm">
          <TextInput
            label="Name"
            value={name}
            onChange={(event) => setName(event.currentTarget.value)}
          />
          {error !== '' && <Text size="sm" c="red">{error}</Text>}
          <Button loading={saving} disabled={name.trim() === ''} onClick={() => void create()}>
            Create group
          </Button>
        </Stack>
      </Modal>
    </Stack>
  );
};
