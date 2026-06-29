---
name: run-tests
description: Run the project test suite to verify correctness. Tests are located in tests/ and c-tests/ and take approximately 60 seconds.
---

# Run Tests

There are two ways to run the test suite. Tests are located in `tests/` and `c-tests/` and typically take about 60 seconds to complete.

## Option 1: Using `make` (Recommended)

Run `make` from the project root. Test output is cached in the `_test_out` directory. If tests fail, look in `_test_out` for detailed failure output.

```bash
make
```

After running, check for failures by examining files in `_test_out/`.

## Option 2: Using `go test` directly

Run all tests directly with Go's test runner:

```bash
go test -count=1 ./...
```

The `-count=1` flag disables test caching to ensure tests are always re-run.

## Notes

- Expect tests to take approximately 60 seconds to complete.
- When using `make`, always check `_test_out` for failure details rather than relying solely on the console summary.
