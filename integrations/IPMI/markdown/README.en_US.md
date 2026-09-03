# IPMI plugin
The ipmi plugin was ported from ipmi exporter. It works by running a series of ipmi commands and converting their output into metrics. If ipmi is not set up properly, no metrics can be collected, so please make sure ipmi is configured correctly.

An example configuration of categraf's ipmi plugin:
```toml
# Read metrics from the bare metal servers via freeipmi
[[instances]]
# target specifies whether to collect locally or remotely
#target="localhost"
# The username and password used for collection. Make sure the ipmi commands produce correct output with these credentials;
# a username/password randomly found online will not necessarily work.
#user = "user"
#pass = "1234"

# ipmi protocol version, supports 1.5 and 2.0
#driver = "LAN_2_0"

# specify the privilege level username
#privilege = "user"

## session-timeout, ms
#timeout = 100000

# Supported collectors: bmc, bmc-watchdog, ipmi, chassis, dcmi, sel, sm-lan-mode
# bmc, ipmi, chassis and dcmi are used by default. Keeping the configuration below is recommended for better dashboard display
collectors = [ "bmc", "ipmi", "chassis", "sel", "dcmi"]

# Sensors you don't care about; exclude them by id
#exclude_sensor_ids = [ 2, 29, 32, 50, 52, 55 ]

# If you want to override the built-in commands with customized arguments, modify the following; keeping them commented out is recommended
#[instances.collector_cmd]
#ipmi = "sudo"
#sel = "sudo"
#[instances.default_args]
#ipmi = [ "--bridge-sensors" ]
#[instances.custom_args]
#ipmi = [ "--bridge-sensors" ]
#sel = [ "ipmi-sel" ]
```
