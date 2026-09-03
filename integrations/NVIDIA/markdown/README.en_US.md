Categraf supports two ways to collect NVIDIA GPU metrics: nvidia_smi and DCGM, provided by the input.nvidia_smi and input.dcgm plugins respectively. Note that dcgm requires the with-cgo-plugin build of categraf to work properly.
# nvidia_smi

The nvidia_smi collection plugin works by reading the output of the nvidia-smi command, converting it into Prometheus-format monitoring data, and reporting it to Nightingale.

It is an integration of the [nvidia_gpu_exporter](https://github.com/utkuozdemir/nvidia_gpu_exporter) code.

## Collection Configuration

The configuration file is `conf/input.nvidia_smi/nvidia_smi.toml`

```toml
# # collect interval
# interval = 15

# The setting below is the most important one. To collect nvidia-smi information,
# uncomment it and provide the path to the nvidia-smi command, preferably an absolute path.
# It makes Categraf execute the local nvidia-smi command to get the status of the local GPUs.
# exec local command
# nvidia_smi_command = "nvidia-smi"

# To collect GPU status from a remote machine, you can use an ssh command to log in
# to the remote host and run nvidia-smi there. Since Categraf is usually deployed
# on every physical machine, this ssh approach is rarely needed in practice.
# exec remote command
# nvidia_smi_command = "ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null SSH_USER@SSH_HOST nvidia-smi"

# Comma-separated list of the query fields.
# You can find out possible fields by running `nvidia-smi --help-query-gpu`.
# The value `AUTO` will automatically detect the fields to query.
query_field_names = "AUTO"
```

# DCGM

The DCGM collection plugin is a fork of dcgm-exporter. It retrieves data by talking to nvidia-dcgm, so the nvidia-dcgm service must be installed first. On Ubuntu-family OSes, you can install it via apt-get install -y datacenter-gpu-manager=1:3.3.5 (in theory any version above this and below 4.0.0 should also work — pay attention to the version number and don't get it wrong). On CentOS, you can download it here: https://developer.download.nvidia.com/compute/cuda/repos/rhel8/x86_64/
After installing the nvidia-dcgm service, start it with systemctl start nvidia-dcgm.service and check its status with systemctl status nvidia-dcgm.service. Proceed with the configuration below only when the service is active.


## Collection Configuration
Make sure the conf/input.dcgm/ directory contains these 3 files: 1.x-compatibility-metrics.csv default-counters.csv dcp-metrics-included.csv.
```toml
[[instances]]
# Specify the metric definition file to use. default-counters.csv is usually enough; you can also try the other two csv files
# path to the file, that contains the DCGM fields to collect
  collectors = "conf/input.dcgm/default-counters.csv"

# Whether this is a K8s environment. Setting it to true attaches Pod information
# Enable kubernetes mapping metrics to kubernetes pods
# kubernetes=false

# Whether to attach the gpu id as a label on the metrics
# Choose Type of GPU ID to use to map kubernetes resources to pods. Possible values: "uid", "device-name"
# kubernetes-gpu-id-type = "uid"

# Whether to use the 1.x namespace
# Use old 1.x namespace
# use-old-namespace = false

# Supported options are f, g, i
# f: FlexKey — if MIG is disabled, monitor all GPUs; if MIG is enabled, monitor all GPU instances
# g: MajorKey — monitor top-level entities: GPUs, NvSwitches, or CPUs
# i: MinorKey — monitor sub-level entities: GPU instances/NvLinks/CPU cores — this option cannot be specified if MIG is disabled
  cpu-devices = "f"

# Same options as cpu-devices
# gpu devices
  devices = "f"

# Same options as cpu-devices
  switch-devices = "f"

# Use a ConfigMap
# ConfigMap <NAMESPACE>:<NAME> for metric data
  configmap-data = "none"

# This is the nvidia-dcgm service mentioned above as a prerequisite. Use localhost:5555 for local collection, or <remote IP>:5555 for remote collection
# Connect to remote hostengine at <HOST>:<PORT>
  remote-hostengine-info = "localhost:5555"

# Allows simulating GPUs in environments without actual GPU hardware, for testing only
# Accept GPUs that are fake, for testing purposes only
# fake-gpus = false

# Replaces every blank space in the GPU model name with a dash, ensuring a continuous, space-free identifier.
# replace-blanks-in-model-name = false
```
