# TODO for 7.4.0 to be ready

```
// TODO for cache updater, add MinimumInterval option to ensure we do not do more than 1 refresh per MinimumInterval
/*
we should have a nextRun time.Time updated as now().Add(RefreshInterval)
we should have a lastRun time.Time updated as now()
we should sleep a second minimumInterval and check if nextRun is before now(). if so, do a run. if not, sleep minimumInterval
a forced run() should update nextRun to be now().add(minimumInterval-(now()-lastRun)) ... and somehow wait for the action to complete
we should have a runNow() for backend/defaults changes, which WILL run now (and update nextRun to be in minimumInterval)
 ... we should really be having a way to update the inventory tables when we update the cache ...
*/
```

* inventory volumes - adjust what we show and how we show it in tables - match cli
* inventory volumes - add buttons for create/mount/delete/grow/detach (show each only for relevant backend, part-fill the form using get vars)

* add inventory clusters
* add inventory clients
* add inventory agi - special page joining AGI volumes and AGI clusters

* Agi - after it stopped, on restart, will it remember which instance size was used for a given volume? If not, will it be aware of need to size, and will it ask the monitor to size (like a file on the volume with sizing information)?
* telemetry - report the command run properly somehow

-=-=-=-=-

other items (future)

* gh issue: add links in eg security groups so that we can be taken straight to gcp/aws console section with this
* aerolab bootstrap eksctl container
* aerolab - add asbench UI
* gh issue - recipe option like jupiter notebooks
