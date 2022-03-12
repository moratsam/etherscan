package gravitas

import (
	_ "github.com/hashicorp/go-multierror"
	_ "golang.org/x/xerrors"
)

// Config encapsulates the required parameters for creating a new Gravitas
// calculator instance.
type Config struct {
	// The number of workers to spin up for computing Gravitas scores. If
	// not specified, a default value of 1 will be used instead.
	ComputeWorkers int
}

// validate checks whether the Gravitas calculator configuration is valid and
// sets the default values where required.
func (c *Config) validate() error {
	var err error
	if c.ComputeWorkers <= 0 {
		c.ComputeWorkers = 1
	}

	return err
}
