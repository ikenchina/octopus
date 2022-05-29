# Saga SDK


octopus提供SDK来方便用户实现AP，RM。

SDK分两个实现
- 事务提交者AP实现
- 事务参与者RM实现

-----

## 事务提交者SDK

实现代码 : github.com/ikenchina/octopus/client/saga



SDK分两种实现

- 原始 API ：直接调用TC接口
- 高级 API ：屏蔽各种TC接口细节，直接提供saga事务函数给开发者



### 原始API 

Client对象，传入TcServer为Tc的服务器地址，如http://saga-tc.test

```
type Client struct {
	TcServer string
}
```



**获取一个全局唯一的global事务ID**

```
func (cli *Client) NewGtid(ctx context.Context) (string, error) 
```



**提交saga事务**

- `define.SagaRequest：saga`事务及子事务(分支)的请求结构

- `define.SagaResponse：saga`事务的执行结果

```
func (cli *Client) Commit(ctx context.Context, req *define.SagaRequest) (*define.SagaResponse, error)
```





### 高级API 

高级API屏蔽了原始API的获取事务ID，提交事务，构建SagaRequest等方法，整合成一个事务函数`SagaTransaction`

- tcServer：TC的服务器地址
- expire：Saga事务的过期时间，过期则TC会执行compensation
- branches：开发者的业务逻辑实现，在此函数中，开发者可以添加子事务，设置事务通知
  - `t *Transaction`：saga事务封装，可以添加子事务，设置回调通知等
  - `gtid string`：saga事务ID，建议函数实现开始就将gtid存储到数据库中，因为如果事务提交给TC后，AP方崩溃了，而此saga事务没有设置回调通知，那么AP将无法知道这个saga事务的存在
  - 返回值：如果返回错误，则事务不会提交

```
func SagaTransaction(ctx context.Context, tcServer string, expire time.Time,
	branches func(t *Transaction, gtid string) error) (*define.SagaResponse, error) 
```



**添加子事务**

- branchID：子事务ID，需要保证一个saga事务内唯一
- commitAction：RM提供给TC的commit接口的URL，TC以http POST方法调用
- compensationAction：RM提供给TC的compensation接口的URL，TC以http DELETE方法调用
- payload：commit接口的请求body，TC调用RM的commit http URL的请求body

```
func (t *Transaction) NewBranch(branchID int, commitAction string, compensationAction string, payload string)
```



**设置事务通知回调**

当saga事务执行完成后，TC调用此接口来通知AP事务的执行结果

- action：AP提供给TC的通知接口，TC以http POST方式调用
- timeout：接口超时时间
- retry：接口重试时间

```
func (t *Transaction) SetNotify(action string, timeout, retry time.Duration)
```



---



## 事务参与者SDK

实现代码 : github.com/ikenchina/octopus/rm/saga

事务参与者，需要在本地数据库创建一个事务表来保存子事务的状态。

事务表请依照 octopus/rm/deployment下面的SQL语句来创建。



对于RM的实现，需要考虑请求乱序及子事务的创建、更新等等问题，SDK屏蔽了这些细节，只需要开发者实现相关业务逻辑即可。



**处理commit请求**

- db : 子事务的数据库实例，暂时只支持gorm
- gtid：saga事务ID
- branchID：子事务ID
- requestBody：子事务请求body
- commit：业务逻辑处理函数，
  - 函数签名的`*gorm.DB`是一个执行事务
  - 返回值error：如果非nil则代表执行失败，TC可能会进行重试

```
func HandleCommit(db *gorm.DB, gtid string, branchID int, requestBody string, 
	commit func(*gorm.DB) error) error 
```



**处理compensation请求**

- db : 子事务的数据库实例，暂时只支持gorm
- gtid：saga事务ID
- branchID：子事务ID
- compensate：业务逻辑处理函数，
  - 函数签名的`*gorm.DB`是一个执行事务
  - 函数签名的`string`是commit请求时的请求body
  - 返回值error：如果非nil则代表失败，TC会进行重试直到成功

```
func HandleCompensation(db *gorm.DB, gtid string, branchID int, 
	compensate func(*gorm.DB, string) error) error 
```





