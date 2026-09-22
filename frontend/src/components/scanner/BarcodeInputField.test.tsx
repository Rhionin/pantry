import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { BarcodeInputField } from './BarcodeInputField';

function renderField(onScan: (barcode: string) => void) {
  return render(
    <MantineProvider>
      <BarcodeInputField onScan={onScan} />
    </MantineProvider>,
  );
}

describe('BarcodeInputField', () => {
  it('calls onScan with the typed barcode when Enter is pressed', () => {
    const onScan = vi.fn();
    renderField(onScan);

    const input = screen.getByLabelText(/barcode scanner input/i);
    fireEvent.change(input, { target: { value: '0123456789012' } });
    fireEvent.keyDown(input, { key: 'Enter' });

    expect(onScan).toHaveBeenCalledWith('0123456789012');
  });

  it('clears the input after a scan is submitted', () => {
    const onScan = vi.fn();
    renderField(onScan);

    const input = screen.getByLabelText(/barcode scanner input/i) as HTMLInputElement;
    fireEvent.change(input, { target: { value: '0123456789012' } });
    fireEvent.keyDown(input, { key: 'Enter' });

    expect(input.value).toBe('');
  });

  it('does not call onScan when Enter is pressed with an empty buffer', () => {
    const onScan = vi.fn();
    renderField(onScan);

    fireEvent.keyDown(screen.getByLabelText(/barcode scanner input/i), { key: 'Enter' });

    expect(onScan).not.toHaveBeenCalled();
  });

  it('renders the input visually hidden but present and accessible in the DOM', () => {
    renderField(vi.fn());

    const input = screen.getByLabelText(/barcode scanner input/i);
    expect(input).toBeInTheDocument();
    expect(input).not.toHaveStyle({ display: 'none' });
  });

  // Reproduction test B (FEAT-001): an HID scanner types a control barcode one
  // character at a time (S, T, O, C, K, _, I, N) and only then sends the Enter
  // terminator. The component contract is to buffer every keystroke and fire
  // onScan exactly once, with the fully assembled string, when the terminator
  // arrives -- never once per character. This drives the field the way a
  // controlled React input receives rapid scanner keystrokes: each keystroke
  // grows the input value before the single terminating Enter.
  it('buffers per-character scanner keystrokes and calls onScan once with the assembled control barcode', () => {
    const onScan = vi.fn();
    renderField(onScan);

    const input = screen.getByLabelText(/barcode scanner input/i) as HTMLInputElement;
    const controlBarcode = 'STOCK_IN';

    // Simulate the scanner typing each character in turn. A controlled input
    // reflects the cumulative value on every keystroke.
    let buffered = '';
    for (const char of controlBarcode) {
      buffered += char;
      fireEvent.keyDown(input, { key: char });
      fireEvent.change(input, { target: { value: buffered } });
    }

    // No terminator yet: nothing should have fired mid-scan.
    expect(onScan).not.toHaveBeenCalled();

    // The single terminating Enter keystroke ends the scan.
    fireEvent.keyDown(input, { key: 'Enter' });

    expect(onScan).toHaveBeenCalledTimes(1);
    expect(onScan).toHaveBeenCalledWith('STOCK_IN');
  });
});
