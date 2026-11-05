// Package buildinfo says which version of Marshal is running.
package buildinfo

// Version is the one version number that the daemon, the command line tool, and the app share.
// Release builds set this with Go’s -X linker flag.
var Version = "0.0.0"
