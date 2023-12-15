# Aerospike Grafana Integrated stack

The AeroLab AGI features allows for graphing the statistics from Aerospike logs in grafana. AeroLab automatically deploys the following on each AGI instance:
* aerospike as a backend storage database
* logingest tool for ingesting statistics from Aerospike logs into the storage database
* grafana for the UI
* aerospike-grafana plugin to allow queries from grafana into aerospike
* helpful tools
  * online filebrowser
  * web console
  * showconf, showinterrupts and showsysinfo collectinfo explorer tools
  * an authenticating proxy service
  * other helpful tools, such as lnav

This document explains the basic usage, and examples, of AGI.

## Usage

See [this page](usage.md) for usage.

## AWS EFS and GCP extra volumes

See [this page](efs.md) for usage with persistent elastic volumes for data storage.

## Notification support

See [this page](notify.md) for usage with notification support.

## AGI Monitor

See [this page](monitor/README.md) for usage with AGI monitor support.

## Examples

See [this page](example.md) for usage examples.
