import { BrowserRouter, Link, Route, Routes } from 'react-router-dom'
import { AppShell, Group, Title } from '@mantine/core'
import './App.css'

// Top-level routing shell. Route elements are placeholders — the real page
// components (ScanQueuePage, InventoryPage, ShoppingListPage) are built in
// tasks 11.1, 12.1, and 13.2.
function App() {
  return (
    <BrowserRouter>
      <AppShell header={{ height: 60 }} padding="md">
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
            <Route path="/" element={<div>Scan Queue</div>} />
            <Route path="/inventory" element={<div>Inventory</div>} />
            <Route path="/shopping" element={<div>Shopping List</div>} />
          </Routes>
        </AppShell.Main>
      </AppShell>
    </BrowserRouter>
  )
}

export default App
