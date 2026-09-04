//go:build nodb

package sqlite

func Available() bool { return false }
