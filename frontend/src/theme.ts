import { createTheme, Popover, Switch, type MantineColorsTuple } from '@mantine/core';

// Shade 6 is the filled brand teal from the locked mark (#0D9488).
const teal: MantineColorsTuple = [
  '#f0fdfa',
  '#ccfbf1',
  '#99f6e4',
  '#5eead4',
  '#2dd4bf',
  '#14b8a6',
  '#0d9488',
  '#0f766e',
  '#115e59',
  '#134e4a',
];

// Shade 6 is the filled Low amber (#F59E0B). Darker steps are for ink on light paper.
const amber: MantineColorsTuple = [
  '#fffbeb',
  '#fef3c7',
  '#fde68a',
  '#fcd34d',
  '#fbbf24',
  '#f7b027',
  '#f59e0b',
  '#d97706',
  '#b45309',
  '#92400e',
];

// Mantine 9 changes defaults that would show up in the UI. These settings
// keep the 7.x look and behavior called out in the 7→8 and 8→9 guides:
// 4px radius, transparent light colors, the old switch thumb, and popovers
// that stay open when their target leaves the DOM.
export const theme = createTheme({
  primaryColor: 'teal',
  primaryShade: 6,
  autoContrast: true,
  luminanceThreshold: 0.22,
  colors: {
    teal,
    amber,
  },
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
});
