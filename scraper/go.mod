module github.com/screwyprof/delegator/scraper

go 1.24.4

require github.com/screwyprof/delegator/pkg v0.0.0

require (
	github.com/jackc/pgx/v5 v5.7.5
	github.com/stretchr/testify v1.10.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-gorp/gorp/v3 v3.1.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/peterldowns/pgtestdb v0.1.1 // indirect
	github.com/peterldowns/pgtestdb/migrators/sqlmigrator v0.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rubenv/sql-migrate v1.8.0 // indirect
	golang.org/x/crypto v0.37.0 // indirect
	golang.org/x/sync v0.13.0 // indirect
	golang.org/x/text v0.24.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// Local development - point to local pkg directory
replace github.com/screwyprof/delegator/pkg => ../pkg
