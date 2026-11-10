package atspi

import (
	"testing"

	"github.com/stubbedev/lys-ui/internal/testhome"
)

func TestMain(m *testing.M) { testhome.Run(m) }

func TestTheTestsReachNoHostSession(t *testing.T) { testhome.RequireNoHostSession(t) }
