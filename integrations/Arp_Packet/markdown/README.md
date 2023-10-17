# 调整间隔时间
如有诉求对此插件本身的采集间隔时间调整的话就启用,单位为秒
interval = 15

# 获取被监控端设备的网卡名称
可用以下命令获取网卡名称列表
```
ip addr | grep '^[0-9]' |awk -F':' '{print $2}'

 lo
 eth0
 br-153e7f4f0c83
 br-2f302c2a8faa
 br-5ae0cdb82efc
 br-68cba8773a8c
 br-c50ca3122079
 docker0
 br-fd769e4347bd
 veth944ac75@if52
```
# 在数组instances中启用eth_device
将以上获取的网卡列表，根据自己的诉求填入，如eth0
```
eth_device="eth0"
```
# 测试是否能获取到值
```
./categraf --test --inputs arp_packet

```
