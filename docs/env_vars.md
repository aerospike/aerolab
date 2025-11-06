# Environment Variables

AeroLab supports the following environment variables:

Env Variable | Possible values | Description
--- | --- | ---
AEROLAB_HOME | FILEPATH | If set, will override the default ~/.aerolab home directory
AEROLAB_LOG_LEVEL | 0-6 | 0=NONE,1=CRITICAL,2=ERROR,3=WARN,4=INFO,5=DEBUG,6=DETAIL
AEROLAB_PROJECT | PROJECTNAME | Aerolab v8 has a notion of projects; setting this will make it work on resources other than in the 'default' aerolab project
AEROLAB_DISABLE_UPGRADE_CHECK | true | If set to a non-empty value, aerolab will not check if upgrades are available
AEROLAB_TELEMETRY_DISABLE | true | If set to a non-empty value, telemetry will be disabled
AEROLAB_CONFIG_FILE | FILEPATH | If set, aerolab will read the given defaults config file instead of $AEROLAB_HOME/conf
AEROLAB_NONINTERACTIVE | true | If set to a non-empty value, aerolab will not ask for confirmation or choices at any point
