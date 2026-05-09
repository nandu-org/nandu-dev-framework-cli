module github.com/nandu-org/nandu-dev-framework-cli

go 1.22

// Single dependency: golang.org/x/term for hidden password input on `ndf login`.
// Cross-platform (Windows/macOS/Linux), part of the Go-team-maintained x/ tree.
// No transitive dependencies beyond the Go stdlib.
require golang.org/x/term v0.27.0

require golang.org/x/sys v0.28.0 // indirect
