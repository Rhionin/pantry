import { describe, expect, it } from 'vitest'
import { contrastRatio, flatten, readableLightShade, readableShade } from './contrast'

// Pairs are independent of the theme resolver. #0a0a0a sits on the sRGB linear
// branch; the swap and the 4.5 boundary keep a wrong gamma or a strict >
// from agreeing with these numbers.
const referencePairs: readonly [string, string, number][] = [
  ['#777', '#fff', 4.48],
  ['#fff', '#777', 4.48],
  ['#767676', '#fff', 4.54],
  ['#000000', '#ffffff', 21],
  ['#595959', '#fff', 7],
  ['#0a0a0a', '#000', 1.0607],
]

describe('contrast', () => {
  it.each(referencePairs)('%s on %s is %s', (foreground, background, ratio) => {
    const precision = ratio < 2 ? 4 : 2
    expect(contrastRatio(foreground, background)).toBeCloseTo(ratio, precision)
  })

  it('flattens a half-black wash over white to gray', () => {
    expect(flatten('#000000', 0.5, '#ffffff')).toBe('#808080')
  })

  it('reports when no shade clears the minimum', () => {
    expect(readableShade(['#ffffff', '#eeeeee'], '#ffffff', 0)).toBe(-1)
    expect(readableLightShade(['#111111', '#000000'], '#000000', 1)).toBe(-1)
    const boundary = contrastRatio('#767676', '#ffffff')
    expect(readableShade(['#777777', '#767676'], '#ffffff', 0, boundary)).toBe(1)
  })
})
