package squashfs

import (
	"os"
	"os/exec"
	"testing"
)

var checkSQFSTar = func(_ *testing.T) {}

func TestMain(m *testing.M) {
	_, err := exec.LookPath("sqfstar")
	if err != nil {
		checkSQFSTar = (*testing.T).SkipNow
	}

	os.Exit(m.Run())
}

func TestLoad(t *testing.T) {
	checkSQFSTar(t)
}
