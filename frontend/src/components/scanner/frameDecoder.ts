// Native BarcodeDetector is fast where browsers ship it (Chrome, Edge, Safari).
// Everywhere else we load ZXing on demand so a phone can still scan.

export interface FrameDecoder {
  detect(source: HTMLVideoElement): Promise<string[]>;
}

export interface BarcodeDetectorResult {
  rawValue: string;
}

export interface BarcodeDetectorInstance {
  detect(source: CanvasImageSource): Promise<BarcodeDetectorResult[]>;
}

export interface BarcodeDetectorConstructor {
  new (options?: { formats?: string[] }): BarcodeDetectorInstance;
  getSupportedFormats?: () => Promise<string[]>;
}

declare global {
  interface Window {
    BarcodeDetector?: BarcodeDetectorConstructor;
  }
}

export const DEFAULT_BARCODE_FORMATS = [
  'ean_13',
  'ean_8',
  'upc_a',
  'upc_e',
  'code_128',
  'code_39',
  'itf',
  'qr_code',
];

async function formatsForNativeDetector(requested: readonly string[]): Promise<string[]> {
  const Detector = window.BarcodeDetector;
  if (Detector === undefined || typeof Detector.getSupportedFormats !== 'function') {
    return [...requested];
  }
  try {
    const available = new Set(await Detector.getSupportedFormats());
    const matched = requested.filter((format) => available.has(format));
    if (matched.length > 0) return matched;
    return [...available];
  } catch {
    return [...requested];
  }
}

async function createNativeDecoder(formats: readonly string[]): Promise<FrameDecoder | null> {
  const Detector = window.BarcodeDetector;
  if (Detector === undefined) return null;
  try {
    const supported = await formatsForNativeDetector(formats);
    const detector = new Detector({ formats: supported });
    return {
      async detect(source) {
        const results = await detector.detect(source);
        return results.map((result) => result.rawValue.trim()).filter((value) => value !== '');
      },
    };
  } catch {
    return null;
  }
}

export async function createFrameDecoder(formats: readonly string[]): Promise<FrameDecoder | null> {
  if (typeof window.BarcodeDetector !== 'undefined') {
    const native = await createNativeDecoder(formats);
    if (native) return native;
  }
  try {
    const { createZxingFrameDecoder } = await import('./zxingFrameDecoder');
    return createZxingFrameDecoder(formats);
  } catch {
    return null;
  }
}
