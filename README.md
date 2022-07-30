# 分布式事务

Octopus是一个基于Golang的分布式事务解决方案，目前支持两种类型的分布式事务
- Saga
- TCC    


具体理论和实现：
- [Orchestration-based saga 事务]
  - [saga理论](doc/README_saga.md)
  - [开发示例](doc/README_saga_demo.md)
  - [SDK设计](doc/README_saga_sdk.md)
- [TCC 事务]
  - [TCC理论](doc/README_tcc.md)
  - [开发示例](doc/README_tcc_demo.md)
  - [SDK设计](doc/README_tcc_sdk.md)


**事务角色**

一般分为三个事务角色
- 事务提交者AP ： 发起分布式事务的业务方
- 事务参与者RM ： 子事务实现者
- 事务协调者TC ： 分布式事务的管理者，管理事务的提交，回滚等


**使用octopus实现一个分布式事务**

使用octopus可以非常便利地实现一个分布式事务。 

如下，以saga为例，实现一个分布式事务。

事务发起方
```
	resp, err := saga.SagaTransaction(ctx, app.tcClient, sagaExpiredTime, func(txn *saga.Transaction, gtid string) error {
      txn.AddGrpcBranch(1, server1, commitMethodName, compensateMethodName)
      txn.AddGrpcBranch(2, server2, commitMethodName, compensateMethodName)
			return nil
		})
```

事务参与方
```
  // 提交逻辑
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

事务管理方由octopus实现，开发者无须考虑，只需要部署即可。


可以看出，只需要几行代码就可以实现一个分布式事务，开发者不用考虑幂等，乱序等异常情况，只需要实现业务逻辑即可。
