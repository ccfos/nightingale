国内用户可以访问gitee镜像仓库： https://gitee.com/mirrors/Nightingale 同步非实时，每天一次，不过速度快

---

# 升级说明

v3.x的版本和v2.x差别巨大，没办法平滑迁移，可以继续使用 [v.2.8.0](https://github.com/didi/nightingale/tree/v2.8.0) ，我们之所以决定升级到v3.x，具体原因 [请看这里](https://mp.weixin.qq.com/s/BoGcqPiIQIuiK7cM3PTvrw) ，简而言之，我们是希望夜莺逐渐演化为一个运维平台。如果v2.x用着也能满足需求，可以继续用v2.x，毕竟，适合自己的才是最好的

# 新版效果

用户资源中心：

![用户资源中心截图](https://s3-gz01.didistatic.com/n9e-pub/image/snapshot/rdb.png)

资产管理中心：

![资产管理中心截图](https://s3-gz01.didistatic.com/n9e-pub/image/snapshot/ams.png)

任务执行中心：

![任务执行中心截图](https://s3-gz01.didistatic.com/n9e-pub/image/snapshot/job.png)

监控告警中心：

![监控告警中心截图](https://s3-gz01.didistatic.com/n9e-pub/image/snapshot/mon.png)


# 安装步骤

1、找个干净的CentOS7，准备好mysql、redis、nginx，简单yum安装一下即可，生产环境mysql建议找dba帮忙来搞

```shell script
yum install -y mariadb* redis nginx
```

2、下载我们编译好的二进制到/home/n9e目录，如果要更换目录，要注意修改nginx.conf，建议先用这个目录，玩熟了再说

```shell script
mkdir -p /home/n9e
cd /home/n9e
wget http://116.85.64.82/n9e.tar.gz
tar zxvf n9e.tar.gz
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
wget http://116.85.64.82/pub.tar.gz
tar zxvf pub.tar.gz
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

上面安装步骤如果走完了仍然没有搭建起来，你可能需要 [使用Docker安装](dockerfiles/README.md) 或者 [查看视频教程](https://mp.weixin.qq.com/s/OAEQ-ec-QM74U0SGoVCXkg)

# 子系统简介

夜莺拆成了四个子系统，分别是：用户资源中心（RDB）、资产管理系统（AMS）、任务执行中心（JOB）、监控告警系统（MON）。下面分别介绍一下这几个子系统的设计初衷

## 用户资源中心

这是一个平台底座，所有的运维系统，都需要依赖这个，内置用户、权限、角色、组织、资源的管理。最核心的是一棵组织资源树，树节点的类别和扩展字段可以自定义，组织资源树的层级结构最简单的组织方式是：租户》项目》模块，复杂一点的组织方式：租户》组织》项目》模块》集群，组织是可以嵌套的。节点上挂两类对象，一个是人员权限，一个是资源，资源可以是各类资源，除了主机设备、网络设备，也可以是rds实例，redis实例，当然，这就需要rds、redis的管控系统和RDB打通了。滴滴在做一些大的中后台商业化解决方案的时候，RDB就是扮演了这么一个底座的角色。

## 资产管理系统

这里的资产管理系统，是偏硬件资产的管理，这个系统的使用者一般是系统部的人，资产管理类人员，应用运维相对不太关注这个系统。开源版本开放了一个主机设备的管理，大家可以二开，增加一些网络设备管理、机柜机架位的管理、配件耗材的管理等等，有了底座，上面再长出一些其他系统都相对容易。agent安装之后，会自动注册到资产管理系统，自动采集到机器的sn、ip、cpu、mem、disk等信息，这些信息为了灵活性考虑，都是用shell采集的，上文“安装步骤”一章有提到，其中最重要的是ip，系统中有很多设备，ip是需要全局唯一，其他的sn、cpu、mem、disk等，如果无法采集成功，可以写死，shell里直接写echo一个假数据即可。

每一条资产，都有一个租户的字段，代表资产归属，需要管理员去分配资产归属（修改资产的所属租户），各个租户才能使用对应的资产，分配完了之后，会出现在用户资源中心的“游离资源”菜单中，各个租户就可以把游离资源挂到资产树上去分门别类的管理使用。树节点的创建是在树上右键哈。

## 任务执行中心

用于批量跑脚本，类似pssh、ansible、saltstack，不过不支持playbook，大道至简，就用脚本撸吧，shell、python、perl、ruby，都行，只要机器上有解析器。因为是内置到夜莺里的，所以体系化会更好一些，和组织资源树的权限是打通的，可以控制不同的人对不同的机器有不同的权限，有些人可以用root账号执行，有些人只能用普通账号执行，历史执行记录都可以通过web页面查看审计。任务本身支持一些控制：暂停点、容忍度、单机超时时间、中途暂停、中途取消、中途Kill等。

一些经常要跑的脚本，可以做成模板，模板时对脚本的一种管理方式，后续就可以基于模板创建任务，填个机器列表就可以执行。比如安装JDK，调整TCP内核参数，调整ulimit等机器初始化脚本，都可以做成模板。

开源版本的任务执行中心，可以看做是一个命令通道，后续可以基于这个命令通道构建一些场景化应用，比如机器初始化平台、服务变更发布平台、配置分发系统等。任务执行中心各类操作都有API对外暴露，具体可参看：[router.go](https://github.com/didi/nightingale/blob/master/src/modules/job/http/router.go) 我司的命令通道每周执行任务量超过60万，就是因为各类上层业务都在依赖这个命令通道的能力。

## 监控告警系统

这块核心逻辑和v2版本差别不大，监控指标分成了设备相关指标和设备无关指标，因为有些自定义监控数据的场景，endpoint不好定义，或者endpoint经常变化，这种就可以使用设备无关指标的方式来处理。监控大盘做了优化，引入了更多类型的图表，但夜莺毕竟是个metrics监控系统，处理的是数值型时序数据，所以，最有用的图表其实就是折线图，其他类型图表，看看就好，场景较少。夜莺也可以对接Grafana，有个专门的[DataSource插件](https://github.com/n9e/grafana-n9e-datasource)，Grafana会更炫酷一些，只是，在数据量大的时候性能较差。

# 系统架构

![n9e系统架构图](https://s3-gz01.didistatic.com/n9e-pub/image/n9e-v3-arch.png)

监控部分的架构和之前没有差别，collector揉进了一些命令执行的能力，所以改了个名字叫agent。引入了三个新组件：rdb、ams、job，rdb是用户资源中心，ams是资产管理系统，job是任务执行中心。agent除了上报监控数据给transfer，还会上报本机信息给ams，注册本机信息到资产管理系统，另外就是与job模块交互，拉取要执行的任务，上报任务执行结果。

# 文档手册

v3版本不准备单独建站了，文档全部使用github wiki： https://github.com/didi/nightingale/wiki 欢迎大家一起完善。另外当前正在录制一套夜莺的教学视频，后续会放到微信公众号：ops-soldier，欢迎关注获取教程

# 交流互助

对于夜莺的建议或修改，请直接提交issue或pr。如想加入【夜莺网友互助交流群】，请加微信好友：UlricQin，注明加群。

# 商业版本

夜莺开源版本是从商业版本中摘取的部分功能，商业版本会更强大，滴滴不止有运维平台的商业化解决方案，还有DevOps、IaaS、PaaS、大数据、安全等各类商业化产品，如有兴趣欢迎联系我们，微信号：UlricQin，注明商业版。


