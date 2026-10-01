import { BrowserRouter, Link, Route, Routes } from 'react-router-dom';
import { AppShell, Group, Title } from '@mantine/core';
import { ScanQueuePage } from './components/queue/ScanQueuePage';
import { InventoryPage } from './components/inventory/InventoryPage';
import { ShoppingListPage } from './components/shopping/ShoppingListPage';
import { BuildStamp } from './components/build/BuildStamp';
import './App.css';

function App() {
  return (
    <BrowserRouter>
      <AppShell header={{ height: 60 }} footer={{ height: 36 }} padding="md">
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
          </Routes>
        </AppShell.Main>
        <AppShell.Footer style={{ borderTop: 'none', background: 'transparent' }}>
          <Group h="100%" px="md" justify="flex-end" align="center">
            <BuildStamp />
          </Group>
        </AppShell.Footer>
      </AppShell>
    </BrowserRouter>
  )
}

export default App
