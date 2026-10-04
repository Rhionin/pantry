import { useCallback, useEffect, useState } from 'react';
import { Burger, Menu } from '@mantine/core';
import { useNavigate } from 'react-router-dom';
import { disconnectProvider, listProviders } from '../../api/client';
import type { ProviderInfo } from '../../types';
import { BuildStamp } from '../build/BuildStamp';
import { CredentialsDialog } from '../shopping/CredentialsDialog';

export interface AppMenuProps {
  onCredentialsChanged: () => void;
}

const canEditCredentials = (provider: ProviderInfo) =>
  provider.capabilities.auth === 'oauth2_authorization_code';

const canDisconnect = (provider: ProviderInfo) =>
  canEditCredentials(provider)
  && provider.credentialsConfigured
  && provider.connectionState === 'connected';

export const AppMenu = ({ onCredentialsChanged }: AppMenuProps) => {
  const navigate = useNavigate();
  const [opened, setOpened] = useState(false);
  const [providers, setProviders] = useState<ProviderInfo[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [loadError, setLoadError] = useState('');
  const [disconnectError, setDisconnectError] = useState('');
  const [disconnectingId, setDisconnectingId] = useState<string | null>(null);
  const [editingId, setEditingId] = useState<string | null>(null);

  const reload = useCallback(async () => {
    try {
      const rows = await listProviders();
      setProviders(rows);
      setLoadError('');
    } catch (requestError) {
      setLoadError(requestError instanceof Error ? requestError.message : 'Unable to load store settings.');
    } finally {
      setLoaded(true);
    }
  }, []);

  useEffect(() => {
    void Promise.resolve().then(reload);
  }, [reload]);

  const disconnect = async (provider: ProviderInfo) => {
    setDisconnectingId(provider.id);
    setDisconnectError('');
    try {
      await disconnectProvider(provider.id);
      await reload();
      onCredentialsChanged();
      setEditingId(null);
      setOpened(false);
    } catch (requestError) {
      setDisconnectError(requestError instanceof Error ? requestError.message : 'Unable to disconnect.');
    } finally {
      setDisconnectingId(null);
    }
  };

  const editable = providers.filter(canEditCredentials);
  const editing = editable.find((provider) => provider.id === editingId) ?? null;

  return (
    <>
      <Menu
        opened={opened}
        onChange={(next) => {
          setOpened(next);
          if (next) {
            void reload();
          }
        }}
        position="bottom-end"
        width={280}
        shadow="md"
        withinPortal
      >
        <Menu.Target>
          <Burger className="app-menu-button" opened={opened} size="sm" aria-label="Menu" />
        </Menu.Target>
        <Menu.Dropdown>
          {editable.map((provider) => (
            <Menu.Item
              key={provider.id}
              onClick={() => {
                setDisconnectError('');
                setEditingId(provider.id);
              }}
            >
              {`Manage ${provider.displayName} connection`}
            </Menu.Item>
          ))}
          {!loaded && editable.length === 0 && (
            <Menu.Item disabled>Loading store settings</Menu.Item>
          )}
          {loaded && loadError !== '' && editable.length === 0 && (
            <Menu.Item disabled>{loadError}</Menu.Item>
          )}
          <Menu.Item onClick={() => navigate('/diagnostics')}>Diagnostics</Menu.Item>
          <div className="app-menu-about">
            <Menu.Divider />
            <Menu.Item onClick={() => navigate('/settings')}>Settings</Menu.Item>
            <Menu.Label className="app-menu-about-label">Build</Menu.Label>
            <div className="app-menu-build">
              <BuildStamp />
            </div>
          </div>
        </Menu.Dropdown>
      </Menu>
      <CredentialsDialog
        provider={editing}
        opened={editing !== null}
        onClose={() => {
          setEditingId(null);
          setDisconnectError('');
        }}
        onChanged={() => {
          void reload().then(() => onCredentialsChanged());
        }}
        onDisconnect={editing !== null && canDisconnect(editing)
          ? () => {
              if (editing !== null) void disconnect(editing);
            }
          : undefined}
        disconnecting={editing !== null && disconnectingId === editing.id}
        disconnectError={disconnectError}
      />
    </>
  );
};
