package cmd_test

import (
	"testing"

	"github.com/thiagomarinho/joca/cmd"
)

func TestVersionCmd_registered(t *testing.T) {
	found := false
	for _, sub := range cmd.RootCmd.Commands() {
		if sub.Name() == "version" {
			found = true
			break
		}
	}
	if !found {
		t.Error("version command not registered on root")
	}
}
