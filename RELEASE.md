### AeroLab v8

AeroLab v8 is a complete refactor of the AeroLab codebase with all API calls, no shell commands or external tool dependencies, and latest SDKs. AeroLab v8 is a major new version of AeroLab with a new CLI, new backend code, and new features, including Aerospike Cloud support. Most commands are backwards compatible with AeroLab v7, but some have been removed or changed. Notably, `json` outputs for list commands is completely different.

#### Status

This Prerelease Build is missing the `WebUI` feature normally available under the `aerolab webui` command, and it's accompanying `rest-api` command. It is otherwise considered feature complete.

While extensive testing has been performed, Aerolab v8 is still in beta and may contain bugs. Please report any issues you find and do not use in production.

#### Usage

Please refer to the [README.md](https://github.com/aerospike/aerolab/tree/v8.0.0/docs) for usage instructions and getting started guides.

#### Migration from AeroLab v7

AeroLab v7 volumes, images, firewalls and instances are not automatically visible in v8. To migrate to v8, use the `config migrate` and `inventory migrate` commands. See [docs/migration-guide.md](https://github.com/aerospike/aerolab/tree/v8.0.0/docs/migration-guide.md) for details. Note that the migration commands are safe to run multiple times and will not corrupt existing v7 configuration or access. In essence, after migration completes, both v7 and v8 can be used side by side for the existing migrated resources.
