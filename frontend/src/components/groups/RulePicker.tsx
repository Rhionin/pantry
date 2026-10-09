import { useState } from 'react';
import { Button, Drawer, Radio, Select, Stack, Text } from '@mantine/core';
import { setGroupRule } from '../../api/client';
import { ruleChoices } from './copy';

export interface RulePickerGroup {
  id: string;
  rule: string;
  pinnedProductId?: string;
  members: { productId: string; name: string }[];
}

export const RulePicker = ({
  group,
  opened,
  onClose,
  onSaved,
}: {
  group: RulePickerGroup;
  opened: boolean;
  onClose: () => void;
  onSaved: () => void;
}) => {
  const [rule, setRule] = useState(group.rule);
  const [pin, setPin] = useState(group.pinnedProductId ?? '');
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const memberOptions = group.members.map((member) => ({ value: member.productId, label: member.name }));
  const productLabel = rule === 'favorite' ? 'Product' : 'If nothing is on sale';

  const save = async () => {
    setSaving(true);
    setError('');
    try {
      const pinnedProductId = rule === 'same_as_ran_out' ? '' : pin;
      await setGroupRule(group.id, rule, pinnedProductId, true);
      onSaved();
      onClose();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to save the rule.');
    } finally {
      setSaving(false);
    }
  };

  return (
    <Drawer opened={opened} onClose={onClose} position="bottom" title="When it runs out" size="auto">
      <Stack gap="md" pb="md">
        <Radio.Group value={rule} onChange={setRule}>
          <Stack gap="sm">
            {ruleChoices.map((choice) => (
              <div key={choice.id}>
                <Radio value={choice.id} label={choice.label} description={choice.hint} />
                {rule === choice.id && choice.id !== 'same_as_ran_out' && (
                  <Select
                    className="rule-choice-extra"
                    aria-label={productLabel}
                    placeholder={productLabel}
                    data={memberOptions}
                    value={pin === '' ? null : pin}
                    onChange={(value) => setPin(value ?? '')}
                    allowDeselect={choice.id === 'best_deal'}
                    comboboxProps={{ withinPortal: true }}
                  />
                )}
              </div>
            ))}
          </Stack>
        </Radio.Group>
        {error !== '' && <Text size="sm" c="red">{error}</Text>}
        <Button color="dark" onClick={() => void save()} loading={saving} disabled={rule === 'favorite' && pin === ''}>
          Save
        </Button>
      </Stack>
    </Drawer>
  );
};
