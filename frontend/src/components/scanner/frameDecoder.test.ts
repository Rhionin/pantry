import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('./zxingFrameDecoder', () => ({
  createZxingFrameDecoder: vi.fn(() => ({
    detect: vi.fn().mockResolvedValue([{ value: 'from-zxing', corners: [], frameWidth: 0, frameHeight: 0 }]),
  })),
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
    await expect(decoder?.detect(document.createElement('video'))).resolves.toEqual([
      { value: '0123456789012', corners: [], frameWidth: 0, frameHeight: 0 },
    ]);
  });

  it('keeps native corner points so the preview can outline the code', async () => {
    const detect = vi.fn().mockResolvedValue([{
      rawValue: '111',
      cornerPoints: [
        { x: 1, y: 2 },
        { x: 3, y: 2 },
        { x: 3, y: 4 },
        { x: 1, y: 4 },
      ],
    }]);
    window.BarcodeDetector = vi.fn(function BarcodeDetector() {
      return { detect };
    }) as unknown as typeof window.BarcodeDetector;
    const video = document.createElement('video');
    Object.defineProperty(video, 'videoWidth', { value: 100 });
    Object.defineProperty(video, 'videoHeight', { value: 80 });

    const decoder = await createFrameDecoder(['ean_13']);

    await expect(decoder?.detect(video)).resolves.toEqual([{
      value: '111',
      corners: [
        { x: 1, y: 2 },
        { x: 3, y: 2 },
        { x: 3, y: 4 },
        { x: 1, y: 4 },
      ],
      frameWidth: 100,
      frameHeight: 80,
    }]);
  });

  it('builds an outline from a bounding box when corners are missing', async () => {
    window.BarcodeDetector = vi.fn(function BarcodeDetector() {
      return {
        detect: vi.fn().mockResolvedValue([{
          rawValue: '111',
          boundingBox: { x: 5, y: 6, width: 10, height: 4 },
        }]),
      };
    }) as unknown as typeof window.BarcodeDetector;
    const video = document.createElement('video');
    Object.defineProperty(video, 'videoWidth', { value: 50 });
    Object.defineProperty(video, 'videoHeight', { value: 40 });

    const decoder = await createFrameDecoder(['ean_13']);

    await expect(decoder?.detect(video)).resolves.toEqual([{
      value: '111',
      corners: [
        { x: 5, y: 6 },
        { x: 15, y: 6 },
        { x: 15, y: 10 },
        { x: 5, y: 10 },
      ],
      frameWidth: 50,
      frameHeight: 40,
    }]);
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
    await expect(decoder?.detect(document.createElement('video'))).resolves.toEqual([
      { value: 'from-zxing', corners: [], frameWidth: 0, frameHeight: 0 },
    ]);
  });

  it('uses ZXing when the browser has no BarcodeDetector', async () => {
    delete window.BarcodeDetector;

    const decoder = await createFrameDecoder(['ean_13']);

    expect(createZxing).toHaveBeenCalledWith(['ean_13']);
    expect(decoder).not.toBeNull();
  });
});
