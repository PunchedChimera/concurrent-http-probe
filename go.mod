// go.mod is Go's equivalent of pom.xml / build.gradle.
// The module path is also the import prefix for your packages.
// We'll update this to match your GitHub username once that's set up.
module github.com/punchedchimera/concurrent-http-probe

// Go version constraint — the minimum version required to build this module.
go 1.22

require github.com/spf13/cobra v1.8.1

// 'indirect' means this is a dependency-of-a-dependency (transitive).
// Go lists them explicitly for reproducible builds — no surprises like Maven's
// dependency mediation rules. go.sum locks the exact checksums.
require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
)
