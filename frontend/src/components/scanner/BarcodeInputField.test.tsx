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

  // Contract guard (FEAT-001): the field must fire onScan exactly once, with
  // the fully assembled string, when the Enter terminator arrives -- never once
  // per character. Note this does NOT reproduce the reported "one card per
  // character" symptom: this controlled input reads its value from React state
  // and only acts on Enter, so it would pass whether or not any per-character
  // defect existed. The reported symptom originated on the POST/classification
  // path (covered in ScanQueuePage.test.tsx), not in this component. This test
  // exists to lock the fire-once-on-Enter contract against regressions.
  it('fires onScan once with the assembled string on Enter, not once per keystroke', () => {
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
