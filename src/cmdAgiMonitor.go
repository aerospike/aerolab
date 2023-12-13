package main

type agiMonitorCmd struct {
	Listen agiMonitorListenCmd `command:"listen" subcommands-optional:"true" description:"Run AGI monitor listener"`
	Create agiMonitorCreateCmd `command:"create" subcommands-optional:"true" description:"Create a client instance and run AGI monitor on it"`
}

type agiMonitorListenCmd struct {
	ListenAddress   string   `long:"address" description:"address to listen on; if autocert is enabled, will also listen on :80" default:"0.0.0.0:443"`                                 // 0.0.0.0:443, not :80 is also required and will be bound to if using autocert
	UseTLS          bool     `long:"tls" description:"enable tls"`                                                                                                                      // enable TLS
	AutoCertDomains []string `long:"autocert" description:"TLS: if specified, will attempt to auto-obtain certificates from letsencrypt for given domains, can be used more than once"` // TLS: if specified, will attempt to auto-obtain certificates from letsencrypt for given domains
	CertFile        string   `long:"cert-file" description:"TLS: certificate file to use if not using letsencrypt; default: generate self-signed"`                                      // TLS: cert file (if not using autocert), default: snakeoil
	KeyFile         string   `long:"key-file" description:"TLS: key file to use if not using letsencrypt; default: generate self-signed"`                                               // TLS: key file (if not using autocert), default: snakeoil
	// TODO: configs for how to perform auto-sizing and auto-rotation for capacity (try another AZ, try another instance type, rotate to on-demand, what instance types to use for sizing, etc)
}

type agiMonitorCreateCmd struct {
	Name string
	agiMonitorListenCmd
	// gcp: zone
	// aws/gcp/docker specific switches, as few as possible, this is an embedded solution
}

// TODO: AUTH
// call: notifier.DecodeAuthJson("") to get the auth json values
// get the instance details from backend
// compare

/* TODO:
receive events from agi-proxy http notifier
authenticate them
if event is sizing:
 - check log sizes, available disk space (GCP) and RAM
 - if disk size too small - grow it
 - if RAM too small, tell agi to stop, shutdown the instance and restart it as larger instance accordingly (configurable sizing options)
if event is spot termination:
 - respond 200 ok, stop on this event is not possible
 - terminate the instance
 - restart the instance as ondemand or as different stop (different AZ, or type) as per configuration for "next in line" rotation - need to carry what we tried so far in a label/tag so we can try the "next step"

what we need:
aerolab agi monitor create --listen 0.0.0.0:4433 --cert x.pem --key y.pem OR --autocert=domain.example.com // --autocert only if we can listen on 80, or otherwise error
aerolab agi monitor run --listen 0.0.0.0:4433 --cert x.pem --key y.pem OR --autocert=domain.example.com // --autocert only if we can listen on 80, or otherwise error

first one creates a none client with aerolab inside and the required systemd file/docker autoload script
second one actually runs the monitor
*/

/*
* TODO: Document agi instance state monitor.
  * `aerolab client create none -n agi-monitor; aerolab client configure aerolab -n agi-monitor; aerolab attach client -n agi-monitor --detach -- /usr/local/bin/aerolab agi monitor`
  * document what it's for: to run sizing for agi instances in AWS/GCP which use volume backing, and to cycle spot to on-demand if capacity becomes unavailable
  * document running monitor locally
  * document usage with AGI instances (need to specify `--monitor-url` and must have a backing volume)
*/
