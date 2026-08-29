// Keystroke-capture input for HID barcode scanners. Scanners of this type act
// as keyboard devices: they type the barcode digits rapidly, then send a
// terminator keystroke (Enter). This component buffers keystrokes and fires
// onScan when the terminator is detected, then clears itself for the next scan.
import { useState } from 'react';
import { VisuallyHidden } from '@mantine/core';

export interface BarcodeInputFieldProps {
  onScan: (barcode: string) => void;
}

export function BarcodeInputField({ onScan }: BarcodeInputFieldProps) {
  const [value, setValue] = useState('');

  function handleKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
    if (event.key !== 'Enter') {
      return;
    }
    // Prevent the Enter keystroke from submitting an enclosing form.
    event.preventDefault();
    const barcode = value.trim();
    if (barcode) {
      onScan(barcode);
    }
    setValue('');
  }

  return (
    <VisuallyHidden>
      <input
        aria-label="Barcode scanner input"
        autoFocus
        value={value}
        onChange={(event) => setValue(event.currentTarget.value)}
        onKeyDown={handleKeyDown}
      />
    </VisuallyHidden>
  );
}
