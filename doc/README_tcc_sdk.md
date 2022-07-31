- [TCC SDK](#tcc-sdk)
  - [事务提交者SDK](#事务提交者sdk)
    - [高级API](#高级api)
    - [原始API](#原始api)
  - [事务参与者SDK](#事务参与者sdk)


# TCC SDK


octopus提供SDK来方便用户实现AP，RM。

SDK分两个实现
- 事务提交者AP实现 : AP接口支持gRPC和HTTP两种协议，但建议使用gRPC接口
- 事务参与者RM实现 : RM可以提供gRPC或HTTP两种协议的接口

----

## 事务提交者SDK

实现代码 : github.com/ikenchina/octopus/ap/tcc



SDK分两种实现

- 原始 API ：直接调用TC接口，可以直接查看高级API
- 高级 API ：屏蔽各种TC接口细节，直接提供tcc事务函数给开发者

高级API较为简洁，我们先看高级API。



### 高级API 

高级API屏蔽了原始API的获取事务ID，提交事务，构建TccRequest等方法，整合成一个事务函数`TccTransaction`


```
func TccTransaction(ctx context.Context, cli *GrpcClient, expire time.Time,
	branches func(ctx context.Context, t *Transaction, gtid string) error) (*pb.TccResponse, error) 
```
- cli ： TC的gRPC client
- expire：Saga事务的过期时间，过期则TC会执行cancel
- branches ：开发者的业务逻辑实现，在此函数中，开发者可以调用`Transaction`来注册子事务
  - `t *Transaction`：tcc事务封装，使用此对象的`Try`方法来`try`子事务（向TC注册子事务，再调用RM的`try`接口）
  - `gtid string`：tcc事务ID，建议函数实现开始就将gtid存储到数据库中，因为如果事务提交给TC后，AP方崩溃了，那么AP将无法知道这个tcc事务的存在
  - 返回值：如果返回错误，则事务会进行回滚



**Try子事务**

添加一个子事务，其RM的接口是gRPC协议。
```
func (t *Transaction) TryGrpc(branchID int, conn *grpc.ClientConn,
	try string, confirm string, cancel string,
	request proto.Message,
	response interface{},
	opts ...func(o *options)) (err error)
```
- branchID ： 子事务ID，需要保证一个TCC事务内唯一
- conn ： rm的gRPC连接
- try ： RM的gRPC服务的try接口方法名
- confirm ： RM的gRPC服务的confirm接口方法名
- cancel ： RM的gRPC服务的cancel接口方法名
- request ： RM的gRPC服务的try、confirm和cancel接口的请求结构体
- response ： RM的gRPC服务的try接口的响应结构体



添加一个子事务，其RM的接口是HTTP协议。
```
func (t *Transaction) TryHttp(branchID int,
	try string, confirm string, cancel string,
	payload []byte,
	opts ...func(o *options)) ([]byte, error) 
```
- branchID ： 子事务ID，需要保证一个TCC事务内唯一
- try ： RM的try接口的URL，由`Transaction`来执行。使用http的POST方法。
- confirm：RM的confirm接口的URL，TC以http的PUT方法来调用
- cancel：RM的cancel接口的URL，TC以http的DELETE方法来调用
- payload：try接口的请求body





### 原始API 

Client对象，传入TcServer为Tc的服务器地址，如http://tc.test

```
type HttpClient struct {
	TcServer string
}
```



**获取一个全局唯一的global事务ID**

```
func (cli *HttpClient) NewGtid(ctx context.Context) (string, error) 
```



**Prepare事务**

prepare一个tcc事务

- 参数：
  - `req *define.TccRequest`：req中的branches应为空
- 返回值
  - `*define.TccResponse`：事务执行结果
  - `error`：事务执行错误

```
func (cli *HttpClient) Prepare(ctx context.Context, req *define.TccRequest) (*define.TccResponse, error)
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
func (cli *HttpClient) Register(ctx context.Context, gtid string, branch *define.TccBranch) (*define.TccResponse, error) 
```



**提交tcc事务**

当AP调用所有RM的`try`接口成功后，则需要调用此`Confirm`来通知TC提交事务，TC再调用RM的`confirm`接口来逐个提交RM子事务。

- 参数
  - `gtid string`
- 返回值
  - `*define.TccResponse`：事务提交状态
  - `error`：提交是否报错

```
func (cli *HttpClient) Confirm(ctx context.Context, gtid string) (*define.TccResponse, error) 
```





**回滚tcc事务**

当AP调用某RM的`try`接口失败，且不再进行重试时，则可以调用此`Cancel`接口来通知TC取消事务，TC再调用RM的`cancel`接口来逐个取消RM子事务。

```
func (cli *HttpClient) Cancel(ctx context.Context, gtid string) (*define.TccResponse, error) 
```





**查询事务信息**

AP可以调用此`Get`接口来查询tcc事务的信息。

```
func (cli *HttpClient) Get(ctx context.Context, gtid string) (*define.TccResponse, error) 
```





---



## 事务参与者SDK

实现代码 : github.com/ikenchina/octopus/rm/tcc

事务参与者，需要在本地数据库创建一个事务表来保存子事务的状态。

事务表请依照 octopus/rm/deployment下面的SQL语句来创建。



对于RM的实现，需要考虑请求乱序及子事务的创建、更新等等问题，SDK屏蔽了这些细节，只需要开发者实现相关业务逻辑即可。



**处理try请求**

```
func HandleTry(ctx context.Context, db *sql.DB, gtid string, branchID int,
	try func(tx *sql.Tx) error) error
```
- 参数
  - db : 子事务的数据库实例
  - gtid：tcc事务ID
  - branchID：子事务ID
  - try：子事务业务逻辑处理函数，
    - 参数tx是一个执行事务
    - 返回值error：如果非nil则代表执行失败，TC可能会进行重试(取决于设置的try次数)

- 返回值：error非空则try失败


HandleTryOrm和HandleTry功能一样，只是数据库实例使用gorm。
```
func HandleTryOrm(ctx context.Context, db *gorm.DB, gtid string, branchID int,
	try func(tx *gorm.DB) error) error
```


**处理commit请求**

```
func HandleConfirm(ctx context.Context, db *sql.DB, gtid string, branchID int,
	confirm func(*sql.Tx) error) error
```
- 参数
  - db
  - gtid
  - branchID
  - confirm：提交子事务业务逻辑，
- 返回值：error代表confirm执行失败，TC会不断重试知道成功



**处理cancel请求**

```
func HandleConfirmOrm(ctx context.Context, db *gorm.DB, gtid string, branchID int,
	confirm func(tx *gorm.DB) error) error
```
- 参数
  - db
  - gtid
  - branchID
  - cancel：提交子事务业务逻辑
- 返回值：error代表cancel执行失败，TC会不断重试知道成功


