package api

// Loopback is the address the daemon always binds, so the test that proves it is this machine and
// nothing else can read the same constant the server does (docs/architecture.md section 13).
func Loopback() string { return loopback }

// FunnelAddress is the port Funnel is opened on, for the test that proves the daemon only ever
// opens that one public listener.
func FunnelAddress() string { return funnelAddr }

// RoutePatterns lists the method and path of every domain route, for the tests in package
// api_test. It reads the same table that registers the routes, so a route cannot be added without
// the token test covering it.
func RoutePatterns() []string {
	specs := domainRoutes()
	patterns := make([]string, 0, len(specs))
	for _, spec := range specs {
		patterns = append(patterns, spec.pattern)
	}
	return patterns
}
