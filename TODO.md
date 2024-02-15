# TODO for 7.4.0 to be ready

* AGI
  * AGI Details button (provide a nice way to show mapping of original->new names and other details, including errors and agi stack logs)
  * Agi - after it stopped, on restart, will it remember which instance size was used for a given volume? If not, will it be aware of need to size, and will it ask the monitor to size (like a file on the volume with sizing information)?

* Other
  * telemetry - report the command run properly somehow
  * gcp expiries - tokens changing in scheduler --> multi-user reinstalls gcloud (solution? use a bucket to store expiries system installed status, version, and region)
