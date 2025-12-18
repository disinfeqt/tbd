module twitter-bookmarks-downloader

go 1.24.2

require (
	github.com/fatih/color v1.18.0
	github.com/imperatrona/twitter-scraper v0.0.17
	github.com/rotisserie/eris v0.5.4
	github.com/stretchr/testify v1.10.0
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.31.1
)

require (
	github.com/AlexEidt/Vidio v1.5.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-sqlite3 v1.14.22 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/net v0.39.0 // indirect
	golang.org/x/sys v0.32.0 // indirect
	golang.org/x/text v0.24.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/imperatrona/twitter-scraper => ./twitter-scraper
