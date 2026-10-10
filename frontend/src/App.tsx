import { useCallback, useEffect, useState } from 'react';
import { BrowserRouter, NavLink, Navigate, Route, Routes, useLocation } from 'react-router-dom';
import { AppShell, Group, Title } from '@mantine/core';
import { ScanAlertHost } from './components/queue/ScanAlertHost';
import { ScanQueuePage } from './components/queue/ScanQueuePage';
import { InventoryPage } from './components/inventory/InventoryPage';
import { ShoppingListPage } from './components/shopping/ShoppingListPage';
import { DiagnosticsPage } from './components/diagnostics/DiagnosticsPage';
import { SettingsPage } from './components/settings/SettingsPage';
import { GroupDetailPage } from './components/groups/GroupDetailPage';
import { HistoryPage } from './components/history/HistoryPage';
import { AppMenu } from './components/shell/AppMenu';
import { HouseholdGate } from './components/auth/HouseholdGate';
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

function SignedInApp() {
  const [credentialsRevision, setCredentialsRevision] = useState(0);
  const notifyCredentialsChanged = useCallback(() => {
    setCredentialsRevision((current) => current + 1);
  }, []);

  return (
    <CredentialsRevisionContext.Provider value={credentialsRevision}>
      <RouteTelemetry />
      <ScanAlertHost />
      <AppShell
          className="app-shell"
          header={{ height: 56 }}
          padding={{ base: 'xs', sm: 'md' }}
        >
          <AppShell.Header>
            <div className="app-header">
              <Title order={3} size="h4" className="app-header-brand"><img className="app-header-mark" src="/favicon.svg" alt="" width={28} height={28} />{"Pantry"}</Title>
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
            <div className="page-frame">
              <Routes>
                <Route path="/" element={<ScanQueuePage />} />
                <Route path="/inventory" element={<InventoryPage />} />
                <Route path="/inventory/:itemId/history" element={<HistoryPage />} />
                <Route path="/shopping" element={<ShoppingListPage />} />
                <Route path="/diagnostics" element={<DiagnosticsPage />} />
                <Route path="/settings" element={<SettingsPage />} />
                <Route path="/groups" element={<Navigate to="/inventory?filter=groups" replace />} />
                <Route path="/groups/suggestions" element={<Navigate to="/inventory?review=1" replace />} />
                <Route path="/groups/:id" element={<GroupDetailPage />} />
                <Route path="/groups/:groupId/history" element={<HistoryPage />} />
              </Routes>
            </div>
          </AppShell.Main>
        </AppShell>
    </CredentialsRevisionContext.Provider>
  );
}

function App() {
  return (
    <BrowserRouter>
      <HouseholdGate>
        <SignedInApp />
      </HouseholdGate>
    </BrowserRouter>
  );
}

export default App
