v3.x终于来了，文档正在编写中，稍安勿躁...

---

# 升级说明

v3.x的版本和v2.x差别巨大，如果短期没办法迁移，可以继续使用 [v.2.8.0](https://github.com/didi/nightingale/tree/v2.8.0) ，我们之所以决定升级到v3.x，具体原因 [请看这里](https://mp.weixin.qq.com/s/BoGcqPiIQIuiK7cM3PTvrw) ，简而言之，我们是希望夜莺逐渐演化为一个运维平台

# 新版效果

用户资源中心：

![](https://s3-gz01.didistatic.com/n9e-pub/image/snapshot/rdb.png)

资产管理中心：

![](https://s3-gz01.didistatic.com/n9e-pub/image/snapshot/ams.png)

任务执行中心：

![](https://s3-gz01.didistatic.com/n9e-pub/image/snapshot/job.png)

监控告警中心：

![](https://s3-gz01.didistatic.com/n9e-pub/image/snapshot/mon.png)


# 安装步骤

1、找个干净的CentOS7，准备好mysql、redis、nginx，简单yum安装一下即可，生产环境mysql建议找dba帮忙来搞

```shell script
yum install -y mariadb* redis nginx
```

2、下载我们编译好的二进制到/home/n9e目录，如果要更换目录，要注意修改nginx.conf，建议先用这个目录，玩熟了再说

```shell script
mkdir -p /home/n9e
cd /home/n9e
wget http://116.85.64.82/n9e-3.0.0.tar.gz
tar zxvf n9e-3.0.0.tar.gz
```

3、初始化数据库，这里假设使用root账号，密码1234，如果不是这个账号密码，注意修改/home/n9e/etc/mysql.yml

```shell script
cd /home/n9e/sql
mysql -uroot -p1234 < n9e_ams.sql
mysql -uroot -p1234 < n9e_hbs.sql
mysql -uroot -p1234 < n9e_job.sql
mysql -uroot -p1234 < n9e_mon.sql
mysql -uroot -p1234 < n9e_rdb.sql
```

4、redis配置修改，默认配置的6379端口，密码为空，如果默认配置不对，可以执行如下命令，看到多个配置文件里有redis相关配置，挨个检查修改下

```shell script
cd /home/n9e/etc
grep redis -r .
```

5、下载前端静态资源文件，放到默认的/home/n9e目录下，如果要改目录，需要修改后面提到的nginx.conf

```shell script
cd /home/n9e
wget http://116.85.64.82/pub.0927.tar.gz
tar zxvf pub.0927.tar.gz
```

6、覆盖nginx.conf，建议大家还是看一下这个配置，熟悉一下nginx配置，夜莺不同web侧组件就是通过nginx的不同location区分的。覆盖完了配置记得reload一下或者重启nginx

```shell script
cp etc/nginx.conf /etc/nginx/nginx.conf
```

7、检查identity.yml，要保证这个shell可以正常获取本机ip，如果实在不能正常获取，自己又不懂shell不会改，在specify字段写死也行

```yaml
# 用来做心跳，给服务端上报本机ip
ip:
  specify: ""
  shell: ifconfig `route|grep '^default'|awk '{print $NF}'`|grep inet|awk '{print $2}'|head -n 1

# MON、JOB的客户端拿来做本机标识
ident:
  specify: ""
  shell: ifconfig `route|grep '^default'|awk '{print $NF}'`|grep inet|awk '{print $2}'|head -n 1
```

8、检查agent.yml的几个shell，挨个检查是否可以跑通，跑不通就改成适合自己的，实在是不会改，直接写死，比如disk部分，写死80Gi直接写：`disk: echo 80Gi`即可

```yaml
report:
  # ...
  sn: dmidecode -s system-serial-number | tail -n 1

  fields:
    cpu: cat /proc/cpuinfo | grep processor | wc -l
    mem: cat /proc/meminfo | grep MemTotal | awk '{printf "%dGi", $2/1024/1024}'
    disk: df -m | grep '/dev/' | grep -v '/var/lib' | grep -v tmpfs | awk '{sum += $2};END{printf "%dGi", sum/1024}'
```

9、启动各个进程，包括mysql、redis、nginx，夜莺的各个组件直接用control脚本启动即可，后续上生产环境，可以用systemd之类的托管

```shell script
cd /home/n9e
./control start all
```

10、登录web，账号root，密码root.2020，进来第一步一定要修改密码，如果nginx报权限类的错误，检查selinux是否关闭了，如下命令可关闭

```shell script
setenforce 0
```



