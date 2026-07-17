package proxy

import "os"

// envLookup is indirected for tests.
var envLookup = os.Getenv
