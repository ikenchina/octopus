# TCC 开发演示



本文我们以银行转账为例子，来演示演示如何开发一个TCC分布式事务。

具体代码请参考：https://github.com/ikenchina/octopus/tree/master/demo/tcc

---

## 角色

开发者只需要关心两个角色，如下

- 事务提交者AP：TCC事务的发起方。对于转账的例子，AP就属于银联的服务
- 事务参与者RM：事务的参与方。对于转账例子，RM就是账户所属的银行的服务。


----


## 事务提交者AP

直接使用octopus/client/tcc下的wrapper.go的封装，调用其`TccTransaction`方法来实现TCC事务。



`TccTransaction`方法提交的TCC事务

```
// 转账
// userID转账 account金额给otherUserID
func (app *Application) Transfer(userID, otherUserID int, account int) (*define.TccResponse, error) {
	tccCtx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	
	// 事务过期时间，超过此时间，如果事务仍然没有提交，则TC会取消事务
	transactionExpireTime := time.Now().Add(time.Second)

	// 直接调用TccTransaction来实现Tcc事务
	tccResp, err := tcc_cli.TccTransaction(tccCtx, tcDomain, transactionExpireTime,
	
		// 事务的业务逻辑实现
		// tcc_cli.Transaction 表示一个TCC事务对象
		func(t *tcc_cli.Transaction, gtid string) error {

			// 将gtid持久化，方便后续查询，也避免事务提交TC时崩溃(可能提交成功也可能失败)，再重启不知道此事务的存在
			app.saveGtidToDb(gtid)

			// 尝试调用账户A的所属银行服务器的try接口
			branchId := 1
			actionAURL := fmt.Sprintf("%s%s/%s/%d", app.rmAHost, service_BasePath, gtid, branchId)
			
			// 调用tcc_cli.Transaction的Try方法，会将此子事务注册到TC，再调用RM的try接口
			// tryAresp为RM的try接口返回的响应body
			tryAresp, err := t.Try(branchId, actionAURL, actionAURL, actionAURL, jsonMarshal(&AccountRecord{
				UserID:  userID,
				Account: -1 * account,
			}))
			if err != nil {
				return err
			}

			// 尝试调用账户B所属银行服务器的try接口
			branchId++
			actionBURL := fmt.Sprintf("%s%s/%s/%d", app.rmBHost, service_BasePath, gtid, branchId)
			tryBresp, err := t.Try(branchId, actionBURL, actionBURL, actionBURL, jsonMarshal(&AccountRecord{
				UserID:  otherUserID,
				Account: account,
			}))
			if err != nil {
				return err
			}
			app.saveResponse(tryAresp)
			app.saveResponse(tryBresp)

			return nil
		})

	// 更新TCC事务状态
	app.updateTccStateToDb(tccResp.Gtid, tccResp.State)
	return tccResp, err
}


```


---


## 事务参与者RM

RM由员工账户所属银行来实现。

RM需要提供 `try`、`confirm`和`cancel`接口来执行、提交和取消子事务。

直接使用octopus/rm/tcc下的wrapper.go的`HandleTry`、`HandleConfirm`和`HandleCancel`来实现接口。

参考：`octopus/demo/tcc/rm.go`



### 创建子事务表

如果是PostgreSQL数据库，则根据`octopus/rm/deployment/postgreSQL.sql`来创建。



### 实现RM



RM提供的接口

- try：提供给AP调用，AP以http的POST方式来调用
- confirm：提供给TC调用，TC以http的PUT方式来调用
- cancel：提供给TC调用，TC以http的DELETE方式来调用

```
// RM service
type RmServiceA struct {
	......
}

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
	body, err := c.GetRawData()
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	request := &AccountRecord{}
	err = json.Unmarshal(body, request)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	gtid := c.Param("gtid")
	branchID, _ := strconv.Atoi(c.Param("branch_id"))
	code := http.StatusOK

	//
	// 调用SDK的HandleTry方法来屏蔽事务try细节，func(*gorm.DB)只需要实现业务逻辑
	//
	err = tccrm.HandleTry(c.Request.Context(), rm.Db, gtid, branchID, string(body),
		// 业务逻辑
		func(tx *gorm.DB) error {
			// 更新银行账户
			// 条件：
			//   1. 账户余额足够：只有balance加上此转账动作大于等于0才可以执行；
			//   2. 冻结资金：且balance_freeze等于0来避免多个事务同时操作一个账户。具体如何冻结取决于业务
			// 更新balance_freeze为转账动作
			txr := tx.Model(Account{}).Where("id=? AND balance_freeze=0 AND balance+?>=0",
				request.UserID, request.Account).
				Update("balance_freeze", request.Account)
			if txr.Error != nil {
				code = http.StatusInternalServerError
				return txr.Error
			}
			
			// 如果不满足条件，则说明余额不足，或者有其他事务在操作此账户
			if txr.RowsAffected == 0 {
				code = http.StatusConflict
				return fmt.Errorf("insufficient balance or freeze != 0") // avoid insufficient balance or concurrency
			}
			return nil
		})
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



**实现confirm接口**

confirm接口来处理TC发送的`confirm`请求

```
func (rm *RmServiceA) confirmHandler(c *gin.Context) {
	gtid := c.Param("gtid")
	branchID, _ := strconv.Atoi(c.Param("branch_id"))
	code := http.StatusOK

	// 调用SDK的HandleConfirm方法来屏蔽事务confirm细节，
	// func(*gorm.DB,string)只需要实现业务逻辑
	err := tccrm.HandleConfirm(c.Request.Context(), rm.Db, gtid, branchID,
		func(tx *gorm.DB, tryBody string) error {
			record := AccountRecord{}
			err := json.Unmarshal([]byte(tryBody), &record)
			if err != nil {
				code = http.StatusBadRequest
				return err
			}
			
			// 将用户冻结资金加到账户余额中，同时清空冻结资金列
			// 
			txr := tx.Model(Account{}).Where("id=?", record.UserID).
				Updates(map[string]interface{}{
					"balance":        gorm.Expr("balance+?", record.Account),
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



**实现cancel接口**

来处理TC发送的`cancel`请求

```
// 
func (rm *RmServiceA) cancelHandler(c *gin.Context) {
	gtid := c.Param("gtid")
	branchID, _ := strconv.Atoi(c.Param("branch_id"))
	code := http.StatusOK

	// 调用SDK的HandleCancel方法来屏蔽事务confirm细节，
	// func(*gorm.DB,string)只需要实现业务逻辑
	err := tccrm.HandleCancel(c.Request.Context(), rm.Db, gtid, branchID,
		func(tx *gorm.DB, tryBody string) error {
			record := AccountRecord{}
			err := json.Unmarshal([]byte(tryBody), &record)
			if err != nil {
				code = http.StatusBadRequest
				return err
			}

			// 取消事务，将冻结资金列清空
			txr := tx.Model(Account{}).Where("id=?", record.UserID).Update(
				"balance_freeze", 0)
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

