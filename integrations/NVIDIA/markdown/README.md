Categraf采集NVIDIA GPU支持两种方式: nvidia_smi和DCGM对应的采集插件是input.nvidia_smi和input.dcgm 需要注意的是dcgm需要安装categraf的with-cgo-plugin的版本才可以正常使用。
# nvidia_smi

nvidia_smi该采集插件的原理，就是读取 nvidia-smi 命令的内容输出，转换为Prometheus格式的监控数据上报给Nightingale夜莺。

是对 [nvidia_gpu_exporter](https://github.com/utkuozdemir/nvidia_gpu_exporter) 代码的集成。

## 采集配置

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
# You can find out possible fields by running `nvidia-smi --help-query-gpu`.
# The value `AUTO` will automatically detect the fields to query.
query_field_names = "AUTO"
```

# DCGM

DCGM 采集插件是fork dcgm-exporter，插件是与nvidia-dcgm交互获取数据, 所以需要先安装nvidia-dcgm服务. 如果是ubuntu系列的os,可以通过 apt-get install -y datacenter-gpu-manager=1:3.3.5(理论上大于这个版本且低于4.0.0的版本也可以), 注意这里的版本号, 不要搞错。 如果是centos, 可以在这里下载: https://developer.download.nvidia.com/compute/cuda/repos/rhel8/x86_64/
nvidia-dcgm服务安装完成后，通过systemctl start nvidia-dcgm.service 启动服务，通过systemctl status nvidia-dcgm.service来查看服务状态，服务处于active 再进行下一步的配置。


## 采集配置
请确保 conf/input.dcgm/目录下包含了 1.x-compatibility-metrics.csv default-counters.csv dcp-metrics-included.csv 这3个文件。
```toml
[[instances]]
# 指定使用的指标定义文件, 一般使用 default-counters.csv就够了，也可以尝试用其他两个csv文件
# path to the file, that contains the DCGM fields to collect
  collectors = "conf/input.dcgm/default-counters.csv"

# 是否是K8s环境，设置为true会附件Pod的信息
# Enable kubernetes mapping metrics to kubernetes pods
# kubernetes=false

# 指标中是否附加 gpu id 作为一个标签
# Choose Type of GPU ID to use to map kubernetes resources to pods. Possible values: "uid", "device-name"
# kubernetes-gpu-id-type = "uid"

# 是否使用 1.x 的ns
# Use old 1.x namespace
# use-old-namespace = false

# 支持的选项是f g i 
# f: FlexKey 如果MIG被禁用，则监控所有GPU；如果MIG被启用，则监控所有GPU实例
# g: MajorKey 监控top-level entities：GPU或NvSwitches或CPU 
# i: MinorKey 监控sub-level entities: GPU实例/NvLinks/CPU核心 - 如果MIG被禁用，则不能指定该选项
  cpu-devices = "f"

# 与cpu-devices的选项一样
# gpu devices
  devices = "f"

# 与cpu-devices的选项一样
  switch-devices = "f"

# 使用ConfigMap 
# ConfigMap <NAMESPACE>:<NAME> for metric data
  configmap-data = "none"

# 这里就是前置依赖的nvidia-dcgm服务, 如果是本机采集，则使用localhost:5555 ,如果是远程采集，则使用远端IP:5555
# Connect to remote hostengine at <HOST>:<PORT>
  remote-hostengine-info = "localhost:5555"

# 允许用户在没有实际GPU硬件的环境中模拟GPU, 仅用于测试
# Accept GPUs that are fake, for testing purposes only
# fake-gpus = false

# 将GPU型号名称中的每个空格替换为破折号，确保标识符连续且无空格。 
# Replaces every blank space in the GPU model name with a dash, ensuring a continuous, space-free identifier.
# replace-blanks-in-model-name = false
```