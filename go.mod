module archive-downloader

go 1.25

// go mod tidy (run by the Docker build) resolves and adds the remaining
// imports: bodgit/sevenzip, nwaples/rardecode/v2.
require (
	github.com/BrandonKowalski/certifiable v1.3.0
	github.com/BrandonKowalski/gabagool/v2 v2.21.0
	go.uber.org/atomic v1.11.0
	gopkg.in/yaml.v3 v3.0.1
)
