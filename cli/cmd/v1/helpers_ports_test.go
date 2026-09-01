package cmd

import (
	"testing"

	"github.com/aerospike-community/aerolab/pkg/backend/backends"
)

func TestParsePortRangeAll(t *testing.T) {
	cases := []struct {
		in       string
		protocol string
		from     int
		to       int
	}{
		{"all", backends.ProtocolAll, -1, -1},
		{"-1", backends.ProtocolAll, -1, -1},
		{"all:anything", backends.ProtocolAll, -1, -1},
		{"22", "tcp", 22, 22},
		{"tcp:3000-3005", "tcp", 3000, 3005},
		{"udp:53", "udp", 53, 53},
	}
	for _, c := range cases {
		protocol, from, to, err := parsePortRange(c.in)
		if err != nil {
			t.Errorf("parsePortRange(%q) error: %v", c.in, err)
			continue
		}
		if protocol != c.protocol || from != c.from || to != c.to {
			t.Errorf("parsePortRange(%q) = (%s, %d, %d), want (%s, %d, %d)", c.in, protocol, from, to, c.protocol, c.from, c.to)
		}
	}
}
