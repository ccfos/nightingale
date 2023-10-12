# sqlserver

forked from telegraf/sqlserver. 这个插件的作用是获取sqlserver的监控指标，这里去掉了Azure相关部分监控，只保留了本地部署sqlserver情况。

# 使用
按照下面方法创建监控账号，用于读取监控数据
USE master;

CREATE LOGIN [categraf] WITH PASSWORD = N'mystrongpassword';

GRANT VIEW SERVER STATE TO [categraf];

GRANT VIEW ANY DEFINITION TO [categraf];
Data Source=10.19.1.1;Initial Catalog=hc;User ID=sa;Password=mystrongpassword;