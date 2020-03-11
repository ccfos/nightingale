
import { prefixCls as gPrefixCls } from '../config';

export const prefixCls = `${gPrefixCls}-dashboard`;
export const metricMap: { [index: string]: any } = {
  CPU: {
    key: 'CPU',
    alias: 'CPU',
    dynamic: true,
    filter: { type: 'prefix', value: 'cpu.' },
  },
  MEM: {
    key: 'MEM',
    alias: '内存',
    dynamic: true,
    filter: { type: 'prefix', value: 'mem.' },
  },
  DISK: {
    key: 'DISK',
    alias: '磁盘',
    dynamic: true,
    filter: { type: 'prefix', value: 'disk.' },
  },
  NET: {
    key: 'NET',
    alias: '网络',
    dynamic: true,
    filter: { type: 'prefix', value: 'net.' },
  },
  SYS: {
    key: 'SYS',
    alias: '系统',
    dynamic: true,
    filter: { type: 'prefix', value: 'sys.' },
  },
  PROC: {
    key: 'PROC',
    alias: '进程',
    dynamic: true,
    filter: { type: 'prefix', value: 'proc.' },
  },
  LOG: {
    key: 'LOG',
    alias: '日志',
    dynamic: true,
    filter: { type: 'prefix', value: 'log.,LOG.' },
  },
};

