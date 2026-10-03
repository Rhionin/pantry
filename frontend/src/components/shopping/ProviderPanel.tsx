import { useState } from 'react';
import { Badge, Button, Group, Menu, Modal, NativeSelect, PasswordInput, Stack, Text, TextInput } from '@mantine/core';
import {
  authorizeProvider,
  clearProviderCredentials,
  disconnectProvider,
  resetProviderLedger,
  saveProviderCredentials,
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
          onChanged={onChanged}
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
  onChanged,
}: {
  provider: ProviderInfo;
  pending: string;
  onConnect: () => void;
  onDisconnect: () => void;
  onResetLedger: () => void;
  onChanged: () => void;
}) => {
  const [credentialsOpen, setCredentialsOpen] = useState(false);
  const busy = pending === provider.id || pending === `${provider.id}-ledger`;
  const saved = provider.credentials;
  const canEditCredentials = provider.capabilities.auth === 'oauth2_authorization_code';

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
          {canEditCredentials && (
            <Menu position="bottom-end" shadow="sm" width={220}>
              <Menu.Target>
                <Button size="xs" variant="subtle" aria-label={`${provider.displayName} setup`}>
                  Setup
                </Button>
              </Menu.Target>
              <Menu.Dropdown>
                <Menu.Item onClick={() => setCredentialsOpen(true)}>
                  Edit credentials
                </Menu.Item>
              </Menu.Dropdown>
            </Menu>
          )}
        </Group>
        {provider.connectionState === 'reauth_required' && (
          <Text size="sm" c="dimmed">The connection expired. Reconnect to keep provisioning.</Text>
        )}
      </Group>
      {canEditCredentials && (
        <Modal
          opened={credentialsOpen}
          onClose={() => setCredentialsOpen(false)}
          title={`${provider.displayName} credentials`}
          size="sm"
        >
          {credentialsOpen && (
            <CredentialForm
              key={`${provider.id}:${saved?.source ?? ''}:${saved?.clientId ?? ''}:${saved?.redirectUri ?? ''}:${saved?.modality ?? ''}:${saved?.secretSet ? '1' : '0'}`}
              provider={provider}
              onChanged={onChanged}
            />
          )}
        </Modal>
      )}
    </Stack>
  );
};

const CredentialForm = ({ provider, onChanged }: { provider: ProviderInfo; onChanged: () => void }) => {
  const saved = provider.credentials;
  const [clientId, setClientId] = useState(saved?.clientId ?? '');
  const [clientSecret, setClientSecret] = useState('');
  const [redirectUri, setRedirectUri] = useState(saved?.redirectUri ?? '');
  const [modality, setModality] = useState(saved?.modality || 'PICKUP');
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');

  const save = async () => {
    setPending(true);
    setError('');
    try {
      await saveProviderCredentials(provider.id, {
        clientId,
        clientSecret,
        redirectUri,
        modality,
      });
      setClientSecret('');
      onChanged();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to save credentials.');
    } finally {
      setPending(false);
    }
  };

  const clear = async () => {
    setPending(true);
    setError('');
    try {
      await clearProviderCredentials(provider.id);
      onChanged();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to clear credentials.');
    } finally {
      setPending(false);
    }
  };

  return (
    <Stack
      gap="xs"
      component="form"
      aria-label={`${provider.displayName} credentials`}
      onSubmit={(event) => {
        event.preventDefault();
        void save();
      }}
    >
      <TextInput
        size="xs"
        label="Client ID"
        value={clientId}
        onChange={(event) => setClientId(event.currentTarget.value)}
      />
      <PasswordInput
        size="xs"
        label="Client secret"
        value={clientSecret}
        placeholder={saved?.secretSet ? 'Leave blank to keep the saved secret' : ''}
        onChange={(event) => setClientSecret(event.currentTarget.value)}
      />
      <TextInput
        size="xs"
        label="Redirect URI"
        value={redirectUri}
        onChange={(event) => setRedirectUri(event.currentTarget.value)}
      />
      <NativeSelect
        size="xs"
        label="Modality"
        value={modality}
        data={[
          { value: 'PICKUP', label: 'Pickup' },
          { value: 'DELIVERY', label: 'Delivery' },
        ]}
        onChange={(event) => setModality(event.currentTarget.value)}
      />
      {saved?.source === 'environment' && (
        <Text size="sm" c="dimmed">These credentials come from the server environment. Saving replaces them until you clear the saved credentials.</Text>
      )}
      {saved?.secretSet && (
        <Text size="sm" c="dimmed">A client secret is saved and is not shown.</Text>
      )}
      <Group gap="xs">
        <Button size="xs" type="submit" loading={pending}>Save credentials</Button>
        {saved?.source === 'saved' && (
          <Button size="xs" type="button" variant="default" loading={pending} onClick={() => void clear()}>
            Clear saved credentials
          </Button>
        )}
      </Group>
      {error !== '' && <Text c="red" size="sm">{error}</Text>}
    </Stack>
  );
};
