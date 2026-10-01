import {
  BarcodeFormat,
  BinaryBitmap,
  DecodeHintType,
  HybridBinarizer,
  HTMLCanvasElementLuminanceSource,
  MultiFormatReader,
} from '@zxing/library';
import type { FrameDecoder } from './frameDecoder';

// Full-resolution frames make the pure-JS decoder too slow to keep up with a
// live preview on a phone, so each frame is scaled down before decoding.
const MAX_FRAME_WIDTH = 720;

const FORMAT_BY_NAME: Record<string, BarcodeFormat> = {
  ean_13: BarcodeFormat.EAN_13,
  ean_8: BarcodeFormat.EAN_8,
  upc_a: BarcodeFormat.UPC_A,
  upc_e: BarcodeFormat.UPC_E,
  code_128: BarcodeFormat.CODE_128,
  code_39: BarcodeFormat.CODE_39,
  itf: BarcodeFormat.ITF,
  qr_code: BarcodeFormat.QR_CODE,
  codabar: BarcodeFormat.CODABAR,
  data_matrix: BarcodeFormat.DATA_MATRIX,
};

export function createZxingFrameDecoder(formats: readonly string[]): FrameDecoder {
  const reader = new MultiFormatReader();
  const hints = new Map<DecodeHintType, unknown>();
  const zxingFormats = formats
    .map((format) => FORMAT_BY_NAME[format])
    .filter((format): format is BarcodeFormat => format !== undefined);
  if (zxingFormats.length > 0) {
    hints.set(DecodeHintType.POSSIBLE_FORMATS, zxingFormats);
  }
  reader.setHints(hints);

  const canvas = document.createElement('canvas');

  return {
    async detect(source) {
      const frameWidth = source.videoWidth;
      const frameHeight = source.videoHeight;
      if (frameWidth === 0 || frameHeight === 0) return [];

      const scale = Math.min(1, MAX_FRAME_WIDTH / frameWidth);
      canvas.width = Math.max(1, Math.round(frameWidth * scale));
      canvas.height = Math.max(1, Math.round(frameHeight * scale));
      const context = canvas.getContext('2d', { willReadFrequently: true });
      if (context === null) return [];
      context.drawImage(source, 0, 0, canvas.width, canvas.height);

      try {
        const luminance = new HTMLCanvasElementLuminanceSource(canvas);
        const bitmap = new BinaryBitmap(new HybridBinarizer(luminance));
        return [reader.decodeWithState(bitmap).getText()];
      } catch {
        return [];
      } finally {
        reader.reset();
      }
    },
  };
}
