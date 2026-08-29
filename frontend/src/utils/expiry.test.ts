import { describe, expect, it } from 'vitest';
import fc from 'fast-check';
import { computeExpiryStatus } from './expiry';

const ONE_DAY_MS = 24 * 60 * 60 * 1000;

// Feature: pantry-management, Property 9: Expiry status is consistent with dates and warning period
//
// Validates: Requirements 2.8, 2.9
//
// For any item instance with an expiration date and any current time, the computed expiry status
// SHALL satisfy: expired when expiresAt < now; near_expiry when 0 <= (expiresAt - now) <= warningDays;
// and ok otherwise. Instances with no expiration date SHALL always have status ok.
describe('computeExpiryStatus', () => {
  it('always returns ok when there is no expiration date', () => {
    fc.assert(
      fc.property(
        fc.date({ noInvalidDate: true }),
        fc.integer({ min: 1, max: 30 }),
        (now, warningDays) => {
          expect(computeExpiryStatus(null, now, warningDays)).toBe('ok');
        },
      ),
    );
  });

  it('satisfies all three status branches for arbitrary dates and warning periods', () => {
    fc.assert(
      fc.property(
        fc.date({
          min: new Date('2020-01-01T00:00:00Z'),
          max: new Date('2030-12-31T23:59:59Z'),
          noInvalidDate: true,
        }),
        fc.integer({ min: 1, max: 30 }),
        fc.integer({ min: -60 * ONE_DAY_MS, max: 60 * ONE_DAY_MS }),
        (now, warningDays, offsetMs) => {
          const expiresAt = new Date(now.getTime() + offsetMs);
          const daysUntilExpiry = offsetMs / ONE_DAY_MS;

          const got = computeExpiryStatus(expiresAt.toISOString(), now, warningDays);

          if (daysUntilExpiry < 0) {
            expect(got).toBe('expired');
          } else if (daysUntilExpiry <= warningDays) {
            expect(got).toBe('near_expiry');
          } else {
            expect(got).toBe('ok');
          }
        },
      ),
    );
  });
});
