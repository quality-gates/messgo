// Package version provides the single version identity used by every messgo output.
package version

// Version is "dev" in local builds and is set from the validated release tag
// with the Go linker's -X flag for release artifacts.
var Version = "dev"
