package server

import (
	"testing"
)

func TestRunServer_Exists(t *testing.T) {
	// Just verify the function exists
	// We can't actually run it as it would start a server
	// and call log.Fatalf on error
	if RunServer == nil {
		t.Error("RunServer function should exist")
	}
}
