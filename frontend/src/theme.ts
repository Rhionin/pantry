import {
  createTheme,
  Popover,
  Switch,
  v8CssVariablesResolver,
  type CSSVariablesResolver,
  type MantineColorsTuple,
  type MantineTheme,
} from '@mantine/core';
import { flatten, readableLightShade, readableShade } from './contrast';

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

const primaryShadeIndex = (theme: MantineTheme, scheme: 'light' | 'dark') =>
  typeof theme.primaryShade === 'number' ? theme.primaryShade : theme.primaryShade[scheme];

const colorScale = (value: MantineTheme['colors'][string]): readonly string[] | null =>
  Array.isArray(value) && value.length >= 10 && typeof value[0] === 'string' ? value : null;

// v8 keeps the transparent light variant: a 10% wash of the filled shade, with
// that same shade as the ink. On white that ink is about 3:1. Step along the
// existing scale until the ink clears WCAG AA. Filled shade 6 stays the brand.
export const cssVariablesResolver: CSSVariablesResolver = (theme) => {
  const resolved = v8CssVariablesResolver(theme);
  const lightShade = primaryShadeIndex(theme, 'light');
  const darkShade = primaryShadeIndex(theme, 'dark');
  const darkPaper = theme.colors.dark[7];

  for (const name of Object.keys(theme.colors)) {
    const scale = colorScale(theme.colors[name]);
    if (!scale) continue;
    const lightWash = flatten(scale[lightShade], 0.1, theme.white);
    const lightInk = readableShade(scale, lightWash, lightShade);
    resolved.light[`--mantine-color-${name}-light-color`] = `var(--mantine-color-${name}-${lightInk})`;

    const darkWash = flatten(scale[Math.max(0, darkShade - 2)], 0.15, darkPaper);
    const darkInk = readableLightShade(scale, darkWash, Math.max(darkShade - 5, 0));
    resolved.dark[`--mantine-color-${name}-light-color`] = `var(--mantine-color-${name}-${darkInk})`;
  }

  const gray = colorScale(theme.colors.gray);
  const dark = colorScale(theme.colors.dark);
  if (gray) {
    resolved.light['--mantine-color-dimmed'] = `var(--mantine-color-gray-${readableShade(gray, theme.white, 6)})`;
  }
  if (dark) {
    resolved.dark['--mantine-color-dimmed'] = `var(--mantine-color-dark-${readableLightShade(dark, darkPaper, 2)})`;
  }

  return resolved;
};

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
