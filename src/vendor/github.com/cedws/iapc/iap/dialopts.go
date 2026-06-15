package iap

import (
	"golang.org/x/oauth2"
)

type DialOption func(*dialOptions)

type dialOptions struct {
	Zone        string
	TokenSource *oauth2.TokenSource
	Region      string
	Project     string
	Port        string
	Network     string
	Interface   string
	Instance    string
	Host        string
	Group       string
	Compress    bool
}

func (d *dialOptions) collectOpts(opts []DialOption) {
	for _, opt := range opts {
		opt(d)
	}
}

// WithTokenSource is a functional option that sets the authorization toke source.
func WithTokenSource(tokenSource *oauth2.TokenSource) func(*dialOptions) {
	return func(d *dialOptions) {
		d.TokenSource = tokenSource
	}
}

// WithCompression is a functional option that enables compression.
func WithCompression() func(*dialOptions) {
	return func(d *dialOptions) {
		d.Compress = true
	}
}

// WithProject is a functional option that sets the project ID.
func WithProject(project string) func(*dialOptions) {
	return func(d *dialOptions) {
		d.Project = project
	}
}

// WithInstance is a functional option that sets the instance, zone, and network interface.
func WithInstance(instance, zone, ninterface string) func(*dialOptions) {
	return func(d *dialOptions) {
		d.Instance = instance
		d.Zone = zone
		d.Interface = ninterface
	}
}

// WithHost is a functional option that sets the host, region, network, and destination group.
func WithHost(host, region, network, destGroup string) func(*dialOptions) {
	return func(d *dialOptions) {
		d.Host = host
		d.Region = region
		d.Network = network
		d.Group = destGroup
	}
}

// WithPort is a functional option that sets the destination port.
func WithPort(port string) func(*dialOptions) {
	return func(d *dialOptions) {
		d.Port = port
	}
}
