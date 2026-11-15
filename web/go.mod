module github.com/screwyprof/delegator/web

go 1.27.3

require (
	github.com/caarlos0/env/v11 v11.4.2
	github.com/jackc/pgx/v5 v5.13.0
	github.com/peterldowns/pgtestdb v0.1.1
	github.com/screwyprof/delegator/migrator v0.0.0-00010101000000-000000000000
	github.com/screwyprof/delegator/pkg v0.0.0
	github.com/screwyprof/delegator/scraper v0.0.0
	github.com/stretchr/testify v1.12.1
	github.com/zolstein/pgx-collect v0.0.0-20260726200924-f9321240541d
)

require (
	github.com/go-gorp/gorp/v3 v3.1.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	github.com/peterldowns/pgtestdb/migrators/sqlmigrator v0.1.1 // indirect
	github.com/rubenv/sql-migrate v1.8.1 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/text v0.44.0 // indirect
)

// Local development - point to local directories
replace github.com/screwyprof/delegator => ../

replace github.com/screwyprof/delegator/pkg => ../pkg

replace github.com/screwyprof/delegator/migrator => ../migrator

replace github.com/screwyprof/delegator/scraper => ../scraper
