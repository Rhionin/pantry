import { useState } from 'react';
import { Button, Group, Stack, Text } from '@mantine/core';
import { exportShoppingList, resolveUnknownProvision } from '../../api/client';
import type { ProviderInfo, ProvisionReport, ShoppingListEntry } from '../../types';
import { provisionQuantity, summarizeNames } from './outcome';

export interface ProvisionButtonProps {
  provider: ProviderInfo | null;
  entries: ShoppingListEntry[];
  useItemIds?: Record<string, string>;
  explainEmpty?: boolean;
  onFinished: () => void;
}

const storeIsReady = (provider: ProviderInfo | null) => (
  provider !== null
  && provider.credentialsConfigured
  && (provider.connectionState === 'connected' || provider.connectionState === 'not_required')
);

const canProvision = (provider: ProviderInfo | null, entries: ShoppingListEntry[], inFlight: boolean) => {
  if (inFlight) return false;
  if (!entries.some((entry) => provisionQuantity(entry) >= 1)) return false;
  // An unconfigured store still accepts the export. The server confirms
  // nothing and returns the planned lines, including a sale the shopper took.
  if (provider === null || !provider.credentialsConfigured) return true;
  return storeIsReady(provider);
};

export const ProvisionButton = ({ provider, entries, useItemIds, explainEmpty = false, onFinished }: ProvisionButtonProps) => {
  const [inFlight, setInFlight] = useState(false);
  const [report, setReport] = useState<ProvisionReport | null>(null);
  const [error, setError] = useState('');

  const staged = entries.some((entry) => provisionQuantity(entry) >= 1);
  const enabled = canProvision(provider, entries, inFlight);
  const label = 'Send to Kroger';

  const provision = async () => {
    setInFlight(true);
    setError('');
    setReport(null);
    try {
      const providerId = provider !== null && provider.credentialsConfigured ? provider.id : undefined;
      const result = await exportShoppingList(providerId, useItemIds);
      setReport(result);
      onFinished();
    } catch (requestError) {
      setReport(null);
      setError(requestError instanceof Error ? requestError.message : 'The shopping list could not be provisioned.');
    } finally {
      setInFlight(false);
    }
  };

  const resolve = async (entryId: string, reachedProvider: boolean) => {
    if (provider === null || entryId === '') return;
    setError('');
    try {
      await resolveUnknownProvision(entryId, provider.id, reachedProvider);
      onFinished();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to record that outcome.');
    }
  };

  const failed = summarizeNames(report?.failedItems ?? []);
  const unknown = summarizeNames(report?.unknownItems ?? []);
  const nothingFailed = (report?.failedItems?.length ?? 0) === 0 && (report?.unknownItems?.length ?? 0) === 0;

  return (
    <Stack gap="xs">
      <Button loading={inFlight} disabled={!enabled} onClick={() => void provision()}>
        {label}
      </Button>
      {explainEmpty && !staged && (
        <Text size="sm">Nothing is staged. Fill the cart before sending it to Kroger.</Text>
      )}
      {provider !== null && !provider.credentialsConfigured && (
        <Text size="sm" c="dimmed">{provider.displayName} is unconfigured.</Text>
      )}
      {error !== '' && <Text c="red" size="sm">{error}</Text>}
      {report !== null && error === '' && nothingFailed && report.exported === 0 && (provider === null || !provider.credentialsConfigured) && (
        <Text size="sm">Nothing was sent. Connect a store to add these items to a cart.</Text>
      )}
      {report !== null && error === '' && nothingFailed && (report.exported > 0 || (provider !== null && provider.credentialsConfigured)) && (
        <Text size="sm">{report.exported} item{report.exported === 1 ? '' : 's'} sent to your cart.</Text>
      )}
      {report !== null && failed.shown.length > 0 && (
        <Text size="sm" c="red">
          Could not add: {failed.shown.join(', ')}
          {failed.overflow > 0 ? `, and ${failed.overflow} more` : ''}. These items remain on your shopping list.
        </Text>
      )}
      {report !== null && unknown.shown.length > 0 && (
        <Stack gap={4}>
          <Text size="sm">
            Unconfirmed: {unknown.shown.join(', ')}
            {unknown.overflow > 0 ? `, and ${unknown.overflow} more` : ''}.
          </Text>
          {(report.entries ?? [])
            .filter((entry) => entry.outcome === 'unknown' && entry.entryId !== '')
            .slice(0, 50)
            .map((entry) => (
              <Group key={entry.entryId} gap="xs">
                <Text size="sm">{entry.name}</Text>
                <Button size="xs" variant="light" onClick={() => void resolve(entry.entryId, true)}>
                  It reached {provider?.displayName ?? 'the cart'}
                </Button>
                <Button size="xs" variant="default" onClick={() => void resolve(entry.entryId, false)}>
                  It did not
                </Button>
              </Group>
            ))}
        </Stack>
      )}
    </Stack>
  );
};
