import { useCallback, useEffect, useState } from 'react';
import { BrowserRouter, NavLink, Route, Routes, useLocation } from 'react-router-dom';
import { AppShell, Group, Title } from '@mantine/core';
import { ScanQueuePage } from './components/queue/ScanQueuePage';
import { InventoryPage } from './components/inventory/InventoryPage';
import { ShoppingListPage } from './components/shopping/ShoppingListPage';
import { DiagnosticsPage } from './components/diagnostics/DiagnosticsPage';
import { SettingsPage } from './components/settings/SettingsPage';
import { GroupsPage } from './components/groups/GroupsPage';
import { InboxPage } from './components/groups/InboxPage';
import { GroupDetailPage } from './components/groups/GroupDetailPage';
import { AppMenu } from './components/shell/AppMenu';
import { CredentialsRevisionContext } from './credentialsRefresh';
import { reportRoute } from './telemetry/client';
import './App.css';

function RouteTelemetry() {
  const { pathname } = useLocation();
  useEffect(() => {
    reportRoute(pathname);
  }, [pathname]);
  return null;
}

function App() {
  const [credentialsRevision, setCredentialsRevision] = useState(0);
  const notifyCredentialsChanged = useCallback(() => {
    setCredentialsRevision((current) => current + 1);
  }, []);

  return (
    <BrowserRouter>
      <CredentialsRevisionContext.Provider value={credentialsRevision}>
        <RouteTelemetry />
        <AppShell
          className="app-shell"
          header={{ height: 56 }}
          padding={{ base: 'xs', sm: 'md' }}
        >
          <AppShell.Header>
            <div className="app-header">
              <Title order={3} size="h4" className="app-header-brand">Pantry</Title>
              <Group component="nav" aria-label="Sections" gap={0} wrap="nowrap" className="app-nav">
                <NavLink to="/" end className="app-nav-link">Scan Queue</NavLink>
                <NavLink to="/inventory" className="app-nav-link">Inventory</NavLink>
                <NavLink to="/shopping" className="app-nav-link">Shopping List</NavLink>
              </Group>
              <div className="app-header-menu">
                <AppMenu onCredentialsChanged={notifyCredentialsChanged} />
              </div>
            </div>
          </AppShell.Header>
          <AppShell.Main>
            <Routes>
              <Route path="/" element={<ScanQueuePage />} />
              <Route path="/inventory" element={<InventoryPage />} />
              <Route path="/shopping" element={<ShoppingListPage />} />
              <Route path="/diagnostics" element={<DiagnosticsPage />} />
              <Route path="/settings" element={<SettingsPage />} />
              <Route path="/groups" element={<GroupsPage />} />
              <Route path="/groups/suggestions" element={<InboxPage />} />
              <Route path="/groups/:id" element={<GroupDetailPage />} />
            </Routes>
          </AppShell.Main>
        </AppShell>
      </CredentialsRevisionContext.Provider>
    </BrowserRouter>
  )
}

export default App
