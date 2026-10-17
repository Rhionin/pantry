// Temporary proof that a failing mobile test turns the CI check red.
describe('mobile CI proof', () => {
  it('fails on purpose', () => {
    expect(true).toBe(false);
  });
});
