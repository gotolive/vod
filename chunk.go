package vod

import (
	"os"
)

type tsChunk struct {
	id       int
	duration float64
	done     chan bool
	f        *os.File
	path     string
}

func (c *tsChunk) Read(p []byte) (int, error) {
	if c.f == nil {
		var err error
		c.f, err = os.Open(c.path)
		if err != nil {
			return 0, err
		}
	}

	return c.f.Read(p)
}

// Close the chunk fd
func (c *tsChunk) Close() error {
	f := c.f
	c.f = nil

	return f.Close()
}

func (c *tsChunk) destroy() {
	_ = c.Close()
	_ = os.Remove(c.path)
}
