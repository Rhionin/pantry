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
});
