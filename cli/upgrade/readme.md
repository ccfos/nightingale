# v5 升级 v6 手册
0. 操作之前，记得备注下数据库！

1. 需要先将你正在使用的夜莺数据源表结构更新到和 v5.15.0 一致，[release](https://github.com/ccfos/nightingale/releases) 页面有每个版本表结构的更新说明，可以根据你正在使用的版本，按照说明，逐个执行的更新表结构的语句

2. 解压 n9e 安装包，导入 upgrade.sql 到 n9e_v5 数据库
```
mysql -h 127.0.0.1 -u root -p1234 < cli/upgrade/upgrade.sql
```

3. 执行 n9e-cli 完成数据库表结构升级, webapi.conf 为 v5 版本 n9e-webapi 正在使用的配置文件
```
./n9e-cli --upgrade --config webapi.conf
```

4. 修改 n9e 配置文件中的数据库为 n9e_v5，启动 n9e 进程
```
nohup ./n9e &> n9e.log &
```

5. n9e 监听的端口为 17000，需要将之前的 web 端口和数据上报的端口，都调整为 17000