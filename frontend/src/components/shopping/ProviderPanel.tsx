import { useState } from 'react';
import { Badge, Button, Group, Stack, Text } from '@mantine/core';
import {
  authorizeProvider,
  disconnectProvider,
  resetProviderLedger,
} from '../../api/client';
import type { ProviderInfo } from '../../types';

export interface ProviderPanelProps {
  providers: ProviderInfo[];
  onChanged: () => void;
}

const stateLabel: Record<ProviderInfo['connectionState'], string> = {
  connected: 'Connected',
  disconnected: 'Disconnected',
  reauth_required: 'Reconnect required',
  not_required: 'No connection needed',
};

export const ProviderPanel = ({ providers, onChanged }: ProviderPanelProps) => {
  const [pending, setPending] = useState('');
  const [error, setError] = useState('');

  if (providers.length === 0) {
    return null;
  }

  const connect = async (provider: ProviderInfo) => {
    setPending(provider.id);
    setError('');
    try {
      const { authorizationUrl } = await authorizeProvider(provider.id);
      window.location.assign(authorizationUrl);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to start the connection.');
      setPending('');
    }
  };

  const disconnect = async (provider: ProviderInfo) => {
    setPending(provider.id);
    setError('');
    try {
      await disconnectProvider(provider.id);
      onChanged();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to disconnect.');
    } finally {
      setPending('');
    }
  };

  const resetLedger = async (provider: ProviderInfo) => {
    setPending(`${provider.id}-ledger`);
    setError('');
    try {
      await resetProviderLedger(provider.id);
      onChanged();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to start a new cart.');
    } finally {
      setPending('');
    }
  };

  return (
    <Stack gap="xs" aria-label="Grocery providers">
      {providers.map((provider) => (
        <ProviderRow
          key={provider.id}
          provider={provider}
          pending={pending}
          onConnect={() => void connect(provider)}
          onDisconnect={() => void disconnect(provider)}
          onResetLedger={() => void resetLedger(provider)}
        />
      ))}
      {error !== '' && <Text c="red" size="sm">{error}</Text>}
    </Stack>
  );
};

const ProviderRow = ({
  provider,
  pending,
  onConnect,
  onDisconnect,
  onResetLedger,
}: {
  provider: ProviderInfo;
  pending: string;
  onConnect: () => void;
  onDisconnect: () => void;
  onResetLedger: () => void;
}) => {
  const busy = pending === provider.id || pending === `${provider.id}-ledger`;

  return (
    <Stack gap="xs">
      <Group justify="space-between" wrap="wrap">
        <Group gap="xs">
          <Text fw={600}>{provider.displayName}</Text>
          {!provider.credentialsConfigured && <Badge color="gray">unconfigured</Badge>}
          {provider.credentialsConfigured && provider.connectionState !== 'not_required' && (
            <Badge variant="light">{stateLabel[provider.connectionState]}</Badge>
          )}
        </Group>
        <Group gap="xs">
          {provider.credentialsConfigured && provider.connectionState === 'disconnected' && (
            <Button size="xs" loading={busy} onClick={onConnect}>
              Connect
            </Button>
          )}
          {provider.credentialsConfigured && provider.connectionState === 'reauth_required' && (
            <Button size="xs" loading={busy} onClick={onConnect}>
              Reconnect
            </Button>
          )}
          {provider.credentialsConfigured && provider.connectionState === 'connected' && (
            <Button size="xs" variant="default" loading={busy} onClick={onDisconnect}>
              Disconnect
            </Button>
          )}
          {provider.credentialsConfigured && (provider.connectionState === 'connected' || provider.connectionState === 'not_required') && (
            <Button
              size="xs"
              variant="light"
              loading={pending === `${provider.id}-ledger`}
              onClick={onResetLedger}
            >
              Start a new cart
            </Button>
          )}
        </Group>
        {provider.connectionState === 'reauth_required' && (
          <Text size="sm" c="dimmed">The connection expired. Reconnect to keep provisioning.</Text>
        )}
      </Group>
    </Stack>
  );
};
