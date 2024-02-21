# TODO for 7.4.0 to be ready

* WebUI: AGI Details button (provide a nice way to show mapping of original->new names and other details, including errors and agi stack logs)
  * consider hosting this on agi-proxy as a webpage instead of webui
* Agi - after it terminated with efs/vol, on restart, will it remember which instance size was used for a given volume? If not, will it be aware of need to size, and will it ask the monitor to size (like a file on the volume with sizing information)?
* Test GCP,AWS,Docker all buttons in inventory system in webui

* AGI BUG (if record too big, stop including some stuff):
```
Feb 20 22:06:19 agi-1 aerolab[4710]: 2024/02/20 22:06:19 +0000 INFO CollectinfoProcessor detail file:/opt/agi/files/collectinfo/x1_s3216bdc_collectInfo.tgz (size:1.94 MiB) (nodeID:) (renameAttempt:true renamed:false processAttempt:false processed:false) (originalName:) errors:
Feb 20 22:06:19 agi-1 aerolab[4710]:         aerospike.PutBins: ResultCode: RECORD_TOO_BIG, Iteration: 0, InDoubt: true, Node: BB979FF7AC1F302 127.0.0.1:3000: Record too big
```
