# TODO for 7.4.0 to be ready

* on any 'create/grow' commands, should we trigger inventory cache refresh in the background? We already trigger on delete run from the inventory pages. We /could/ trigger on any action completion instead, but that might be a lot (and we'd need a resettable timer for timed refresh)
  * maybe a struct tag for commands that require an inventory refresh and if they are triggered, inventory refresh should happen
* 'enable services' in gcp needs special handling (some sort of button present in inventory)
* inventory expiry system - add buttons to install, delete, change frequency
* inventory volumes - adjust what we show and how we show it in tables - match cli
* inventory volumes - add buttons for create/mount/delete/grow/detach (show each only for relevant backend, part-fill the form using get vars)

* add inventory clusters
* add inventory clients
* add inventory agi - special page joining AGI volumes and AGI clusters

* Agi - after it stopped, on restart, will it remember which instance size was used for a given volume? If not, will it be aware of need to size, and will it ask the monitor to size (like a file on the volume with sizing information)?
* telemetry - report the command run properly somehow
