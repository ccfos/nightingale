#!/bin/bash
sed -i 's/127.0.0.1/mysql/g' etc/mysql.yml
sed -i 's/addr: 127.0.0.1:6379/addr: redis:6379/g' etc/rdb.yml

mysqlRootPassword=1234

until mysql -hmysql -u root -p$mysqlRootPassword  -e ";" ; do
    echo "Can't connect mysql, retry"
    sleep 5
done

mysql -hmysql -uroot -p$mysqlRootPassword < sql/n9e_ams.sql
mysql -hmysql -uroot -p$mysqlRootPassword < sql/n9e_hbs.sql
mysql -hmysql -uroot -p$mysqlRootPassword < sql/n9e_job.sql
mysql -hmysql -uroot -p$mysqlRootPassword < sql/n9e_mon.sql
mysql -hmysql -uroot -p$mysqlRootPassword < sql/n9e_rdb.sql
mysql -hmysql -uroot -p$mysqlRootPassword < sql/n9e_rdb.sql
mysql -hmysql -uroot -p$mysqlRootPassword < sql/n9e_rdb.sql

./control start all
sleep infinity

