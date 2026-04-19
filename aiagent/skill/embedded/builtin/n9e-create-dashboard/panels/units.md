# 全部单位值 (standardOptions.util)

## Misc
`none` `sishort` `count`

## 百分比
`percent` (0-100) | `percentUnit` (0.0-1.0)

## 数据量
`bitsSI` `bytesSI` `bitsIEC` `bytesIEC` `kibibytes` `kilobytes` `mebibytes` `megabytes` `gibibytes` `gigabytes` `tebibytes` `terabytes` `pebibytes` `petabytes`

## 数据速率
`packetsSec` `bitsSecSI` `bytesSecSI` `bitsSecIEC` `bytesSecIEC` `kibibytesSec` `kibibitsSec` `kilobytesSec` `kilobitsSec` `mebibytesSec` `mebibitsSec` `megabytesSec` `megabitsSec` `gibibytesSec` `gibibitsSec` `gigabytesSec` `gigabitsSec` `tebibytesSec` `tebibitsSec` `terabytesSec` `terabitsSec` `pebibytesSec` `pebibitsSec` `petabytesSec` `petabitsSec`

## 吞吐量
`cps` (counts/s) `ops` (ops/s) `reqps` (requests/s) `rps` (reads/s) `wps` (writes/s) `iops` (I/O ops/s) `eps` (events/s) `mps` (messages/s) `recps` (records/s) `rowsps` (rows/s)
`cpm` (counts/min) `opm` (ops/min) `reqpm` (requests/min) `rpm` (reads/min) `wpm` (writes/min) `epm` (events/min) `mpm` (messages/min) `recpm` (records/min) `rowspm` (rows/min)

## 时间
`seconds` `milliseconds` `microseconds` `nanoseconds` `datetimeSeconds` `datetimeMilliseconds` `humantimeSeconds` `humantimeMilliseconds`

## 温度
`celsius` (°C) `fahrenheit` (°F) `kelvin` (K)

## 长度
`millimeter` `meter` `kilometer` `inch` `foot` `mile`

## 能量
`dBm` (Decibel-milliwatt)

## 聚合函数 (calc)

用于 stat、table、barGauge、pie、gauge、hexbin 等面板的 `custom.calc` 字段：

`lastNotNull` 最后一个非空值（默认）| `last` 最后一个值 | `firstNotNull` 第一个非空值 | `first` 第一个值 | `min` 最小值 | `max` 最大值 | `avg` 平均值 | `sum` 求和 | `count` 计数

table 面板额外支持：`origin` 原始值
