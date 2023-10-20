# Using direnv to manage multiple aerolab projects

DirEnv can be used to reasily manage and isolate multiple aerolab projects with their own default configurations and backends. This document shows basics of such configuration.

## DirEnv Project

The [Direnv Website](https://direnv.net/) has multiple examples of usage and installation.

## Install DirEnv

Run the following to install DirEnv:

```
curl -sfL https://direnv.net/install.sh | bash
```

After this to enable DirEnv execute the below (depending on the shell you are using), and then restart your shell/terminal:

Shell | Command
--- | ---
bash | `echo 'eval "$(direnv hook bash)"' >> ~/.bashrc`
zsh | `echo 'eval "$(direnv hook zsh)"' >> ~/.zshrc`

For other shells, see [the official documentation](https://direnv.net/docs/hook.html)

## Configure

### Configure a project directory for general AWS use

```
mkdir -p ~/aerolab/aws/keys ~/aerolab/aws/features ~/aerolab/aws/chdir
cd ~/aerolab/aws
echo export AEROLAB_CONFIG_FILE=~/aerolab/aws/conf > .envrc
direnv allow
aerolab config backend -t aws -r us-east-1 -p ~/aerolab/aws/keys
aerolab config defaults -k '*.ChDir' -v ~/aerolab/aws/chdir
aerolab config defaults -k '*.FeaturesFilePath' -v ~/aerolab/aws/features
```

### Configure a project directory for general GCP use

```
mkdir -p ~/aerolab/gcp/keys ~/aerolab/gcp/features ~/aerolab/gcp/chdir
cd ~/aerolab/gcp
echo export AEROLAB_CONFIG_FILE=~/aerolab/gcp/conf > .envrc
direnv allow
aerolab config backend -t gcp -o my-gcp-project-name -p ~/aerolab/gcp/keys
aerolab config defaults -k '*.ChDir' -v ~/aerolab/gcp/chdir
aerolab config defaults -k '*.FeaturesFilePath' -v ~/aerolab/gcp/features
```

### Configure a project directory for general docker use use

```
mkdir -p ~/aerolab/docker/keys ~/aerolab/docker/features ~/aerolab/docker/chdir
cd ~/aerolab/docker
echo export AEROLAB_CONFIG_FILE=~/aerolab/docker/conf > .envrc
direnv allow
aerolab config backend -t docker
aerolab config defaults -k '*.ChDir' -v ~/aerolab/docker/chdir
aerolab config defaults -k '*.FeaturesFilePath' -v ~/aerolab/docker/features
```

## Usage

Just `cd` into one of the directories (`~/aerolab/docker|aws|gcp`) and DirEnv will apply the environment to allow Aerolab to use that specific directory as configured. Usage is as simple as entering the directory and executing AeroLab commands from there.

## Example for specific project, setting other defaults

For example, using the gcp backend, but setting specific defaults, like Zone, in an isolated location.

```
mkdir -p ~/aerolab/gcp-us-central1-a/keys ~/aerolab/gcp-us-central1-a/features ~/aerolab/gcp-us-central1-a/chdir
cd ~/aerolab/gcp-us-central1-a
echo export AEROLAB_CONFIG_FILE=~/aerolab/gcp-us-central1-a/conf > .envrc
direnv allow
aerolab config backend -t gcp -o my-gcp-project-name -p ~/aerolab/gcp-us-central1-a/keys
aerolab config defaults -k '*.ChDir' -v ~/aerolab/gcp-us-central1-a/chdir
aerolab config defaults -k '*.FeaturesFilePath' -v ~/aerolab/gcp-us-central1-a/features
aerolab config defaults -v '*.Gcp.Zone' -v us-central1-a
```

From now on, just `cd` into `~/aerolab/gcp-us-central1-a` and executing aerolab from that directory will result in this particular config being applied.

## Notes

If using a common location for the `ChDir` for temporary files and one for features file path, same path can be applied everywhere and does not need to be project specific.
