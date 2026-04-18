# testdb — per-test database helpers

This package provides `NewTx` (transaction-wrapped, auto-rollback) and
`NewDB` (real connection, truncate-on-cleanup) for integration tests.

Tests requiring these helpers skip unless `TEST_DATABASE_URL` points at
a Postgres instance with the marketplace-api migrations applied.

## Role-level revoke tests

Some security tests (e.g. `business_entity_attestations` DELETE rejection)
need a non-superuser connection. Set `TEST_DATABASE_APP_URL` to a DSN whose
role has been granted only `SELECT, INSERT, UPDATE` on the table — NOT
`DELETE`. Migration 000043 applies the `REVOKE DELETE` during migration.

If `TEST_DATABASE_APP_URL` is unset, role-scoped tests skip with a clear
message. Set this in CI so security regressions can't land silently.
