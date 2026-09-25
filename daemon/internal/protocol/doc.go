// Package protocol holds the types that go over the wire between the daemon and its clients,
// and the small rules that belong to them: the timestamp format, ids, error codes, topics, and
// the fixed lists of allowed values. They are written once here and generated into TypeScript
// for the app (packages/protocol), so the two sides cannot drift. Keep the package free of I/O
// and of imports of other daemon packages. docs/architecture.md section 11.5 explains the rules.
package protocol
