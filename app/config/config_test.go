package config

import (
	"flag"
	"os"
	"testing"
)

func withArgs(t *testing.T, args []string) {
	t.Helper()
	origArgs := os.Args
	origFlagSet := flag.CommandLine
	t.Cleanup(func() {
		os.Args = origArgs
		flag.CommandLine = origFlagSet
	})
	flag.CommandLine = flag.NewFlagSet(args[0], flag.ContinueOnError)
	os.Args = args
}

func TestParse_BlindStoreParticipantDefaultsToSelectiveMode(t *testing.T) {
	withArgs(t, []string{"securep2p"})

	cfg := Parse()
	if !cfg.BlindStoreParticipant {
		t.Fatal("regular nodes should participate in selective blind-store replication by default")
	}
	if cfg.StoreNode {
		t.Fatal("regular nodes should not be store nodes by default")
	}
}

func TestParse_StoreNodeForcesParticipation(t *testing.T) {
	withArgs(t, []string{"securep2p", "-store-node"})

	cfg := Parse()
	if !cfg.StoreNode {
		t.Fatal("StoreNode should be true when -store-node is passed")
	}
	if !cfg.BlindStoreParticipant {
		t.Fatal("store nodes must always subscribe to blind-store")
	}
}
