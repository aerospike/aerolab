# TODO for 7.4.0 to be ready

* WebUI: AGI Details button (provide a nice way to show mapping of original->new names and other details, including errors and agi stack logs)
  * consider hosting this on agi-proxy as a webpage instead of webui
* Agi - after it stopped, on restart, will it remember which instance size was used for a given volume? If not, will it be aware of need to size, and will it ask the monitor to size (like a file on the volume with sizing information)?
* Agi - check if it rotated instance using monitor on usage to avoid out of memory
* Docker with `--no-autoexpose` causes IPs on clusters/clients in inventory listing to show up as null
* Test GCP,AWS,Docker all buttons in inventory system in webui
