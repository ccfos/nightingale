# conntrack

运维老鸟应该会遇到 conntrack table full 的报错吧，这个插件就是用于监控 conntrack 的情况，forked from `telegraf/conntrack`

## Measurements & Fields

- conntrack
    - ip_conntrack_count (int, count): the number of entries in the conntrack table
    - ip_conntrack_max (int, size): the max capacity of the conntrack table

## 告警

可以配置一条这样的告警规则 `conntrack_ip_conntrack_count / ip_conntrack_max > 0.8`