package api

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
