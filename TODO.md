# TODO for 7.4.0 to be ready

* add inventory clusters
* add inventory clients
* add inventory agi - special page joining AGI volumes and AGI clusters
* if backend or defaults changed via cli, find a way to provide a reload in the browser. probably this: on backend/defaults change, safe timestamp of last change in file. webui inventory refresh system: check timestamp of last change. if exists and is newer than what we remember from start, reload.
* when switching backend, force inventory refresh and force-clear all datatables plus early-update inventory listing names
* when switching to a backend that doesn't work, what happens? need errors popping up!
* latest build still has "old name new name" bug! need to fix - docker; also cannot delete old template?
* inventory templates - add "create" option that takes us to template create
* inventory firewalls - add "create" and "delete" options to take us to relevant config/backend/...
* 'enable services' in gcp needs special handling (some sort of button present in inventory)
* inventory expiry system - add buttons to install, delete, change frequency
* inventory volumes - adjust what we show and how we show it in tables - match cli
* inventory volumes - add buttons for create/mount/delete/grow/detach (show each only for relevant backend, part-fill the form using get vars)
* Agi - after it stopped, on restart, will it remember which instance size was used for a given volume? If not, will it be aware of need to size, and will it ask the monitor to size (like a file on the volume with sizing information)?
* telemetry - report the command run properly somehow
