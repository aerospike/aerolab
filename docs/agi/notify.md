# AGI notification support

## HTTPS

AeroLab supports sending notifications for instance events to a HTTPS endpoint in a json format.

To setup notifications at AGI creation, the following parameters may be configured for `aerolab agi create` command:

Parameter | Description
--- | ---
`--notify-web-endpoint=` | http(s) URL to contact with a notification
`--notify-web-header=` | a header to set for notification; for example to use Authorization tokens; format: Name=value
`--notify-web-abort-on-fail` | if set, ingest will be aborted if the notification system receives an error response
`--notify-web-abort-code=` | set to status codes on which to abort the operation
`--notify-web-ignore-cert` | set to make https calls ignore invalid server certificate

Only the first parameter is required.

### Example notification output

```json
{
    "Event": "SERVICE_DOWN",
    "EventDetail": "",
    "IngestStatus": {
        "Ingest": {
            "Running": true,
            "CompleteSteps": {
                "Init": false,
                "Download": false,
                "Unpack": false,
                "PreProcess": false,
                "ProcessLogs": false,
                "ProcessCollectInfo": false,
                "CriticalError": "",
                "DownloadStartTime": "0001-01-01T00:00:00Z",
                "DownloadEndTime": "0001-01-01T00:00:00Z",
                "ProcessLogsStartTime": "0001-01-01T00:00:00Z",
                "ProcessLogsEndTime": "0001-01-01T00:00:00Z"
            },
            "DownloaderCompletePct": 0,
            "DownloaderTotalSize": 0,
            "DownloaderCompleteSize": 0,
            "LogProcessorCompletePct": 0,
            "LogProcessorTotalSize": 0,
            "LogProcessorCompleteSize": 0,
            "Errors": null
        },
        "AerospikeRunning": false,
        "PluginRunning": true,
        "GrafanaHelperRunning": true
    }
}
```

### Event types

```go
AgiEventInitComplete       = "INGEST_STEP_INIT_COMPLETE" // ingest step completed
AgiEventDownloadComplete   = "INGEST_STEP_DOWNLOAD_COMPLETE" // ingest step completed
AgiEventUnpackComplete     = "INGEST_STEP_UNPACK_COMPLETE" // ingest step completed
AgiEventPreProcessComplete = "INGEST_STEP_PREPROCESS_COMPLETE" // ingest step completed
AgiEventProcessComplete    = "INGEST_STEP_PROCESS_COMPLETE" // ingest step completed
AgiEventIngestFinish       = "INGEST_FINISHED" // ingest finished
AgiEventServiceDown        = "SERVICE_DOWN" // a service has gone down, notifies each time a service unexpectedly goes down
AgiEventServiceUp          = "SERVICE_UP" // a service has recovered, notifies every time a service recovers, only if another didn't go down at the same time
AgiEventMaxAge             = "MAX_AGE_REACHED" // power off will be executed in a minute
AgiEventMaxInactive        = "MAX_INACTIVITY_REACHED" // power off will be executed in a minute
AgiEventSpotNoCapacity     = "SPOT_INSTANCE_CAPACITY_SHUTDOWN" // AWS shutting instance down due to capacity demand
```

Only `AgiEventSpotNoCapacity` fills the `EventDetail` part, with the event details provided by AWS.
