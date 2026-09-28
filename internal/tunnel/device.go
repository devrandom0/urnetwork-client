// Package tunnel opens the TUN device and runs the dataplane loop between it and the provider.
package tunnel

import "io"

// Device is the TUN interface the dataplane reads from and writes to.
type Device interface {
	io.ReadWriteCloser
	Name() string
}
