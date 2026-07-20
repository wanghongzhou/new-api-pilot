# Integration Tests

MySQL 8.4 integration and worker acceptance tests referenced by
`docs/acceptance/manifest.yaml` live in this directory. SQLite is not a valid
substitute for the locking, collation, and transaction behavior under test.
