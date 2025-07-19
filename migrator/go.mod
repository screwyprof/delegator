module github.com/screwyprof/delegator/migrator

go 1.24.4

require (
	github.com/caarlos0/env/v11 v11.3.1
	github.com/jackc/pgx/v5 v5.7.5
	github.com/peterldowns/pgtestdb v0.1.1
	github.com/peterldowns/pgtestdb/migrators/sqlmigrator v0.1.1
	github.com/rubenv/sql-migrate v1.8.0
	github.com/screwyprof/delegator/pkg v0.0.0
	github.com/screwyprof/delegator/scraper v0.0.0
	github.com/stretchr/testify v1.10.0
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-gorp/gorp/v3 v3.1.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/crypto v0.40.0 // indirect
	golang.org/x/exp v0.0.0-20240909161429-701f63a606c0 // indirect
	golang.org/x/sync v0.16.0 // indirect
	golang.org/x/text v0.27.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/screwyprof/delegator => ../

replace github.com/screwyprof/delegator/pkg => ../pkg

replace github.com/screwyprof/delegator/scraper => ../scraper
