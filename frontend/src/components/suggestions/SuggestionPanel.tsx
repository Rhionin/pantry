import { useState } from 'react';
import { Alert, Button, Card, Group, Loader, NumberInput, Stack, Text, Title } from '@mantine/core';
import { getSuggestion, setTargetQuantity } from '../../api/client';
import type { TargetQuantitySuggestion } from '../../types';

export interface SuggestionPanelProps {
  itemId: string;
  productName: string;
  onTargetQuantitySaved?: () => void;
}

const errorMessage = (error: unknown, fallback: string) =>
  error instanceof Error ? error.message : fallback;

export const SuggestionPanel = ({
  itemId,
  productName,
  onTargetQuantitySaved,
}: SuggestionPanelProps) => {
  const [suggestion, setSuggestion] = useState<TargetQuantitySuggestion | null>(null);
  const [manualMode, setManualMode] = useState(false);
  const [manualQuantity, setManualQuantity] = useState<number | string>(0);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [confirmation, setConfirmation] = useState('');

  const requestSuggestion = async () => {
    setLoading(true);
    setError('');
    setConfirmation('');
    try {
      const result = await getSuggestion(itemId);
      setSuggestion(result);
      setManualMode(result.dataInsufficient);
    } catch (requestError) {
      setError(errorMessage(requestError, 'Unable to load a target quantity suggestion.'));
    } finally {
      setLoading(false);
    }
  };

  const saveTargetQuantity = async (quantity: number) => {
    setSaving(true);
    setError('');
    setConfirmation('');
    try {
      await setTargetQuantity(itemId, quantity);
      setConfirmation(`Target quantity set to ${quantity}.`);
      setManualMode(false);
      onTargetQuantitySaved?.();
    } catch (requestError) {
      setError(errorMessage(requestError, 'Unable to save the target quantity.'));
    } finally {
      setSaving(false);
    }
  };

  const manualTarget = typeof manualQuantity === 'number' ? manualQuantity : Number(manualQuantity);
  const manualTargetIsValid = manualQuantity !== '' && Number.isInteger(manualTarget) && manualTarget >= 0;

  return (
    <Card component="section" withBorder padding="sm" aria-labelledby={`suggestion-${itemId}`}>
      <Stack gap="xs">
        <Title id={`suggestion-${itemId}`} order={3} size="h5">Target quantity for {productName}</Title>
        {suggestion === null && !loading && (
          <Button size="xs" onClick={() => void requestSuggestion()}>Get suggestion</Button>
        )}
        {loading && <Loader aria-label="Loading target quantity suggestion" />}
        {error !== '' && (
          <Alert color="red" py="xs">
            {error}
          </Alert>
        )}
        {confirmation !== '' && (
          <Alert color="green" py="xs">
            {confirmation}
          </Alert>
        )}
        {suggestion !== null && suggestion.dataInsufficient && (
          <Alert color="yellow" title="Not enough consumption history" py="xs">
            Only {suggestion.consumptionEventCount} consumption events are recorded. Set a target manually for now.
          </Alert>
        )}
        {suggestion !== null && !suggestion.dataInsufficient && (
          <Stack gap="xs">
            <Text fw={600} size="sm">Suggested target: {suggestion.suggestedQuantity}</Text>
            <Text size="sm">{suggestion.reasoning}</Text>
            <Group gap="xs">
              <Button
                size="xs"
                loading={saving}
                onClick={() => void saveTargetQuantity(suggestion.suggestedQuantity)}
              >
                Accept
              </Button>
              <Button size="xs" variant="default" onClick={() => setManualMode(true)}>Set manually</Button>
            </Group>
          </Stack>
        )}
        {manualMode && (
          <Stack component="form" gap="xs" onSubmit={(event) => {
            event.preventDefault();
            if (manualTargetIsValid) void saveTargetQuantity(manualTarget);
          }}>
            <NumberInput
              size="xs"
              label="Manual target quantity"
              min={0}
              step={1}
              allowDecimal={false}
              value={manualQuantity}
              onChange={setManualQuantity}
              w={160}
            />
            <Button size="xs" type="submit" loading={saving} disabled={!manualTargetIsValid}>
              Save manual target
            </Button>
          </Stack>
        )}
      </Stack>
    </Card>
  );
};
