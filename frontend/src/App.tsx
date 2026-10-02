import { useEffect } from 'react';
import { BrowserRouter, Link, NavLink, Route, Routes, useLocation } from 'react-router-dom';
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
      <AppShell
        className="app-shell"
        header={{ height: { base: 48, sm: 56 } }}
        padding={{ base: 'xs', sm: 'md' }}
      >
        <AppShell.Header>
          <Group h="100%" px="sm" justify="space-between" wrap="nowrap" gap="xs">
            <Title order={3} size="h4">Pantry</Title>
            <Group component="nav" aria-label="Sections" gap="sm" wrap="nowrap" className="app-nav">
              <NavLink to="/" end>Scan Queue</NavLink>
              <NavLink to="/inventory">Inventory</NavLink>
              <NavLink to="/shopping">Shopping List</NavLink>
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
