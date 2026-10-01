import { describe, expect, it } from 'vitest';
import { createZxingFrameDecoder } from './zxingFrameDecoder';

describe('createZxingFrameDecoder', () => {
  it('returns no codes before the camera has produced a frame', async () => {
    const decoder = createZxingFrameDecoder(['ean_13', 'code_128']);
    const video = document.createElement('video');

    await expect(decoder.detect(video)).resolves.toEqual([]);
  });
});
