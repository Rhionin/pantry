import { useCallback, useEffect, useState } from 'react';
import { Burger, Menu } from '@mantine/core';
import { useNavigate } from 'react-router-dom';
import { listProviders } from '../../api/client';
import type { ProviderInfo } from '../../types';
import { BuildStamp } from '../build/BuildStamp';
import { CredentialsDialog } from '../shopping/CredentialsDialog';

export interface AppMenuProps {
  onCredentialsChanged: () => void;
}

const canEditCredentials = (provider: ProviderInfo) =>
  provider.capabilities.auth === 'oauth2_authorization_code';

export const AppMenu = ({ onCredentialsChanged }: AppMenuProps) => {
  const navigate = useNavigate();
  const [opened, setOpened] = useState(false);
  const [providers, setProviders] = useState<ProviderInfo[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [loadError, setLoadError] = useState('');
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
    void reload();
  }, [reload]);

  const editable = providers.filter(canEditCredentials);
  const editing = editable.find((provider) => provider.id === editingId) ?? null;

  return (
    <>
      <Menu
        opened={opened}
        onChange={(next) => {
          setOpened(next);
          if (next) void reload();
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
            <Menu.Item key={provider.id} onClick={() => setEditingId(provider.id)}>
              {`Edit ${provider.displayName} credentials`}
            </Menu.Item>
          ))}
          {!loaded && editable.length === 0 && (
            <Menu.Item disabled>Loading store settings</Menu.Item>
          )}
          {loaded && loadError !== '' && editable.length === 0 && (
            <Menu.Item disabled>{loadError}</Menu.Item>
          )}
          <Menu.Item onClick={() => navigate('/diagnostics')}>Diagnostics</Menu.Item>
          <div className="app-menu-build">
            <BuildStamp />
          </div>
        </Menu.Dropdown>
      </Menu>
      <CredentialsDialog
        provider={editing}
        opened={editing !== null}
        onClose={() => setEditingId(null)}
        onChanged={() => {
          void reload().then(() => onCredentialsChanged());
        }}
      />
    </>
  );
};