export const metricsMeta: { [index: string]: any } = {
  'cpu.idle': {
    meaning: '全局CPU空闲率',
    unit: '%',
  },
  'cpu.util': {
    meaning: '全局CPU利用率',
    unit: '%',
  },
  'cpu.sys': {
    meaning: '全局内核态cpu时间比例',
    unit: '%',
  },
  'cpu.user': {
    meaning: '全局用户态cpu时间比例(nice值为负不统计)',
    unit: '%',
  },
  'cpu.irq': {
    meaning: '全局硬中断CPU时间占比',
    unit: '%',
  },
  'cpu.softirq': {
    meaning: '全局软中断CPU时间占比',
    unit: '%',
  },
  'cpu.steal': {
    meaning: '等待Hipervisor处理其他虚拟核的时间占比',
    unit: '%',
  },
  'cpu.iowait': {
    meaning: '等待I/O的CPU时间占比',
    unit: '%',
  },
  'cpu.loadavg.1': {
    meaning: '1分钟内平均活动进程数',
    unit: '个',
  },
  'cpu.loadavg.5': {
    meaning: '5分钟内平均活动进程数',
    unit: '个',
  },
  'cpu.loadavg.15': {
    meaning: '15分钟内平均活动进程数',
    unit: '个',
  },
  'mem.bytes.total': {
    meaning: '内存总大小',
    unit: 'Byte',
  },
  'mem.bytes.cached': {
    meaning: '高速缓存占用的内存大小',
    unit: 'Byte',
  },
  'mem.bytes.buffers': {
    meaning: '文件缓冲占用的内存大小',
    unit: 'Byte',
  },
  'mem.bytes.free': {
    meaning: '可用内存大小',
    unit: 'Byte',
  },
  'mem.bytes.used': {
    meaning: '已用内存大小',
    unit: 'Byte',
  },
  'mem.bytes.used.percent': {
    meaning: '已用内存占比',
    unit: '%',
  },
  'mem.swap.bytes.total': {
    meaning: 'swap总大小',
    unit: 'Byte',
  },
  'mem.swap.bytes.free': {
    meaning: '空闲swap大小',
    unit: 'Byte',
  },
  'mem.swap.bytes.used': {
    meaning: '已用swap大小',
    unit: 'Byte',
  },
  'mem.swap.bytes.used.percent': {
    meaning: '已用swap占比',
    unit: '%',
  },
  'disk.cap.bytes.total': {
    meaning: '所有分区容量大小之和',
    unit: 'Byte',
  },
  'disk.cap.bytes.free': {
    meaning: '所有分区空闲大小之和',
    unit: 'Byte',
  },
  'disk.cap.bytes.used': {
    meaning: '所有分区已用大小之和',
    unit: 'Byte',
  },
  'disk.cap.bytes.used.percent': {
    meaning: '所有分区已用大小占比',
    unit: '%',
  },
  'disk.bytes.total': {
    meaning: '某分区大小',
    unit: 'Byte',
  },
  'disk.bytes.free': {
    meaning: '某分区余量大小',
    unit: 'Byte',
  },
  'disk.bytes.used': {
    meaning: '某分区用量大小',
    unit: 'Byte',
  },
  'disk.bytes.used.percent': {
    meaning: '某分区用量占比',
    unit: '%',
  },
  'disk.inodes.total': {
    meaning: '某分区inode总数量',
    unit: '个',
  },
  'disk.inodes.free': {
    meaning: '某分区inode余量',
    unit: '个',
  },
  'disk.inodes.used': {
    meaning: '某分区inode用量',
    unit: '个',
  },
  'disk.inodes.used.percent': {
    meaning: '某分区inode用量占比',
    unit: '%',
  },
  'disk.io.util': {
    meaning: '某硬盘I/O利用率',
    unit: '%',
  },
  'disk.io.svctm': {
    meaning: '每次I/O服务时间',
    unit: 'ms',
  },
  'disk.io.await': {
    meaning: '每次I/O处理时间：等待+服务',
    unit: 'ms',
  },
  'disk.io.avgrq_sz': {
    meaning: '单次I/O平均大小',
    unit: '扇区数',
  },
  'disk.io.avgqu_sz': {
    meaning: '平均队列长度',
    unit: '个',
  },
  'disk.io.read.request': {
    meaning: '某硬盘每秒读请求数量',
    unit: '次/s',
  },
  'disk.io.write.request': {
    meaning: '某硬盘每秒写请求数量',
    unit: '次/s',
  },
  'disk.io.read.bytes': {
    meaning: '某硬盘每秒读取字节数',
    unit: 'Byte',
  },
  'disk.io.write.bytes': {
    meaning: '某硬盘每秒写入字节数',
    unit: 'Byte',
  },
  'disk.rw.error': {
    meaning: '某个分区读写探测，是否报错',
    unit: '错误码，0表示没报错',
  },
  'net.in.bits': {
    meaning: '某块网卡的入向流量',
    unit: 'bits/s',
  },
  'net.out.bits': {
    meaning: '某块网卡的出向流量',
    unit: 'bits/s',
  },
  'net.in.dropped': {
    meaning: '某块网卡的入向丢包量',
    unit: 'Packet/s',
  },
  'net.out.dropped': {
    meaning: '某块网卡的出向丢包量',
    unit: 'Packet/s',
  },
  'net.in.pps': {
    meaning: '某块网卡的入向包量',
    unit: 'Packet/s',
  },
  'net.out.pps': {
    meaning: '某块网卡的出向包量',
    unit: 'Packet/s',
  },
  'net.in.errs': {
    meaning: '某块网卡的入向错误包量',
    unit: 'Packet/s',
  },
  'net.out.errs': {
    meaning: '某块网卡的出向错误包量',
    unit: 'Packet/s',
  },
  'net.in.percent': {
    meaning: '某块网卡的已使用的接收带宽百分比',
    unit: '%',
  },
  'net.out.percent': {
    meaning: '某块网卡的已使用的发送带宽百分比',
    unit: '%',
  },
  'net.bandwidth.mbits': {
    meaning: '某块网卡的带宽',
    unit: 'mbits',
  },
  'net.bandwidth.mbits.total': {
    meaning: '所有网卡的带宽之和',
    unit: 'mbits',
  },
  'net.in.bits.total': {
    meaning: '所有网卡入向总流量',
    unit: 'bits/s',
  },
  'net.out.bits.total': {
    meaning: '所有网卡出向总流量',
    unit: 'bits/s',
  },
  'net.in.bits.total.percent': {
    meaning: '所有网卡入向总流量占比',
    unit: '%',
  },
  'net.out.bits.total.percent': {
    meaning: '所有网卡出向总流量占比',
    unit: '%',
  },
  'net.sockets.used': {
    meaning: '已使用的所有协议的socket数量(协议包括tcp、udp等)',
    unit: '个',
  },
  'net.sockets.tcp.inuse': {
    meaning: '正在使用的tcp socket数量',
    unit: '个',
  },
  'net.sockets.tcp.timewait': {
    meaning: '等待关闭的tcp连接数',
    unit: '个',
  },
  'sys.fs.files.used': {
    meaning: '系统已分配文件句柄数',
    unit: '个',
  },
  'sys.fs.files.free': {
    meaning: '系统剩余文件句柄数',
    unit: '个',
  },
  'sys.fs.files.max': {
    meaning: '系统最大文件句柄数',
    unit: '个',
  },
  'sys.fs.files.used.percent': {
    meaning: '系统文件句柄使用率',
    unit: '%',
  },
  'sys.ps.process.total': {
    meaning: '系统进程总数',
    unit: '个',
  },
  'sys.ps.entity.total': {
    meaning: '系统调度单元总数',
    unit: '个',
  },
  'sys.ntp.offset.ms': {
    meaning: '系统时间偏移量',
    unit: 'ms',
  },
  'sys.net.netfilter.nf_conntrack_max': {
    meaning: 'conntrack最大值',
    unit: '个',
  },
  'sys.net.netfilter.nf_conntrack_count': {
    meaning: 'conntrack用量',
    unit: '个',
  },
  'sys.net.netfilter.nf_conntrack_count.percent': {
    meaning: 'conntrack用量占比',
    unit: '%',
  },
};

export const baseMetrics = [
  'cpu.util',
  'cpu.loadavg.1',
  'mem.bytes.used.percent',
  'disk.bytes.used.percent',
];
