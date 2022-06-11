# TCC SDK



SDK分两个实现

- 事务提交者AP实现
- 事务参与者RM实现


----

## 事务提交者SDK

实现代码 : github.com/ikenchina/octopus/client/tcc



SDK分两种实现

- 原始 API ：直接调用TC接口，可以直接查看高级API
- 高级 API ：屏蔽各种TC接口细节，直接提供tcc事务函数给开发者



### 原始API 

Client对象，传入TcServer为Tc的服务器地址，如http://tc.test

```
type Client struct {
	TcServer string
}
```



**获取一个全局唯一的global事务ID**

```
func (cli *Client) NewGtid(ctx context.Context) (string, error) 
```



**Prepare事务**

prepare一个tcc事务

- 参数：
  - `req *define.TccRequest`：req中的branches应为空
- 返回值
  - `*define.TccResponse`：事务执行结果
  - `error`：事务执行错误

```
func (cli *Client) Prepare(ctx context.Context, req *define.TccRequest) (*define.TccResponse, error)
```



**注册子事务**

注册子事务到TC，AP应该先注册子事务，再调用RM的`try`方法。

- 参数
  - `gtid string`：tcc事务 ID
  - `branch *define.TccBranch`：子事务
- 返回值
  - `*define.TccResponse`：tcc事务，包含已经注册的子事务和事务状态
  - `error：是否注册失败`

```
func (cli *Client) Register(ctx context.Context, gtid string, branch *define.TccBranch) (*define.TccResponse, error) 
```



**提交tcc事务**

当AP调用所有RM的`try`接口成功后，则需要调用此`Confirm`来通知TC提交事务，TC再调用RM的`confirm`接口来逐个提交RM子事务。

- 参数
  - `gtid string`
- 返回值
  - `*define.TccResponse`：事务提交状态
  - `error`：提交是否报错

```
func (cli *Client) Confirm(ctx context.Context, gtid string) (*define.TccResponse, error) 
```





**回滚tcc事务**

当AP调用某RM的`try`接口失败，且不再进行重试时，则可以调用此`Cancel`接口来通知TC取消事务，TC再调用RM的`cancel`接口来逐个取消RM子事务。

```
func (cli *Client) Cancel(ctx context.Context, gtid string) (*define.TccResponse, error) 
```





**查询事务信息**

AP可以调用此`Get`接口来查询tcc事务的信息。

```
func (cli *Client) Get(ctx context.Context, gtid string) (*define.TccResponse, error) 
```





### 高级API 

高级API屏蔽了原始API的获取事务ID，提交事务，构建TccRequest等方法，整合成一个事务函数`TccTransaction`

- tcServer：TC的服务器地址
- expire：Saga事务的过期时间，过期则TC会执行compensation
- tryFunctions：开发者的业务逻辑实现，在此函数中，开发者可以调用`Transaction`来注册子事务
  - `t *Transaction`：tcc事务封装，使用此对象的`Try`方法来`try`子事务（向TC注册子事务，再调用RM的`try`接口）
  - `gtid string`：tcc事务ID，建议函数实现开始就将gtid存储到数据库中，因为如果事务提交给TC后，AP方崩溃了，那么AP将无法知道这个tcc事务的存在
  - 返回值：如果返回错误，则事务不会提交

```
func TccTransaction(ctx context.Context, tcServer string, expire time.Time, 
	tryFunctions func(t *Transaction, gtid string) error) (*define.TccResponse, error) 
```



**Try子事务**

- branchID：子事务ID，需要保证一个tcc事务内唯一
- try：RM提供给AP调用的try接口的URL，由`Transaction`来执行。使用http的POST方法。
- confirm：RM提供给TC调用的confirm接口的URL，TC以http的PUT方法来调用
- cancel：RM提供给TC调用的cancel接口的URL，TC以http的DELETE方法来调用
- payload：try接口的请求body

```
func (t *Transaction) Try(branchID int, try string, confirm string, cancel string, payload string, opts ...func(o *Options)) ([]byte, error) 
```





---



## 事务参与者SDK

实现代码 : github.com/ikenchina/octopus/rm/tcc

事务参与者，需要在本地数据库创建一个事务表来保存子事务的状态。

事务表请依照 octopus/rm/deployment下面的SQL语句来创建。



对于RM的实现，需要考虑请求乱序及子事务的创建、更新等等问题，SDK屏蔽了这些细节，只需要开发者实现相关业务逻辑即可。



**处理try请求**

- 参数
  - db : 子事务的数据库实例，暂时只支持gorm
  - gtid：tcc事务ID
  - branchID：子事务ID
  - tryBody：子事务try请求body
  - try：子事务业务逻辑处理函数，
    - 函数签名的`*gorm.DB`是一个执行事务
    - 返回值error：如果非nil则代表执行失败，TC可能会进行重试

- 返回值：error非空则try失败

```
func HandleTry(ctx context.Context, db *gorm.DB, gtid string, branchID int, tryBody string, 
	try func(stx *gorm.DB) error) error 
```



**处理commit请求**

- 参数
  - db
  - gtid
  - branchID
  - confirm：提交子事务业务逻辑，tryBody为执行try接口时的请求body
- 返回值：error代表confirm执行失败，TC会不断重试知道成功

```
func HandleConfirm(ctx context.Context, db *gorm.DB, gtid string, branchID int, 
	confirm func(stx *gorm.DB, tryBody string) error) error
```



**处理cancel请求**

- 参数
  - db
  - gtid
  - branchID
  - cancel：提交子事务业务逻辑，tryBody为执行try接口时的请求body
- 返回值：error代表cancel执行失败，TC会不断重试知道成功

```
func HandleCancel(ctx context.Context, db *gorm.DB, gtid string, branchID int, 
	cancel func(stx *gorm.DB, tryBody string) error) error 
```







