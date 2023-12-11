package main

type agiMonitorCmd struct {
	Listen agiMonitorListenCmd `command:"listen" subcommands-optional:"true" description:"Run AGI monitor listener"`
	Create agiMonitorCreateCmd `command:"create" subcommands-optional:"true" description:"Create a client instance and run AGI monitor on it"`
}

type agiMonitorListenCmd struct {
}

type agiMonitorCreateCmd struct {
}

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
