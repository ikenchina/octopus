# 部署



## 部署数据库

### 部署 postgreSQL

创建数据库用户
```
CREATE USER $user_name WITH ENCRYPTED PASSWORD '$user_password';
```

创建数据库，及赋予权限给新用户
```
CREATE DATABASE $db_name;
GRANT ALL PRIVILEGES ON DATABASE $db_name to $user_name;
```

登录新用户
```
psql -d $db_name -U $user_name -W
```


根据 octopus/tc/deployment/pg.sql 创建schema和表。


**注意事项**

由于TC的dtx.global_txn使用了条件索引，需要注意索引膨胀问题，定期做vacuum。



## 部署二进制

使用systemd进行管理，参考 dtx.tc.service
