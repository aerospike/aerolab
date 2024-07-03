# WebUI Hosted Mode

Launching AeroLab Web-based user interface (WebUI) is as simple as running `aerolab webui`.

In shared-account situation, it may be desirable to host aerolab on a webserver for multiple users. This document describes the options possible and required to achieve this.

##  Extra parameters

To use hosted-mode, extra parameters at the end of the `aerolab webui` command must be provided. Full list of parameters for tuning the `webui` behaviours and limits can be obtained by running `aerolab webui help`.

The below outlines an example with just the hosted-mode specific parameters:

```
aerolab webui --listen 0.0.0.0:3333 --listen [::]:3333 --nobrowser --block-server-ls --unique-firewalls --agi-strict-tls --ws-proxy-origin aerolab.example.com --strict-agi-tls
```

Explanation of parameters:
* listen on port 3333 on all IPs (tcp4 and tcp6)
* do not attempt to open the browser
* block server-side `ls` file exploration feature fully and only allow upload/download client feature
  * you may omit this, and indeed can set the opposite: `--always-server-ls` - this will give users full access to the server's data
  * default behaviour is to allow `ls` if user is connecting from `localhost` and the following headers are not set: `X-Real-IP` and `X-Forwarded-For`.
* have a unique firewall/security group name per user
* (optional) enable strict TLS checks in all AGI components - this requires extra setup (see below)
* (optional) when behind a proxy which sets an `Origin` header, specify which origin headers are allowed for WebSocket support

## TLS for WebUI

