package pack

import "testing"

func TestNormalizedConcurrency(t *testing.T) {
	c := Config{}
	if got := c.normalizedConcurrency(); got <= 0 {
		t.Fatalf("expected >0 CPUs, got %d", got)
	}
	c.Concurrency = 7
	if got := c.normalizedConcurrency(); got != 7 {
		t.Fatalf("expected 7, got %d", got)
	}
}
