- [Saga 开发演示](#saga-开发演示)
	- [角色](#角色)
	- [事务提交者AP](#事务提交者ap)
	- [事务参与者RM](#事务参与者rm)
		- [创建子事务表](#创建子事务表)
		- [实现gRPC协议的RM](#实现grpc协议的rm)
		- [实现http协议的RM](#实现http协议的rm)


# Saga 开发演示


Saga适合长事务，不会锁定资源，以补偿的方式来取消对数据的操作。

本文我们以发工资为例子，来演示演示如何开发一个saga分布式事务，公司给员工发工资组成一个Saga事务。

先从公司账户扣除所有员工的工资，再分别给员工账户发工资，如果某个用户账户所在银行调用失败，则不断重试直到成功，达到最终一致性；或如果不再重试，则需要回滚，先从已发工资的员工账户扣除已发的工资，最后加到公司账户中，但存在中间状态，可能在事务执行中，给员工账户加工资了，但事务没有结束而员工花费了这笔工资，如果需要回滚则会可能存在用户账户不够扣除的情况，要避免这种情况则需要使用TCC事务。



具体代码请参考：https://github.com/ikenchina/octopus/tree/master/demo



## 角色

开发者只需要关心两个角色，如下

- 事务提交者AP：Saga事务的发起方。那对于发工资例子，那AP就是公司的服务。
- 事务参与者RM：事务的参与方。对于发工资例子，RM就是银行的服务。





## 事务提交者AP

直接使用octopus/ap/saga下的wrapper.go的封装，调用其`SagaTransaction`方法来实现Saga事务。

如果AP需要TC事务结束后通知AP，则需要提供一个http或者gRPC接口给TC来回调。

```
// 发工资的Saga事务实现
func (app *SagaApplication) Pay(users []*saga_rm.BankAccountRecord) (*pb.SagaResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	notifyAction := fmt.Sprintf("http://localhost%s/saga/notify", app.listen)
	transactionExpiredTime := time.Now().Add(1*time.Minute)

	//
	resp, err := saga_cli.SagaTransaction(ctx, app.tcClient, transactionExpiredTime,
		func(t *saga_cli.Transaction, gtid string) error {

			// 开发者可以在数据库中将saga事务存储起来，后面TC回调来通知事务状态时，可以查询来更新此事务
			app.saveGtidToDb(gtid)

			// 设置回调接口给TC来通知AP事务的最终态，
			// 也可以选择不通知，如果不通知，则需要定期查询TC事务的状态
			t.SetHttpNotify(notifyAction, time.Second, time.Second)

			// 遍历所有员工
			for i, user := range users {
				branchID := i + 1
				// 用户账户的银行信息：银行rpc地址等
				host := app.getUserBank(user.UserID)

				// 如果银行提供的接口是gRPC协议
				if host.Protocol == "grpc" {
					// 银行gRPC服务的commit和compensation接口的method
					commit := "/bankservice.SagaBankService/In"
					compensation := "/bankservice.SagaBankService/Out"
					// commit和compensation的请求体
					payload := user.ToPb()

					// 添加一个子事务，如果TC调用RM失败两次，则会回滚saga事务
					t.AddGrpcBranch(branchID, host.Target, commit, compensation, payload, saga_cli.WithMaxRetry(1))

				} else if host.Protocol == "http" { // 如果银行提供的接口是http协议
					// commit和compensation使用actionURL同一个URL, payload是commit和compensation的请求body
					actionURL := fmt.Sprintf("%s%s/%s/%d", host.Target, saga_rm.SagaRmBankServiceBasePath, gtid, branchID)
					payload := jsonMarshal(user)

					// 添加一个子事务，如果TC调用RM失败两次，则会回滚saga事务
					t.AddHttpBranch(branchID, actionURL, actionURL, payload, saga_cli.WithMaxRetry(1))
				}
			}
			return nil
		})
		
	// resp ：TC响应分布式事务的结果信息
	// err ：事务是否执行出错
	return resp, err
}
```


---


## 事务参与者RM

RM由员工账户所属银行来实现。

RM需要提供 `commit`和`compensation`接口给TC调用来提交子事务。

直接使用octopus/rm/saga下的wrapper.go的`HandleCommit`和`HandleCompensation`来实现接口。

参考：`octopus/test/rm/saga`



### 创建子事务表

如果开发者不使用rm package提供的`HandleCommit`和`HandleCompensation`系列方法，则不需要创建子事务表。    
开发者可以自己实现commit相关的事务逻辑，只需要保证幂和避免乱序等异常即可(根据gtid和branch id)。 

如果开发者使用`HandleCommit`和`HandleCompensation`系列方法，则需要创建子事务表。    
如果是PostgreSQL数据库，则根据`octopus/rm/deployment/postgreSQL.sql`来创建。


建议开发者使用`HandleCommit`和`HandleCompensation`系列方法，因为其处理了乱序等异常情况。


### 实现gRPC协议的RM


**实现提交**
```
func (rm *SagaRmBankGrpcService) In(ctx context.Context, in *pb.SagaRequest) (*pb.SagaResponse, error) {
	// 从ctx中提取gtid和branch id， ParseContextMeta在octopus/common/grpc实现
	gtid, bid := cgrpc.ParseContextMeta(ctx)
	resp := &pb.SagaResponse{}

	// 调用HandleCommit来实现提交逻辑，具体业务逻辑在func(tx *sql.Tx) error中(tx是一个数据库事务)，
	// HandleCommit实现了异常处理，幂等性 等逻辑，
	err := sagarm.HandleCommit(ctx, rm.db, gtid, bid, func(tx *sql.Tx) error {

		// 直接更新用户银行账号余额，给员工发工资
		rx, err := tx.Exec("UPDATE account SET balance=balance+$1 WHERE id=$2", in.GetAccount(), in.GetUserId())
		if err != nil {
			return status.Error(codes.Internal, err.Error())
		}
		ar, err := rx.RowsAffected()
		if err != nil {
			return status.Error(codes.Internal, txr.Error.Error())
		}
		if ar == 0 { 
			return status.Error(codes.NotFound, "user does not exist")
		}
		return nil
	})
	return resp, err
}
```


**实现补偿**
```
func (rm *SagaRmBankGrpcService) Out(ctx context.Context, in *pb.SagaRequest) (*pb.SagaResponse, error) {
	gtid, bid := cgrpc.ParseContextMeta(ctx)
	resp := &pb.SagaResponse{}

	// 调用 HandleCompensation 在func(tx *sql.Tx) error中实现补偿逻辑
	err := sagarm.HandleCompensation(ctx, rm.db, gtid, bid, func(tx *sql.Tx) error {
		// 补偿： -1 * in.GetAccount() 
		rx, err := tx.Exec("UPDATE account SET balance=balance+$1 WHERE id=$2", -1 * in.GetAccount(), in.GetUserId())
		if err != nil {
			return err
		}
		ar, err := rx.RowsAffected()
		if err != nil {
			return status.Error(codes.Internal, txr.Error.Error())
		}
		if ar == 0 {
			return status.Error(codes.NotFound, "user does not exist")
		}
		return nil
	})
	return resp, err
}
```


### 实现http协议的RM

RM提供`commit`和`compensation`接口

- commit：TC以http的POST方式调用此接口
- compensation：TC以DELETE方式调用此接口

```
// http服务，提供commit和compensation接口
func (rm *RmService) start() error {
	app := gin.New()
	
	// commit提供POST方式接口，compensation提供DELETE方式接口
	app.POST(service_BasePath+"/:gtid/:branch_id", rm.commitHandler)
	app.DELETE(service_BasePath+"/:gtid/:branch_id", rm.compensationHandler)
	
	rm.httpServer = &http.Server{
		Addr:    rm.listen,
		Handler: app,
	}
	return rm.httpServer.ListenAndServe()
}
```



**实现commit接口**

```
// commit接口
func (rm *RmService) commitHandler(c *gin.Context) {
	gtid := c.Param("gtid")
	branchID, _ := strconv.Atoi(c.Param("branch_id"))

	// 读取，反序列化 http body
	request := &BankAccountRecord{}
	err := http_util.ParseHttpJsonRequest(c, request)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	code := http.StatusOK

	// 调用HandleCommit
	// HandleCommitOrm 和HandleCommit一样，只是数据库访问使用ORM的方式(gorm)
	err = sagarm.HandleCommitOrm(c.Request.Context(), rm.db, gtid, branchID,
		func(tx *gorm.DB) error {
			txr := tx.Model(BankAccount{}).Where("id=?", request.UserID).
				Update("balance", gorm.Expr("balance+?", request.Account))
			if txr.Error != nil {
				code = http.StatusInternalServerError
				return txr.Error
			}
			if txr.RowsAffected == 0 {
				code = http.StatusNotFound
				return fmt.Errorf("user does not exist")
			}
			return nil
		})
	
	// 如果事务执行失败，则返回错误，通知TC这个commit执行失败
	if err != nil {
		if code == http.StatusOK {
			code = http.StatusInternalServerError
		}
		c.Status(code)
		_, _ = c.Writer.Write([]byte(err.Error()))
		return
	}
}
```



**实现compensation接口**

```
func (rm *RmService) compensationHandler(c *gin.Context) {
	gtid := c.Param("gtid")
	branchID, _ := strconv.Atoi(c.Param("branch_id"))
	request := &BankAccountRecord{}
	err := http_util.ParseHttpJsonRequest(c, request)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	code := http.StatusOK

	// 调用HandleCompensation
	err = sagarm.HandleCompensationOrm(c.Request.Context(), rm.db, gtid, branchID,
	
		// compensation实现的业务逻辑，逻辑会在一个事务中执行
		// body是commit请求时的body，由AP提供
		func(tx *gorm.DB) error {
			
			// 取消 commit 逻辑给员工银行账户添加的工资，
			txr := tx.Model(BankAccount{}).Where("id=?", request.UserID).
				Update("balance", gorm.Expr("balance+?", -1*request.Account))
			if txr.Error != nil {
				code = http.StatusInternalServerError
				return txr.Error
			}
			if txr.RowsAffected == 0 {
				code = http.StatusNotFound
				return fmt.Errorf("user does not exist")
			}
			return nil
		})

	// 如果compensation事务失败，则返回给TC，TC会不断重试直到成功
	if err != nil {
		if code == http.StatusOK {
			code = http.StatusInternalServerError
		}
		c.Status(code)
		_, _ = c.Writer.Write([]byte(err.Error()))
		return
	}
}
```
