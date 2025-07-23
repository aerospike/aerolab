# Configuring docker and podman for Aerolab

1. Install docker or podman on your machine using one of the below methods:
   * Linux/Windows/MacOS: [Docker Desktop](https://www.docker.com/products/docker-desktop/)
   * Linux/Windows/MacOS: [Podman Desktop](https://podman-desktop.io/downloads/)
   * Linux: Native Docker
   * Linux: Native Podman

2. Start Docker or Podman on your machine, once it is installed, and follow any required setup instructions.
   * If installing Podman Desktop on Windows, either WSL2 or HyperV may be selected. Both methods will work.

3. (Podman only) Enable Docker Compatibility mode using one of the below methods:
   * Podman Desktop: In `Settings->Preferences` select `Docker Compatibility` option menu, and enable it.
   * Podman on Linux: typically you will need to install the `podman-docker` package and enable the podman docker service, like so: `systemctl enable --now podman.socket`. You may need to then set the `DOCKER_HOST=` environment variable accordingly

4. (Docker/Podman Desktop on MacOS/Linux only) Adjust the CPU and RAM resources as needed:
   * Podman Desktop: Navigate to `Settings->Resources` and hit the `settings` cog under the Virtual Machine (where is shows CPU and RAM utilisation). Adjust CPUs, RAM and Disk as needed
   * Docker Desktop: Click the `Preferences` option in the docker tray-icon. Configure the required disk, RAM and CPU resources.

5. (Podman Desktop on Windows only) Configure docker cli
   * Download the zip file from [https://download.docker.com/win/static/stable/x86_64/](https://download.docker.com/win/static/stable/x86_64/)
   * Create a folder called `Aerolab` in your home directory and unzip the `docker.exe` into that folder
   * In powershell, run the following command to update the `PATH` variable: `[Environment]::SetEnvironmentVariable("Path", "$env:Path;$HOME\Aerolab", "User")`
   * Close powershell and reopen in

6. Test to see that docker is running: run `docker version` at the command line.

7. Install aerolab as per the GETTING_STARTED guide for your OS
   * If installing on windows, feel free to unzip the aerolab windows version as `aerolab.exe` into the same `Aerolab` folder and simply start using it from Powershell.

8. Configure aerolab

```
aerolab config backend -t docker
aerolab inventory list
```

9. Optionally upgrade aerolab to latest: `aerolab upgrade --edge`
