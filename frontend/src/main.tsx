import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { createTheme, MantineProvider, Popover, Switch, v8CssVariablesResolver } from '@mantine/core'
import { Notifications } from '@mantine/notifications'
import '@mantine/core/styles.css'
import '@mantine/notifications/styles.css'
import './index.css'
import App from './App.tsx'
import { installBrowserTelemetry } from './telemetry/client'

installBrowserTelemetry()

// Mantine 9 changes defaults that would show up in the UI. These settings
// keep the 7.x look and behavior called out in the 7→8 and 8→9 guides:
// 4px radius, transparent light colors, the old switch thumb, and popovers
// that stay open when their target leaves the DOM.
const theme = createTheme({
  defaultRadius: 'sm',
  components: {
    Switch: Switch.extend({
      defaultProps: {
        withThumbIndicator: false,
      },
    }),
    Popover: Popover.extend({
      defaultProps: {
        hideDetached: false,
      },
    }),
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <MantineProvider theme={theme} cssVariablesResolver={v8CssVariablesResolver} defaultColorScheme="auto">
      <Notifications pauseResetOnHover="notification" />
      <App />
    </MantineProvider>
  </StrictMode>,
)
