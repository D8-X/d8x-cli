# D8X CLI

D8X CLI is a tool which helps you to easiliy spin up d8x-trader-backend and
other d8x broker services.

Setup includes provisioning resources on supported cloud providers, configuring servers, deploying swarm cluster and individual services.

For more information check and usage out the `d8x help` command.

## Building From Source

```bash
go build -o d8x ./main.go
sudo mv d8x /usr/bin/d8x
```

## Using A Release

Head to [releases](https://github.com/D8-X/d8x-cli/releases), download and
extract the d8x binary and place it in your `PATH`. 

Note that binary releases are provided only for Linux. To run D8X-CLI on other
platforms you will need to [build it from source](#building-from-source). See
FAQ supported platforms for details.

## Configuration Files
Configuration files are key and the most involved part to setup D8X Perpetuals Backend:
find out how to configure the system in the
[README](README_CONFIG.md).

## FAQ

<details>
  <summary>How do I update the server software images to a new version?</summary>

  You login to the server where your software resides (e.g., the broker-server, or the
  swarm-manager for which you can get the ip with `d8x ip manager`).

  -  Find the hash (sha256:...) of the service you want to update by navigating to the root of the github repository, click on packages (or [here](https://github.com/orgs/D8-X/packages)), choose the package and version you want to update and the hash is displayed on the top. For example, choose "trader-main" and click the relevant version for the main broker services.
  -  Find the name of the service via `docker service ls`
  -  Now you can update the backend-service application by using the docker update command. For example:
  
  ```
  docker service update --image "ghcr.io/d8-x/d8x-trader-main:dev@sha256:aea8e56d6077c733a1d553b4291149712c022b8bd72571d2a852a5478e1ec559" stack_api
  ```
</details>

<details>
  <summary>Supported platforms</summary>

  D8X-CLI is tested and runs natively on Linux. MacOS might work, but you will
  need to manually install ansible and terraform on your system.

  D8X-CLI is not tested on Windows and will most probably not work, we would
  recommend using WSL2 to run D8X-CLI on Windows.

</details>


