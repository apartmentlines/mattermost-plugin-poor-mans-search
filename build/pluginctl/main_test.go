package main

import "testing"

// TestPackageCompiles keeps `go test ./...` from reporting this helper package
// as having no test files.
func TestPackageCompiles(_ *testing.T) {}
