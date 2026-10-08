// Presents the shared scanner direction. The parent owns the value and the
// write (POST /api/scanner/mode); this control does not keep its own mode.
import { SegmentedControl } from '@mantine/core';
import type { ScanDirection } from '../../types';

const OPTIONS: { label: string; value: ScanDirection }[] = [
  { label: 'STOCK IN', value: 'stock_in' },
  { label: 'STOCK OUT', value: 'stock_out' },
];

export interface ScannerModeSwitchProps {
  mode: ScanDirection;
  onChange: (mode: ScanDirection) => void;
  label: string;
  className?: string;
  size?: 'sm' | 'md' | 'lg';
}

export function ScannerModeSwitch({
  mode,
  onChange,
  label,
  className,
  size = 'md',
}: ScannerModeSwitchProps) {
  return (
    <SegmentedControl
      className={className}
      aria-label={label}
      fullWidth
      size={size}
      radius="xl"
      color={mode === 'stock_in' ? 'blue' : 'orange'}
      value={mode}
      onChange={(value) => {
        if (value === 'stock_in' || value === 'stock_out') onChange(value);
      }}
      data={OPTIONS}
    />
  );
}
