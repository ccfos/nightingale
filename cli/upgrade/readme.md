# v5 升级 v6 手册

1. 解压 n9e 安装包
2. 导入 upgrade.sql 到 n9e_v5 数据库
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

5. n9e 监听的端口为 17000，如果想使用之前的端口，可以在配置文件中将端口改为 18000