# [v7.6.0](https://github.com/aerospike/aerolab/releases/tag/7.6.0)

_Release Date: April 8, 2024_

**Release Notes:**
* TODO: inventory webui view: always show current backend in the top
* TODO: add asbench ui to aerolab webui
* TODO: add eksctl bootstrap option to aerolab as a command
* TODO: add option to specify to inventory multiple regions (with a given list) in aws
* TODO: add option to store ssh keys centrally in cloud for sharing clusters with other users (in bucket/secret manager/etc)
* TODO: make instance-types webui option a dropdown in cloud backends
* TODO: add getting-started tooltip on first use (option to cancel and run again)
* TODO: top-right jobs list - add "show all user jobs" switch, add username/email info to each job
* TODO: add support for special owner/user header which will define the user running this (so it can be set by authenticating proxy)
* TODO: change weblog path: ./weblog/user-owner/items.log
* TODO: file choice for upload form type: user to select local path - upload via aerolab or directly (key share?)
  * TODO: disable local-path-explorer if either connected to using non-local-address (outsider) or a proxy header is set (via proxy)
* TODO: implement "simple" mode which will have the list of options greatly reduced, present "simple/full" slider
  * TODO: definitions should be a list of items, like aerolab config defaults without the values
  * TODO: should have a sane default selection (support team will have a tuned selection)
  * TODO: user should be able to specify their own list as either replacement, addition or removal of items
  * TODO: add option to disable full mode view option toggle altogether
* TODO: add aerolab HA option
  * TODO: support storing inventory cache in a location other than local disk (aerospike DB?)
  * TODO: support each aerolab generating it's own NODE-ID and storing it in a distributed DB
    * TODO: highest node-id wins and is the one updating the inventory, all others read the cache only
      * TODO: check who is highest every time reading from cache
    * TODO: each node inserts it's nodeid into a nodes set with a 30-second timeout on record, every 10 seconds
      * TODO: this way if a node is not available after 30 seconds, it's entry expires and another is chosen
* TODO: revisit certificate handling, specifically for AGI and AGI-monitor, to allow for shipping a signed cert and performing cert validation
* TODO: TBD - find a way to open firewall to allow access by all authenticated users? how do we handle firewall issues,
  especially with http or agi endpoints? are they personal, with option to add auth to agi and open to the world?
  * TODO: consider tailscale integration(?)
