# docker

The Docker input uses the Docker API and emits `docker_*` metrics. The bundled
`Docker Dashboard` was validated against these metrics, so this input is
required for that dashboard.

cAdvisor is also supported, but it emits `container_*` metrics and requires the
dashboard under the separate `cAdvisor` integration.

## Configuration

```toml
[[instances]]
endpoint = "unix:///var/run/docker.sock"
gather_services = false
gather_extend_memstats = false
container_id_label_enable = true
container_id_label_short_style = false
timeout = "5s"
total_include = ["cpu", "blkio", "network"]
```

Run `./categraf --test --inputs docker` and confirm `docker_up`,
`docker_n_containers_running`, and `docker_container_cpu_usage_percent`.

## change

1. Using `container_id` as label not field
1. Some metrics have been deleted

## Container ID Label

The following two options control the container_id label:

```ini
container_id_label_enable = true
container_id_label_short_style = false
```

By default container_id_label_enable is set to true, which means the container ID is added as a label. container_id_label_short_style controls the short format: container IDs are long, and if short_style is set to true, only the first 12 characters are kept.

## Permissions

It is best to run Categraf as the root account; otherwise requests to docker.sock may run into permission problems. In that case, add the account Categraf runs as to the docker group. Assuming Categraf runs as the categraf account:

```
sudo usermod -aG docker categraf
```

## Running in a Container

If Categraf runs inside a container, the docker unix socket needs to be mounted into the Categraf container, e.g. start the Categraf container with a parameter like `-v /var/run/docker.sock:/var/run/docker.sock`. In a compose environment, you can also add the volume configuration to the docker compose file:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock
```

## Disabling This Plugin

- Option 1: rename the `input.docker` directory to something that does not start with `input.`
- Option 2: leave the endpoint setting empty in docker.toml
