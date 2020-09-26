v3.x终于来了，文档正在编写中，稍安勿躁...

---

# 升级说明

v3.x的版本和v2.x差别巨大，如果短期没办法迁移，可以继续使用 [v.2.8.0](https://github.com/didi/nightingale/tree/v2.8.0) ，我们之所以决定升级到v3.x，具体原因 [请看这里](https://mp.weixin.qq.com/s/BoGcqPiIQIuiK7cM3PTvrw) ，简而言之，我们是希望夜莺逐渐演化为一个运维平台

# 新版效果

几张图

# 安装步骤

1、找个干净的CentOS7，准备好mysql、redis、nginx，简单yum安装一下即可，生产环境mysql建议找dba帮忙来搞

```
yum install -y mariadb* redis nginx
```

2、下载我们编译好的二进制到/home/n9e目录，如果要更换目录，要注意修改nginx.conf，建议先用这个目录，玩熟了再说

```
mkdir -p /home/n9e
cd /home/n9e
wget http://116.85.64.82/n9e-3.0.0.tar.gz
tar zxvf n9e-3.0.0.tar.gz
```

3、初始化数据库，这里假设使用root账号，密码1234，如果不是这个账号密码，注意修改/home/n9e/etc/mysql.yml

```
cd /home/n9e/sql
mysql -uroot -p1234 < n9e_ams.sql
mysql -uroot -p1234 < n9e_hbs.sql
mysql -uroot -p1234 < n9e_job.sql
mysql -uroot -p1234 < n9e_mon.sql
mysql -uroot -p1234 < n9e_rdb.sql
```

4、redis配置修改，默认配置的6379端口，密码为空，如果默认配置不对，可以执行如下命令，看到多个配置文件里有redis相关配置，挨个检查修改下

```
cd /home/n9e/etc
grep redis -r .
```

5、下载前端静态资源文件



6、覆盖nginx.conf


7、检查identity.yml


8、检查agent.yml的shell


