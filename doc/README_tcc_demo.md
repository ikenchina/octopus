- [TCC 开发演示](#tcc-开发演示)
	- [角色](#角色)
	- [事务提交者AP](#事务提交者ap)
	- [事务参与者RM](#事务参与者rm)
		- [创建子事务表](#创建子事务表)
		- [实现gRPC协议的RM](#实现grpc协议的rm)
		- [实现HTTP协议的RM](#实现http协议的rm)

# TCC 开发演示



本文我们以银行转账为例子，来演示演示如何开发一个TCC分布式事务。

具体代码请参考：https://github.com/ikenchina/octopus/tree/master/demo

---

## 角色

开发者只需要关心两个角色，如下

- 事务提交者AP：TCC事务的发起方。对于转账的例子，AP就属于银联的服务
- 事务参与者RM：事务的参与方。对于转账例子，RM就是账户所属的银行的服务。


----


## 事务提交者AP

直接使用octopus/ap/tcc下的wrapper.go的封装，调用其`TccTransaction`方法来实现TCC事务。



`TccTransaction`方法提交的TCC事务

```
// 转账
func (app *TccApplication) Transfer(users []*tcc_rm.BankAccountRecord) (*pb.TccResponse, error) {
	tccCtx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	// 事务过期时间，超过此时间，如果事务仍然没有提交，则TC会取消事务
	transactionExpireTime := time.Now().Add(time.Second)

	// 直接调用TccTransaction来实现Tcc事务
	tccResp, err := tcc_cli.TccTransaction(tccCtx, app.tcClient, transactionExpireTime,
		func(ctx context.Context, t *tcc_cli.Transaction, gtid string) error {

			// 将gtid持久化，方便后续查询，也避免事务提交TC时崩溃(可能提交成功也可能失败)，再重启不知道此事务的存在
			app.saveGtidToDb(gtid)

			// 遍历需要转账的用户
			for i, user := range users {
				branchID := i + 1
				host := app.getBank(user.UserID)

				// 如果银行是gRPC接口
				if host.Protocol == "grpc" {
					cli := app.getBankGrpcClient(host.Target)
					response := &pb.TccResponse{}
					request := &tcc_pb.TccRequest{
						UserId:  int32(user.UserID),
						Account: int32(user.Account),
					}

					// TryGrpc ： 注册这个子事务到TC，再调用RM(账户所属银行)的try接口
					err := t.TryGrpc(
						branchID, // 分支事务ID
						cli,      // rm的grpc connection
						"/bankservice.TccBankService/Try",
						"/bankservice.TccBankService/Confirm", // 银行的Confirm接口，由TC调用
						"/bankservice.TccBankService/Cancel",  // 银行的Cancel接口，由TC调用
						request,                               // grpc try请求结构
						response)                              // grpc try请求返回结构
					if err != nil {
						return err
					}
				} else if host.Protocol == "http" { // 如果银行是http接口
					// try, confirm, cancel为同一个URL，由http method区分，分别为POST, PUT, DELETE方法
					actionAURL := fmt.Sprintf("%s%s/%s/%d", host.Target, tcc_rm.TccRmBankServiceBasePath, gtid, branchID)

					// Transaction提供了TryHttp接口：将子事务注册到TC，再调用RM(银行)的try接口
					_, err := t.TryHttp(branchID,
						actionAURL, // try接口
						actionAURL, // confirm接口
						actionAURL, // cancel接口
						jsonMarshal(&tcc_rm.BankAccountRecord{
							UserID:  user.UserID,
							Account: user.Account,
						}))
					if err != nil {
						return err
					}
				}
			}

			return nil
		})

	app.updateStateToDb(tccResp.GetTcc().GetGtid(), tccResp.GetTcc().GetState())
	return tccResp, err
}
```



---


## 事务参与者RM

RM由员工账户所属银行来实现。

RM需要提供 `try`、`confirm`和`cancel`接口来执行、提交和取消子事务。

直接使用octopus/rm/tcc下的wrapper.go的`HandleTry`、`HandleConfirm`和`HandleCancel`来实现接口。

参考：`octopus/test/rm/tcc`



### 创建子事务表


如果开发者不使用rm package提供的`HandleTry`、`HandleConfirm`和`HandleCancel`系列方法，则不需要创建子事务表。    
开发者可以自己实现commit相关的事务逻辑，只需要保证幂和避免乱序等异常即可(根据gtid和branch id)。 

如果开发者使用`HandleTry`、`HandleConfirm`和`HandleCancel`系列方法，则需要创建子事务表。    
如果是PostgreSQL数据库，则根据`octopus/rm/deployment/postgreSQL.sql`来创建。


建议开发者使用`HandleTry`、`HandleConfirm`和`HandleCancel`系列方法，因为其处理了乱序等异常情况且保证了幂等。



### 实现gRPC协议的RM

**实现try**

```
func (rm *TccRmBankGrpcService) Try(ctx context.Context, request *pb.TccRequest) (*pb.TccResponse, error) {
	gtid, branchID := sgrpc.ParseContextMeta(ctx)
	err := tccrm.HandleTry(ctx, rm.db, gtid, branchID,
		func(tx *sql.Tx) error {
			rx, err := tx.Exec("UPDATE account SET balance_freeze=$1 WHERE id=$2 AND balance_freeze=0",
				request.GetAccount(), request.GetUserId())
			if err != nil {
				return status.Error(codes.Internal, err.Error())
			}
			ar, err := rx.RowsAffected()
			if err != nil {
				return status.Error(codes.Internal, err.Error())
			}
			if ar == 0 {
				return status.Error(codes.NotFound, "user does not exist")
			}
			return nil
		})
	return &pb.TccResponse{}, err
}
```

**实现confirm**

confirm使用HandleConfirmOrm来实现事务，不同于HandleConfirm，此函数使用gorm数据库封装
```
func (rm *TccRmBankGrpcService) Confirm(ctx context.Context, request *pb.TccRequest) (*pb.TccResponse, error) {
	gtid, branchID := sgrpc.ParseContextMeta(ctx)

	err := tccrm.HandleConfirmOrm(ctx, rm.db, gtid, branchID,
		func(tx *gorm.DB) error {
			// 将用户冻结资金加到账户余额中，同时清空冻结资金列
			txr := tx.Model(BankAccount{}).Where("id=?", request.GetUserId()).
				Updates(map[string]interface{}{
					"balance":        gorm.Expr("balance+balance_freeze"),
					"balance_freeze": 0})
			if txr.Error != nil {
				return status.Error(codes.Internal, txr.Error.Error())
			}
			if txr.RowsAffected == 0 {
				return status.Error(codes.FailedPrecondition, "insufficient balance or freeze != 0")
			}
			return nil
		})
	return &pb.TccResponse{}, err
}

```


**实现cancel**

```
func (rm *TccRmBankGrpcService) Cancel(ctx context.Context, request *pb.TccRequest) (*pb.TccResponse, error) {
	gtid, branchID := sgrpc.ParseContextMeta(ctx)

	err := tccrm.HandleCancelOrm(ctx, rm.db, gtid, branchID,
		func(tx *gorm.DB) error {
			// 取消事务，将冻结资金列清空
			txr := tx.Model(BankAccount{}).Where("id=?", request.GetUserId()).Update(
				"balance_freeze", 0)
			if txr.Error != nil {
				return status.Error(codes.Internal, txr.Error.Error())
			}
			if txr.RowsAffected == 0 {
				return status.Error(codes.FailedPrecondition, "insufficient balance or freeze != 0")
			}
			return nil
		})
	return &pb.TccResponse{}, err
}

```






### 实现HTTP协议的RM


RM提供的接口

- try：提供给AP调用，AP以HTTP的POST方式来调用
- confirm：提供给TC调用，TC以HTTP的PUT方式来调用
- cancel：提供给TC调用，TC以HTTP的DELETE方式来调用

```
func (rm *RmServiceA) start(listen string, db *gorm.DB) error {
	rm.Db = db
	app := gin.New()
	
	// 提供try接口
	app.POST(service_BasePath+"/:gtid/:branch_id", rm.tryHandler)
	// confirm接口
	app.PUT(service_BasePath+"/:gtid/:branch_id", rm.confirmHandler)
	// cancel接口
	app.DELETE(service_BasePath+"/:gtid/:branch_id", rm.cancelHandler)
	rm.httpServer = &http.Server{
		Addr:    listen,
		Handler: app,
	}
	return rm.httpServer.ListenAndServe()
}
```



**实现try接口**

try接口来处理AP发送的`try`请求

```
// try接口
func (rm *RmServiceA) tryHandler(c *gin.Context) {
	// 事务信息
	gtid, branchID, request, err := rm.parseRequest(c)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	code := http.StatusOK

	// 调用SDK的HandleTry方法来屏蔽事务try细节，func(*gorm.DB)只需要实现业务逻辑
	err = tccrm.HandleTryOrm(c.Request.Context(), rm.db, gtid, branchID,
		func(tx *gorm.DB) error {
			// 更新银行账户
			// 条件：
			//   1. 账户余额足够：只有balance加上此转账动作大于等于0才可以执行；
			//   2. 冻结资金：且balance_freeze等于0来避免多个事务同时操作一个账户。具体如何冻结取决于业务
			// 更新balance_freeze为转账动作
			txr := tx.Model(BankAccount{}).
				Where("id=? AND balance_freeze=0 ", request.UserID).
				Update("balance_freeze", request.Account)

			if txr.Error != nil {
				code = http.StatusInternalServerError
				return txr.Error
			}
			// 如果不满足条件，则说明余额不足，或者有其他事务在操作此账户
			if txr.RowsAffected == 0 {
				code = http.StatusConflict
				return fmt.Errorf("insufficient balance or freeze != 0")
			}
			return nil
		})
	rm.handleErr(err, code)
}
```



**实现confirm接口**

confirm接口来处理TC发送的`confirm`请求

```

func (rm *TccRmBankService) confirmHandler(c *gin.Context) {
	gtid, branchID, request, err := rm.parseRequest(c)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	code := http.StatusOK

	// 调用SDK的HandleConfirm方法来屏蔽事务confirm细节，
	// func(*gorm.DB,string)只需要实现业务逻辑
	err = tccrm.HandleConfirmOrm(c.Request.Context(), rm.db, gtid, branchID,
		func(tx *gorm.DB) error {
			//
			// 将用户冻结资金加到账户余额中，同时清空冻结资金列
			txr := tx.Model(BankAccount{}).Where("id=?", request.UserID).
				Updates(map[string]interface{}{
					"balance":        gorm.Expr("balance+balance_freeze"),
					"balance_freeze": 0})
			if txr.Error != nil {
				code = http.StatusInternalServerError
				return txr.Error
			}
			if txr.RowsAffected == 0 {
				code = http.StatusInternalServerError
				return fmt.Errorf("internal error")
			}
			return nil

		})
	rm.handleErr(err, code)
}
```





**实现cancel接口**

来处理TC发送的`cancel`请求

```
func (rm *TccRmBankService) cancelHandler(c *gin.Context) {
	gtid, branchID, request, err := rm.parseRequest(c)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	code := http.StatusOK

	// 调用SDK的HandleCancel方法来屏蔽事务confirm细节，
	// func(*gorm.DB,string)只需要实现业务逻辑
	err = tccrm.HandleCancelOrm(c.Request.Context(), rm.db, gtid, branchID,
		func(tx *gorm.DB) error {
			// 取消事务，将冻结资金列清空
			txr := tx.Model(BankAccount{}).Where("id=?", request.UserID).
			Update("balance_freeze", 0)
			if txr.Error != nil {
				code = http.StatusInternalServerError
				return txr.Error
			}
			if txr.RowsAffected == 0 {
				code = http.StatusInternalServerError
				return fmt.Errorf("internal error")
			}
			return nil
		})
	rm.handleErr(err, code)
}
```


