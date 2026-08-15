# Test Coverage Scripts

This directory contains scripts for managing test coverage in the Pantry project.

## Scripts

### test-coverage.sh

Runs all tests with coverage and enforces a minimum coverage threshold.

```bash
./scripts/test-coverage.sh
```

**Features:**
- Runs tests with `-coverpkg=./...` to measure coverage across all packages
- Calculates total line coverage percentage
- Enforces minimum coverage threshold (configurable)
- Exits with non-zero code if tests fail or coverage is below threshold
- **Automatically updates the threshold** when coverage increases (ratchet mechanism)

**Configuration:**
The `COVERAGE_THRESHOLD` variable is set at the top of the script. 

**Important:** When the script detects that coverage has increased beyond the current threshold, it automatically updates itself to use the new, higher coverage value as the threshold. This creates a "ratchet" effect where coverage can only go up, never down. When this happens, commit the updated script file to preserve the new baseline.

### show-coverage.sh

Runs tests and displays coverage without enforcement. Useful during development.

```bash
./scripts/show-coverage.sh

# Generate HTML coverage report
./scripts/show-coverage.sh --html
```

**Features:**
- Shows total coverage percentage
- Optional HTML report generation with `--html` flag
- No threshold enforcement - purely informational

## Usage Guidelines

- Run `test-coverage.sh` after completing each task (required by AGENTS.md)
- Use `show-coverage.sh` during development to check current coverage
- Use `show-coverage.sh --html` to see detailed line-by-line coverage
