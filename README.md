- [分布式事务](#分布式事务)
  - [简介](#简介)
  - [使用octopus实现一个分布式事务](#使用octopus实现一个分布式事务)
  - [相关文档](#相关文档)
  - [测试](#测试)

# 分布式事务


## 简介

Octopus是一个基于Golang的分布式事务解决方案，目前支持两种类型的分布式事务
- Saga
- TCC    



**事务角色**

一般分为三个事务角色
- 事务提交者AP ： 发起分布式事务的业务方
- 事务参与者RM ： 子事务实现者
- 事务协调者TC ： 分布式事务的管理者，管理事务的提交，回滚等


Octopus已经实现TC，AP和RM的相关封装，用户只需要实现业务逻辑即可，不需要关心分布式事务的细节以及各种异常情况的处理。


## 使用octopus实现一个分布式事务

使用octopus可以非常便利地实现一个分布式事务。 

如下，以saga为例，实现一个分布式事务。

**事务发起方AP**
```
  // 使用SagaTransaction启动一个分布式Saga 事务，然后添加子事务分支
	resp, err := saga.SagaTransaction(ctx, app.tcClient, sagaExpiredTime, 
    func(txn *saga.Transaction, gtid string) error {   
      txn.AddGrpcBranch(1, server1, commitMethodName, compensateMethodName)
      txn.AddGrpcBranch(2, server2, commitMethodName, compensateMethodName)
			return nil
		})
```

**事务参与方RM**

Saga分布式事务的RM要实现commit和compensation接口，开发者也只需要实现相关业务逻辑。
```
  // 提交逻辑，在HandleCommit的func(tx *sql.Tx)实现业务逻辑
	err := saga.HandleCommit(ctx, rm.db, gtid, bid, func(tx *sql.Tx) error {
    // 实现业务逻辑即可
    ......
	})

  // 回滚逻辑(补偿)
	err := saga.HandleCompensation(ctx, rm.db, gtid, bid, func(tx *sql.Tx) error {
    // 实现业务逻辑即可
    ......
	})
```

事务管理方TC由octopus实现，开发者只需要部署，无须开发任何代码。


可以看出，只需要几行代码就可以实现一个分布式事务，开发者不用考虑幂等，乱序等异常情况，只需要实现业务逻辑即可。


## 相关文档

具体理论和实现：
- [Orchestration-based saga 事务]
  - [saga理论](doc/README_saga.md)
  - [开发示例](doc/README_saga_demo.md)
  - [SDK设计](doc/README_saga_sdk.md)
- [TCC 事务]
  - [TCC理论](doc/README_tcc.md)
  - [开发示例](doc/README_tcc_demo.md)
  - [SDK设计](doc/README_tcc_sdk.md)
- [部署](doc/README_deployment.md)



## 测试


**单元测试**

各单元测试均通过，若提交PR，请确保单元测试通过，且覆盖新代码。


**压力测试**

github.com/ikenchina/octopus/test/perf/README.md 介绍了相关压力测试信息。

