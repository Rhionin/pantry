// Keystroke-capture input for HID barcode scanners. Scanners of this type act
// as keyboard devices: they type the barcode digits rapidly, then send a
// terminator keystroke (Enter). This component buffers keystrokes and fires
// onScan when the terminator is detected, then clears itself for the next scan.
import { useEffect, useRef, useState } from 'react';
import { VisuallyHidden } from '@mantine/core';

export interface BarcodeInputFieldProps {
  onScan: (barcode: string) => void;
}

export function BarcodeInputField({ onScan }: BarcodeInputFieldProps) {
  const [value, setValue] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

  // autoFocus scrolls the focused field into view. On a phone that field is
  // clipped, the soft keyboard opens, and the queue stops accepting pans.
  useEffect(() => {
    inputRef.current?.focus({ preventScroll: true });
  }, []);

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
      {/* Prevent password-manager autofill (1Password, LastPass) from
          stealing focus from the scanner field or injecting autofill UI. */}
      <input
        ref={inputRef}
        aria-label="Barcode scanner input"
        inputMode="none"
        data-1p-ignore
        data-lpignore="true"
        autoComplete="off"
        value={value}
        onChange={(event) => setValue(event.currentTarget.value)}
        onKeyDown={handleKeyDown}
      />
    </VisuallyHidden>
  );
}
