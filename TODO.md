# TODO for 7.4.0 to be ready

* WebUI: AGI Details button (provide a nice way to show mapping of original->new names and other details, including errors and agi stack logs)
  * consider hosting this on agi-proxy as a webpage instead of webui
* Agi - after it terminated with efs/vol, on restart, will it remember which instance size was used for a given volume? If not, will it be aware of need to size, and will it ask the monitor to size (like a file on the volume with sizing information)?
* Test GCP,AWS,Docker all buttons in inventory system in webui
* Weird bug: `2024/02/21 14:27:12 -0800 invalid character '<' looking for beginning of value` in webui logs - where is that from; also the inventory stops refreshing when this happens; out of API requests?
