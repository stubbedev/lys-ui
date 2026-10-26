package headlesstest

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/capture"

	"github.com/stubbedev/lys-ui/app"
)

// Two connections in one process (an application and a capture client,
// as a shell taking a screenshot holds) each bind a hotplugged output
// on their own connection. The binding once kept the registry's
// connection in a process-wide map keyed by proxy id, so the first
// connection's registry (id 2, like every registry) resolved to the
// second connection: the output it bound took an id there that never
// went out, and every later request creating an object on the second
// connection waited for it forever.
func TestTwoConnectionsBindAHotpluggedOutput(t *testing.T) {
	requireEnv(t)
	if testEnv.Compositor().Name() != "sway" {
		t.Skip("output hotplug drives sway's IPC")
	}
	sess, err := app.Connect()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(sess.Close)
	c, err := capture.Connect()
	if err != nil {
		t.Fatalf("capture connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	before := len(sess.Outputs())

	if err := swayIPCCommand(testEnv.Dir, "create_output"); err != nil {
		t.Fatalf("create_output: %v", err)
	}
	name := ""
	deadline := time.Now().Add(5 * time.Second)
	for len(sess.Outputs()) <= before {
		if time.Now().After(deadline) {
			t.Fatalf("the application never saw the hotplugged output (%d outputs)", len(sess.Outputs()))
		}
		if err := sess.Roundtrip(); err != nil {
			t.Fatalf("application roundtrip after the hotplug: %v", err)
		}
	}
	if err := sess.Roundtrip(); err != nil {
		t.Fatalf("application roundtrip: %v", err)
	}
	outs := sess.Outputs()
	name = outs[len(outs)-1].Name
	t.Cleanup(func() { _ = swayIPCCommand(testEnv.Dir, "output "+name+" unplug") })

	done := make(chan error, 1)
	go func() {
		if err := c.Refresh(); err != nil {
			done <- err
			return
		}
		_, err := c.CaptureOutput(c.Outputs()[0], capture.Options{})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("capture after the hotplug: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the capture connection stalled after the application bound the hotplugged output")
	}
}
