# nvidia_smi

该采集插件的原理，就是读取 nvidia-smi 的内容输出，转换为监控数据上报。是把 [nvidia_gpu_exporter](https://github.com/utkuozdemir/nvidia_gpu_exporter) 的代码给集成过来了。

## Configuration

配置文件在 `conf/input.nvidia_smi/nvidia_smi.toml`

```toml
# # collect interval
# interval = 15

# 下面这个配置是最重要的配置，如果要采集 nvidia-smi 的信息，就打开下面的配置，
# 给出 nvidia-smi 命令的路径，最好是给绝对路径
# 相当于让 Categraf 执行本机的 nvidia-smi 命令，获取本机 GPU 的状态信息
# exec local command
# nvidia_smi_command = "nvidia-smi"

# 如果想远程方式采集远端机器的 GPU 状态信息，可以使用 ssh 命令，登录远端机器
# 在远端机器执行 nvidia-smi 的命令输出，通常 Categraf 是部署在每个物理机上的
# 所以，ssh 这种方式，理论上用不到
# exec remote command
# nvidia_smi_command = "ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null SSH_USER@SSH_HOST nvidia-smi"

# Comma-separated list of the query fields.
# You can find out possible fields by running `nvidia-smi --help-query-gpus`.
# The value `AUTO` will automatically detect the fields to query.
query_field_names = "AUTO"
```

## TODO

GPU 卡已经关注哪些监控指标，缺少监控大盘JSON和告警规则JSON，欢迎大家 PR