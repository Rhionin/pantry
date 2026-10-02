import { useEffect } from 'react';
import { BrowserRouter, Link, Route, Routes, useLocation } from 'react-router-dom';
import { AppShell, Box, Group, Title } from '@mantine/core';
import { ScanQueuePage } from './components/queue/ScanQueuePage';
import { InventoryPage } from './components/inventory/InventoryPage';
import { ShoppingListPage } from './components/shopping/ShoppingListPage';
import { DiagnosticsPage } from './components/diagnostics/DiagnosticsPage';
import { BuildStamp } from './components/build/BuildStamp';
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
  return (
    <BrowserRouter>
      <RouteTelemetry />
      <AppShell className="app-shell" header={{ height: 60 }} padding="md">
        <AppShell.Header>
          <Group h="100%" px="md" justify="space-between">
            <Title order={3}>Pantry</Title>
            <Group>
              <Link to="/">Scan Queue</Link>
              <Link to="/inventory">Inventory</Link>
              <Link to="/shopping">Shopping List</Link>
            </Group>
          </Group>
        </AppShell.Header>
        <AppShell.Main>
          <Routes>
            <Route path="/" element={<ScanQueuePage />} />
            <Route path="/inventory" element={<InventoryPage />} />
            <Route path="/shopping" element={<ShoppingListPage />} />
            <Route path="/diagnostics" element={<DiagnosticsPage />} />
          </Routes>
        </AppShell.Main>
        <AppShell.Footer
          className="build-footer"
          style={{
            position: 'static',
            height: 'auto',
            transform: 'none',
            borderTop: 'none',
            background: 'transparent',
          }}
        >
          <Group px="md" py={6} justify="space-between" align="center" wrap="nowrap" gap="sm">
            <Link to="/diagnostics" className="diagnostics-link">Diagnostics</Link>
            <Box style={{ flex: '1 1 auto', minWidth: 0 }}>
              <BuildStamp />
            </Box>
          </Group>
        </AppShell.Footer>
      </AppShell>
    </BrowserRouter>
  )
}

export default App
