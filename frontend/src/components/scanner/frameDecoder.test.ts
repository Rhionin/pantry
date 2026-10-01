import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('./zxingFrameDecoder', () => ({
  createZxingFrameDecoder: vi.fn(() => ({ detect: vi.fn().mockResolvedValue(['from-zxing']) })),
}));

import { createZxingFrameDecoder } from './zxingFrameDecoder';
import { createFrameDecoder } from './frameDecoder';

const createZxing = vi.mocked(createZxingFrameDecoder);

describe('createFrameDecoder', () => {
  const originalBarcodeDetector = window.BarcodeDetector;

  afterEach(() => {
    if (originalBarcodeDetector === undefined) {
      delete window.BarcodeDetector;
    } else {
      window.BarcodeDetector = originalBarcodeDetector;
    }
    createZxing.mockClear();
  });

  it('uses the native detector and does not load ZXing when the browser has one', async () => {
    const detect = vi.fn().mockResolvedValue([{ rawValue: ' 0123456789012 ' }]);
    window.BarcodeDetector = vi.fn(function BarcodeDetector() {
      return { detect };
    }) as unknown as typeof window.BarcodeDetector;

    const decoder = await createFrameDecoder(['ean_13', 'upc_a']);

    expect(createZxing).not.toHaveBeenCalled();
    expect(window.BarcodeDetector).toHaveBeenCalledWith({ formats: ['ean_13', 'upc_a'] });
    await expect(decoder?.detect(document.createElement('video'))).resolves.toEqual(['0123456789012']);
  });

  it('asks the native detector only for formats it supports', async () => {
    const Detector = vi.fn(function BarcodeDetector() {
      return { detect: vi.fn().mockResolvedValue([]) };
    }) as unknown as NonNullable<typeof window.BarcodeDetector>;
    Detector.getSupportedFormats = vi.fn().mockResolvedValue(['qr_code', 'ean_13']);
    window.BarcodeDetector = Detector;

    await createFrameDecoder(['upc_a', 'ean_13', 'code_128']);

    expect(Detector).toHaveBeenCalledWith({ formats: ['ean_13'] });
    expect(createZxing).not.toHaveBeenCalled();
  });

  it('falls back to ZXing when the native detector cannot be constructed', async () => {
    window.BarcodeDetector = vi.fn(function BarcodeDetector() {
      throw new Error('unsupported format');
    }) as unknown as typeof window.BarcodeDetector;

    const decoder = await createFrameDecoder(['ean_13']);

    expect(createZxing).toHaveBeenCalledWith(['ean_13']);
    await expect(decoder?.detect(document.createElement('video'))).resolves.toEqual(['from-zxing']);
  });

  it('uses ZXing when the browser has no BarcodeDetector', async () => {
    delete window.BarcodeDetector;

    const decoder = await createFrameDecoder(['ean_13']);

    expect(createZxing).toHaveBeenCalledWith(['ean_13']);
    expect(decoder).not.toBeNull();
  });
});
