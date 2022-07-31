# 压力测试


## 部署TC

见 github.com/ikenchina/octopus/doc/README_deployment.md


## 部署RM

先初始化所需要的数据库：
- 部署RM所需要的表：github.com/ikenchina/octopus/rm/deployment，根据目录sql进行初始化。
- 部署业务所需要的表：github.com/ikenchina/octopus/demo/demo.sql
- 初始化测试用户
```
// 以发工资为例，负数是公司的账户，正数是员工的账户
insert into dtx.account(id) select * from generate_series(-10000, 10000);
```

然后运行二进制
```
go run . --config ./config.json --role=rm --txn=saga
```
如果测试tcc，则改成`--txn=tcc`


## 运行AP

```
go run . --config ./config.json --role=ap --txn=saga
```





