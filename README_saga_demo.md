# Saga 开发演示



本文我们以发工资为例子，来演示演示如何开发一个saga分布式事务，公司给员工发工资组成一个Saga事务。

具体代码请参考：https://github.com/ikenchina/octopus/tree/master/demo/saga



## 角色

开发者只需要关心两个角色，如下

- 事务提交者AP：Saga事务的发起方。那对于发工资例子，那AP就是公司的服务。
- 事务参与者RM：事务的参与方。对于发工资例子，RM就是银行的服务。





## 事务提交者AP

直接使用octopus/client/saga下的wrapper.go的封装，调用其`SagaTransaction`方法来实现Saga事务。

如果AP需要TC事务结束后通知AP，则需要提供一个http接口给TC来回调。

`SagaTransaction`方法提交的Saga事务，对于子事务的重试策略是无限重试(若TC调用RM的commit失败，则会不断重试)。

```
// 发工资的Saga事务实现
func (app *Application) PayWage(employees []*AccountRecord) (*define.SagaResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	notifyAction := fmt.Sprintf("http://localhost%s/saga/notify", app.listen)
	transactionExpiredTime := time.Now().Add(1*time.Minute)

	//
	resp, err := saga_cli.SagaTransaction(ctx, tcDomain, transactionExpiredTime,
		func(t *saga_cli.Transaction, gtid string) error {

			// 在数据库中将saga事务存储起来，后面TC回调来通知事务状态时，可以查询来更新此事务
			app.saveGtidToDb(gtid)

			// 设置回调接口给TC来通知AP事务的最终态，
			// 也可以选择不通知，如果不通知，则需要定期查询TC事务的状态
			t.SetNotify(notifyAction, time.Second, time.Second)

			// 遍历所有员工
			for i, employee := range employees {
				// 给每个员工发工资都属于一个事务
				// actionURL是调用银行的URL
				branchID := i + 1
				actionURL := fmt.Sprintf("%s%s/%s/%d", app.employeeHosts[employee.UserID], service_BasePath, gtid, branchID)
				
				// 为Saga分布式事务添加子事务
				// commit和compensation使用actionURL同一个URL, 
				// http请求的body就是employee的Json序列化数据，数据会由TC放在commit的请求body中
				t.NewBranch(branchID, actionURL, actionURL, jsonMarshal(employee))
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

参考：`octopus/demo/saga/rm.go`



### 创建子事务表

如果是PostgreSQL数据库，则根据`octopus/rm/deployment/postgreSQL.sql`来创建。



### 实现RM



提供`commit`和`compensation`接口

- commit：以http的POST方式提供给TC
- compensation：以DELETE方式提供给TC

```
// RM service
type RmService struct {
	......
}

// http服务，提供commit和compensation接口
func (rm *RmService) start() error {
	app := gin.New()
	
	// commit接口以POST方式提供
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
	// 读取http请求body，反序列化为AccountRecord
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

	// 调用HandleCommit
	// gtid和branchID作为branch的全局id，body是commit的请求body
	// func(*gorm.DB)error 是RM实现commit的业务逻辑，此逻辑会在一个数据库事务中执行
	err = sagarm.HandleCommit(rm.Db, gtid, branchID, string(body),
		func(tx *gorm.DB) error {
		
			// 直接更新用户银行账号余额，给员工发工资
			txr := tx.Model(Account{}).Where("id=?", request.UserID).
				Update("balance", gorm.Expr("balance+?", request.Account))
			if txr.Error != nil {
				code = http.StatusInternalServerError
				return txr.Error
			}
			
			// 如果不存在，则说明用户不存在
			if txr.RowsAffected == 0 {
				code = http.StatusNotFound
				return fmt.Errorf("user does not exist")
			}
			return nil
		})
	
	// 如果事务执行失败，则返回错误，通知TC这个commit执行失败
	if err != nil {
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
	code := http.StatusOK

	// 调用HandleCompensation
	err := sagarm.HandleCompensation(rm.Db, gtid, branchID,
	
		// compensation实现的业务逻辑，逻辑会在一个事务中执行
		// body是commit请求时的body，由AP提供
		func(tx *gorm.DB, body string) error {
			// 反序列为AccountRecord
			record := AccountRecord{}
			err := json.Unmarshal([]byte(body), &record)
			if err != nil {
				code = http.StatusBadRequest
				return err
			}
			
			// 取消 commit 逻辑给员工银行账户添加的工资，
			txr := tx.Model(Account{}).Where("id=?", record.UserID).
				Update("balance", gorm.Expr("balance+?", -1 * request.Account))
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
		c.Status(code)
		_, _ = c.Writer.Write([]byte(err.Error()))
		return
	}
}
```

















