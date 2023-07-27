# S.M.A.R.T. 插件

从[telegraf](https://github.com/influxdata/telegraf/blob/master/plugins/inputs/smart/README.md) fork，略作改动

Get metrics using the command line utility `smartctl` for
S.M.A.R.T. (Self-Monitoring, Analysis and Reporting Technology) storage
devices. SMART is a monitoring system included in computer hard disk drives
(HDDs) and solid-state drives (SSDs) that detects and reports on various
indicators of drive reliability, with the intent of enabling the anticipation of
hardware failures.  See smartmontools (<https://www.smartmontools.org/>).

SMART information is separated between different measurements: `smart_device` is
used for general information, while `smart_attribute` stores the detailed
attribute information if `attributes = true` is enabled in the plugin
configuration.

If no devices are specified, the plugin will scan for SMART devices via the
following command:

```sh
smartctl --scan
```

Metrics will be reported from the following `smartctl` command:

```sh
smartctl --info --attributes --health -n <nocheck> --format=brief <device>
```

This plugin supports _smartmontools_ version 5.41 and above, but v. 5.41 and
v. 5.42 might require setting `nocheck`, see the comment in the sample
configuration.  Also, NVMe capabilities were introduced in version 6.5.

To enable SMART on a storage device run:

```sh
smartctl -s on <device>
```

## NVMe vendor specific attributes

For NVMe disk type, plugin can use command line utility `nvme-cli`. It has a
feature to easy access a vendor specific attributes.  This plugin supports
nmve-cli version 1.5 and above (<https://github.com/linux-nvme/nvme-cli>).  In
case of `nvme-cli` absence NVMe vendor specific metrics will not be obtained.

Vendor specific SMART metrics for NVMe disks may be reported from the following
`nvme` command:

```sh
nvme <vendor> smart-log-add <device>
```

Note that vendor plugins for `nvme-cli` could require different naming
convention and report format.

To see installed plugin extensions, depended on the nvme-cli version, look at
the bottom of:

```sh
nvme help
```

To gather disk vendor id (vid) `id-ctrl` could be used:

```sh
nvme id-ctrl <device>
```

Association between a vid and company can be found there:
<https://pcisig.com/membership/member-companies>.

Devices affiliation to being NVMe or non NVMe will be determined thanks to:

```sh
smartctl --scan
```

and:

```sh
smartctl --scan -d nvme
```


## Configuration

```toml @示例
# Read metrics from storage devices supporting S.M.A.R.T.
[[instances]]
## Optionally specify the path to the smartctl executable
# path_smartctl = "/usr/bin/smartctl"

## Optionally specify the path to the nvme-cli executable
# path_nvme = "/usr/bin/nvme"

## Optionally specify if vendor specific attributes should be propagated for NVMe disk case
## ["auto-on"] - automatically find and enable additional vendor specific disk info
## ["vendor1", "vendor2", ...] - e.g. "Intel" enable additional Intel specific disk info
# enable_extensions = ["auto-on"]

## On most platforms used cli utilities requires root access.
## Setting 'use_sudo' to true will make use of sudo to run smartctl or nvme-cli.
## Sudo must be configured to allow the categraf user to run smartctl or nvme-cli
## Sudo must be configured to allow the categraf user to run smartctl or nvme-cli
## without a password.
use_sudo = true

## Skip checking disks in this power mode. Defaults to
## "standby" to not wake up disks that have stopped rotating.
## See --nocheck in the man pages for smartctl.
## smartctl version 5.41 and 5.42 have faulty detection of
## power mode and might require changing this value to
## "never" depending on your disks.
# nocheck = "standby"

## Gather all returned S.M.A.R.T. attribute metrics and the detailed
## information from each drive into the 'smart_attribute' measurement.
attributes = true

## Optionally specify devices to exclude from reporting if disks auto-discovery is performed.
# excludes = [ "/dev/pass6" ]

## Optionally specify devices and device type, if unset
## a scan (smartctl --scan and smartctl --scan -d nvme) for S.M.A.R.T. devices will be done
## and all found will be included except for the excluded in excludes.
# devices = [ "/dev/ada0 -d atacam", "/dev/nvme0"]
# devices = ["dev/nvme0 -d nvme", "/dev/nvme0"]

## Timeout for the cli command to complete.
timeout = "30s"

## Optionally call smartctl and nvme-cli with a specific concurrency policy.
## By default, smartctl and nvme-cli are called in separate threads (goroutines) to gather disk attributes.
## Some devices (e.g. disks in RAID arrays) may have access limitations that require sequential reading of
## SMART data - one individual array drive at the time. In such case please set this configuration option
## to "sequential" to get readings for all drives.
## valid options: concurrent, sequential
# read_method = "concurrent"
```

## Permissions
采集需要sudo权限

## Metrics

- smart_device:
  - tags:
    - capacity
    - device
    - enabled
    - model
    - serial_no
    - wwn
  - fields:
    - exit_status
    - health_ok
    - media_wearout_indicator
    - percent_lifetime_remain
    - read_error_rate
    - seek_error
    - temp_c
    - udma_crc_errors
    - wear_leveling_count

- smart_attribute:
  - tags:
    - capacity
    - device
    - enabled
    - fail
    - flags
    - id
    - model
    - name
    - serial_no
    - wwn
  - fields:
    - exit_status
    - threshold
    - value
    - worst
    - critical_warning
    - temperature_celsius
    - available_spare
    - available_spare_threshold
    - percentage_used
    - data_units_read
    - data_units_written
    - host_read_commands
    - host_write_commands
    - controller_busy_time
    - power_cycle_count
    - power_on_hours
    - unsafe_shutdowns
    - media_and_data_integrity_errors
    - error_information_log_entries
    - warning_temperature_time
    - critical_temperature_time
    - program_fail_count
    - erase_fail_count
    - wear_leveling_count
    - end_to_end_error_detection_count
    - crc_error_count
    - media_wear_percentage
    - host_reads
    - timed_workload_timer
    - thermal_throttle_status
    - retry_buffer_overflow_count
    - pll_lock_loss_count

### Flags

The interpretation of the tag `flags` is:

- `K` auto-keep
- `C` event count
- `R` error rate
- `S` speed/performance
- `O` updated online
- `P` prefailure warning

### Exit Status

The `exit_status` field captures the exit status of the used cli utilities
command which is defined by a bitmask. For the interpretation of the bitmask see
the man page for smartctl or nvme-cli.

## Device Names

Device names, e.g., `/dev/sda`, are _not persistent_, and may be
subject to change across reboots or system changes. Instead, you can use the
_World Wide Name_ (WWN) or serial number to identify devices. On Linux block
devices can be referenced by the WWN in the following location:
`/dev/disk/by-id/`.

## Troubleshooting

If you expect to see more SMART metrics than this plugin shows, be sure to use a
proper version of smartctl or nvme-cli utility which has the functionality to
gather desired data. Also, check your device capability because not every SMART
metrics are mandatory. For example the number of temperature sensors depends on
the device specification.

If this plugin is not working as expected for your SMART enabled device,
please run these commands and include the output in a bug report:

For non NVMe devices (from smartctl version >= 7.0 this will also return NVMe
devices by default):

```sh
smartctl --scan
```

For NVMe devices:

```sh
smartctl --scan -d nvme
```

Run the following command replacing your configuration setting for NOCHECK and
the DEVICE (name of the device could be taken from the previous command):

```sh
smartctl --info --health --attributes --tolerance=verypermissive --nocheck NOCHECK --format=brief -d DEVICE
```

If you try to gather vendor specific metrics, please provide this command
and replace vendor and device to match your case:

```sh
nvme VENDOR smart-log-add DEVICE
```

If you have specified devices array in configuration file, and categraf only
shows data from one device, you should change the plugin configuration to
sequentially gather disk attributes instead of collecting it in separate threads
(goroutines). To do this find in plugin configuration read_method and change it
to sequential:

```toml
    ## Optionally call smartctl and nvme-cli with a specific concurrency policy.
    ## By default, smartctl and nvme-cli are called in separate threads (goroutines) to gather disk attributes.
    ## Some devices (e.g. disks in RAID arrays) may have access limitations that require sequential reading of
    ## SMART data - one individual array drive at the time. In such case please set this configuration option
    ## to "sequential" to get readings for all drives.
    ## valid options: concurrent, sequential
    read_method = "sequential"
```

## Example Output

```text
smart_device_health_ok agent_hostname=1.2.3.4 device=nvme0 model=INTEL_SSDPE2KX040T8 serial_no=PHLJ830200CH4P0DGN 1
smart_device_temp_c agent_hostname=1.2.3.4 device=nvme0 model=INTEL_SSDPE2KX040T8 serial_no=PHLJ830200CH4P0DGN 53
smart_attribute_program_fail_count agent_hostname=1.2.3.4 device=nvme0 model= name=Program_Fail_Count serial_no=PHLJ830200CH4P0DGN 0
smart_attribute_erase_fail_count agent_hostname=1.2.3.4 device=nvme0 model= name=Erase_Fail_Count serial_no=PHLJ830200CH4P0DGN 0
smart_attribute_wear_leveling_count agent_hostname=1.2.3.4 device=nvme0 model= name=Wear_Leveling_Count serial_no=PHLJ830200CH4P0DGN 34360328200
```
