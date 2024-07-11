# docker

forked from telegraf/inputs.docker

## change

1. Using `container_id` as label not field
1. Some metrics have been deleted

## 容器ID标签

通过下面两个配置来控制 container_id 这个标签：

```ini
container_id_label_enable = true
container_id_label_short_style = false
```

默认 container_id_label_enable 设置为 true，表示启用，即会把容器ID放到标签里，container_id_label_short_style 是短格式，容器ID很长，如果把 short_style 设置为 true，就会只截取前面12位

## 权限问题

Categraf 最好是用 root 账号来运行，否则，请求 docker.sock 可能会遇到权限问题，需要把 Categraf 的运行账号，加到 docker group 中，假设 Categraf 使用 categraf 账号运行：

```
sudo usermod -aG docker categraf
```

## 运行在容器里

如果 Categraf 运行在容器中，docker 的 unix socket 就需要挂到 Categraf 的容器里，比如通过 `-v /var/run/docker.sock:/var/run/docker.sock` 这样的参数来启动 Categraf 的容器。如果是在 compose 环境下，也可以在 docker compose 配置中加上 volume 的配置：

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock
```

## 停用该插件

- 方法一：把 `input.docker` 目录改个别的名字，不用 `input.` 打头
- 方法二：docker.toml 中的 endpoint 配置留空