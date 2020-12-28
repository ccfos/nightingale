# 安装 M3db，ETCD

下载安装包 [m3db-install.tar.gz](https://s3-gz01.didistatic.com/n9e-pub/tarball/m3db-install.tar.gz) 


## 配置
#### etcd证书
```shell
# 修改 ./etcd/certs/config/hosts-etcd
# 填写etcd的机器列表

# 生成证书
cd etcd/certs
./reinit-root-ca.sh
./update-etcd-client-certs.sh
./update-etcd-server-certs.sh
```


#### m3db 配置
```shell
# 修改 ./m3db/etc/m3dbnode.yml
db:
  config:
    service:
      etcdClusters:
        - zone: embedded
          endpoints:
            - 10.255.0.146:2379  # 这里需要修改 etcd 节点信息
            - 10.255.0.129:2379
```


## 安装部署
```shell
. ./functions.sh
# 设置安装的机器节点，
hosts="{node1} {node2} {node3}"
# 设置好ssh Public key认证后，设置 publick key 用户名
user="dc2-user"

# 同步文件
sync

# 安装
install

# 检查
status
```
