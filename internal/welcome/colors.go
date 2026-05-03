package welcome

import (
	"os"
	"strings"
)

type colors struct {
	enabled bool
}

func newColors() *colors {
	return &colors{enabled: shouldUseColor()}
}

func shouldUseColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	if os.Getenv("TOKENDOG_FORCE_COLOR") != "" {
		return true
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func (c *colors) wrap(code, s string) string {
	if !c.enabled {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

func (c *colors) bold(s string) string   { return c.wrap("1", s) }
func (c *colors) dim(s string) string    { return c.wrap("2", s) }
func (c *colors) red(s string) string    { return c.wrap("31", s) }
func (c *colors) green(s string) string  { return c.wrap("32", s) }
func (c *colors) yellow(s string) string { return c.wrap("33", s) }
func (c *colors) cyan(s string) string   { return c.wrap("36", s) }
func (c *colors) gray(s string) string   { return c.wrap("90", s) }