AeroLab WebUI does not provide TLS support out of the box. If TLS and/or authentication are required, use a TLS/Authenticating proxy in front of aerolab webui. Have a look at [this project](https://github.com/oauth2-proxy/oauth2-proxy/releases) as a useable example. When using a proxy, either:
* the proxy must not set OR override `Origin` header to match aerolab listen Host
* or when starting aerolab, provide `--ws-proxy-origin=` parameter with host that will be allowed (for example `--ws-proxy-origin aerolab.example.com`)

## AGI Strict TLS

This mode requries all AGI instances and AGI Monitor to have valid TLS certificates. For the purpose of completeness, the below section explains how to set the whole WebUI and AGI to use strict TLS checking. This will also explain how to start AGI instances with valid DNS names (required) in this mode.

Since aerolab, agi and agi-monitor communicate with each other, once `--agi-strict-tls` is enabled, these components must be able to communicate using valid TLS certificates.

Component | Description
--- | ---
AeroLab WebUI | does not use certificates, requires a separate TLS proxy in front
AGI | Has it's own web server and requires TLS certificates. These can be provided using `aerolab agi create ... --proxy-ssl-cert=cert.pem --proxy-ssl-key=key.pem`. If deploying AGIMonitor at the same time (ie using `--with-monitor` and AGI Monitor does not exist yet), also provide agi-monitor certificates using either autocert or manual cert: `--with-monitor --monitor-autocert= --monitor-autocert-email= --monitor-cert-file= --monitor-key-file= `. See `aerolab agi create help` for more switches and details.
AGI (DNS) | For AGI certificates to be valid, they must refer to a valid domain. This can be done manually, or alternatively in AWS the certificates could be created for a wildcard domain per region (ex `*.eu-west-1.agi.example.com`), the domain `example.com` hosted in AWS Route53, and AGI could automatically handle creating domains in route53 for each instance as `{instanceid}.{region}.agi.{domain}` for example `i-156312974982.eu-west-1.agi.example.com`. To have this work, use `aerolab agi create ... --route53-zoneid= --route53-domain=`.
AGI Monitor | The AGI Monitor can have certificates provided either as part of deploying the first AGI instance (see AGI part above) or using `aerolab agi monitor create ... --autocert= --autocert-email= --cert-file= --key-file=`. Either autocert OR cert+key information has to be provided. See `aerolab agi monitor help` for details. It is adviseable to create AGI Monitor manually for a hosted `aerolab` to avoid race conditions when multiple users try to create an AGI instance at the same time. A domain must also be created for AGI Monitor to use, so that it's DNS will be accurate. If a domain is manually configured, `aerolab agi create ... --agi-monitor-url=` must be manually specified for each AGI instance, as they will be unaware of the domain name that was used. Alternatively, aerolab can be told to create the domain in AWS route53 automatically using `aerolab agi monitor create ... --route53-zoneid= --route53-fqdn=`.

### Example

Example usage with auto domains (assumes AeroLab WebUI is deployed on a server, and the commands are being executed from said server, on AWS):
```
# deploy AGI Monitor, use autocert for certificates, and configure route53 DNS automatically
aerolab agi monitor create --autocert=agimonitor.eu-west-1.example.com --autocert-email=robert@example.com --route53-zoneid=XZ12784628 --route53-fqdn=agimonitor.eu-west-1.example.com --strict-agi-tls

# set the defaults for AGI so WebUI users don't have to fill them
aerolab config defaults -k AGI.Create.WithAGIMonitorAuto -v true
aerolab config defaults -k AGI.Create.ProxyCert -v cert.pem # cert for *.eu-west-1.agi.example.com
aerolab config defaults -k AGI.Create.ProxyKey -v key.pem   # key for *.eu-west-1.agi.example.com
aerolab config defaults -k AGI.Create.Aws.Route53ZoneId -v XZ12784628
aerolab config defaults -k AGI.Create.Aws.Route53DomainName -v eu-west-1.agi.example.com
aerolab config defaults -k AGI.Create.Aws.WithEFS -v true

# start webui
aerolab webui --listen 0.0.0.0:3333 --listen [::]:3333 --nobrowser --block-server-ls --unique-firewalls --agi-strict-tls --ws-proxy-origin aerolab.example.com
```

## Certbot Example

```
certbot certonly --dns-route53 -n -d '*.ca-central-1.agi.aerolab.aerospike.me'
```

## Notes

While aerolab can be installed on the destination machine as `aerolab cluster add aerolab -n NAME` or `aerolab client configure aerolab -n NAME`, the version deployed will not be the full /embedded/ version. To then upgrade to a full aerolab version, run this: `aerolab attach shell -n NAME -- aerolab upgrade [--edge]` or `aerolab attach client -n NAME -- aerolab upgrade [--edge]`.

## Simple mode

AeroLab WebUI comes with simple mode. By default the said mode can be switched on and off. In certain situations, such as when hosting aerolab for users, it may be useful to disable full mode and select which simple-mode features are allowed. To start aerolab, forcing simple-mode, add `--force-simple-mode` to the command.

To configure extra options (such as enabling and disabling form fields and commands) in simple mode, create a file at `~/.aerolab/www-simple-mode.list` (or wherever `$AEROLAB_HOME` points to if set). The contents is a one-per-line list of commands and switches. Note that to enable subcommands, the top level commands must be enabled.

All items can be obtained by executing `aerolab config defaults` option. Once that is done, put the keys in the list file.

Prepend `-` to disallow or `+` to allow.

For example, to disallow `cluster grow` command and all `cluster partition` commands, while explicitly enabling `cluster create` with `--count`:
```
-cluster.grow
-cluster.partition
-cluster.partition.create
-cluster.partition.mkfs
-cluster.partition.conf
-cluster.partition.list
+clustr.create.count
```

### Modifiers

While the defaults are ment to provide a sane single-user experience, special modifiers can be put in the first line to either start with allowing all options or start with disallowing all options (ignoring software defaults). The below allows all options in simple mode EXCEPT `cluster grow` command.

```
+ALL
-cluster.grow
```

Option `-ALL` is also allowed to perform the reverse.

### WebUI Inventory - hiding tabs

Tabs are always enabled by default (the `-ALL` modifier has no effect). If for example AGI is disabled in simple mode, you may wish to disable the `AGI` tab in the inventory listing from showing. To do that, just add `-INVENTORY:AGI` line in the list.

Full list of tab disabling options for inventory tabs:

```
INVENTORY:CLUSTERS
INVENTORY:CLIENTS
INVENTORY:AGI
INVENTORY:TEMPLATES
INVENTORY:VOLUMES
INVENTORY:FIREWALLS
INVENTORY:EXPIRY
INVENTORY:SUBNETS
```