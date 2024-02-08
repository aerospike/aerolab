# TODO for 7.4.0 to be ready

* UI
  * make cluster-delete and client-delete inventory functions smarter - make them combined nodes for each cluster and execute as such - single delete per clusterName/clientName
  * implement TODO buttons in index.js
    * ensure buttons are smart to combine calls for each cluster set of nodes into a single call, filling NodeNo accordingly
  * add `attach` button for every nodes in cluster and client views (new last column) where node is running (otherwise disable button)
  * add the following buttons to AGI listing (new last column): status, details, change-label, run ingest, connect, get share link (auth token URL copy to clipboard)

* AGI Improvements
  * Agi - after it stopped, on restart, will it remember which instance size was used for a given volume? If not, will it be aware of need to size, and will it ask the monitor to size (like a file on the volume with sizing information)?
  * Agi - consider adding graphs for security.c log message aggregations (like warnings/errors dashboard)

* Other
  * telemetry - report the command run properly somehow
