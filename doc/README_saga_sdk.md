- [Saga SDK](#saga-sdk)
	- [事务提交者SDK](#事务提交者sdk)
		- [高级API](#高级api)
		- [原始API](#原始api)
			- [gRPC API](#grpc-api)
			- [http API](#http-api)
	- [事务参与者SDK](#事务参与者sdk)

# Saga SDK


octopus提供SDK来方便用户实现AP，RM。

SDK分两个实现
- 事务提交者AP实现 : AP接口支持gRPC和HTTP两种协议，但建议使用gRPC接口
- 事务参与者RM实现 : RM可以提供gRPC或HTTP两种协议的接口

-----

## 事务提交者SDK

实现代码 : github.com/ikenchina/octopus/ap/saga


SDK分两种实现
- 原始 API ：直接调用TC接口
- 高级 API ：屏蔽各种TC接口细节，直接提供saga事务函数给开发者


高级API较为简洁，我们先看高级API。


### 高级API 

我们只看AP的gRPC的高级API。

高级API屏蔽了一些交互细节，如原始API的获取事务ID，提交事务，构建SagaRequest等方法，整合成一个事务函数`SagaTransaction`


```
func SagaTransaction(ctx context.Context, cli *GrpcClient, expire time.Time,
	branches func(t *Transaction, gtid string) error) (*pb.SagaResponse, error)
```
- cli ： TC的gRPC client
- expire：Saga事务的过期时间，过期则TC会执行compensation
- branches：开发者的业务逻辑实现，在此函数中，开发者可以添加子事务，设置事务通知
  - `t *Transaction`：saga事务封装，可以添加子事务，设置回调通知等
  - `gtid string`：saga事务ID，建议函数实现开始就将gtid存储到数据库中，因为如果事务提交给TC后，AP方崩溃了，而此saga事务没有设置回调通知，那么AP将无法知道这个saga事务的存在
  - 返回值：如果返回错误，则事务不会提交


**添加子事务**

添加一个子事务，其RM的接口是gRPC协议。
```
func (t *Transaction) AddGrpcBranch(branchID int, rmServer string,
	commitAction string, compensationAction string,
	payload proto.Message,
	opts ...branchFunctions)
```
- branchID ： 子事务ID，需要保证一个saga事务内唯一
- rmServer ： rm的gRPC server地址
- commitAction ： gRPC server的commit method名
- compensationAction ： gRPC server的compensation method名
- payload ： commit和compensation的请求体


添加一个子事务，其RM的接口是HTTP协议。
```
func (t *Transaction) AddHttpBranch(branchID int,
	commitAction string, compensationAction string,
	payload []byte,
	opts ...branchFunctions)
```
- branchID：子事务ID，需要保证一个saga事务内唯一
- commitAction：RM提供给TC的commit接口的URL，TC以http POST方法调用
- compensationAction：RM提供给TC的compensation接口的URL，TC以http DELETE方法调用
- payload：commit接口的请求body，TC调用RM的commit http URL的请求body




**设置事务通知回调**

当TC完成saga事务提交后，可以设置一个回调接口，让TC来通知AP。


如果AP的回调接口是gRPC协议的，则
```
func (t *Transaction) SetGrpcNotify(grpcServer string, action string, timeout, retry time.Duration) 
```
- grpcServer ：AP提供的回调接口的gRPC server地址
- action ： 回调接口的gRPC method
- timeout ： 接口超时时间
- retry ： 接口重试时间

TC会一直调用回调接口直到接口返回成功。

`octopus/define/proto/saga/ap.proto`的`ApNotifyRequest`定义了此notify接口的request结构。


如果是HTTP协议，则接口须提供POST方法，`octopus/define/saga.go`的`SagaResponse`定义了请求体。




### 原始API 


#### gRPC API

创建一个AP的grpc client
```
func NewGrpcClient(target string) (*GrpcClient, error)
```
- target ： AP的gRPC地址


**分配一个saga事务全局ID**

```
NewGtid(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*SagaResponse, error)
```


**提交saga事务**
```
Commit(ctx context.Context, in *SagaRequest, opts ...grpc.CallOption) (*SagaResponse, error)
```


**获取saga事务信息**
```
Get(ctx context.Context, in *SagaRequest, opts ...grpc.CallOption) (*SagaResponse, error)
```



#### http API

Client对象，传入TcServer为Tc的服务器地址，如http://saga-tc.test

```
type HttpClient struct {
	TcServer string
}
```



**获取一个全局唯一的saga事务ID**

```
func (cli *HttpClient) NewGtid(ctx context.Context) (string, error) 
```



**提交saga事务**

- `define.SagaRequest：saga`事务及子事务(分支)的请求结构

- `define.SagaResponse：saga`事务的执行结果

```
func (cli *HttpClient) Commit(ctx context.Context, req *define.SagaRequest) (*define.SagaResponse, error)
```





---



## 事务参与者SDK

实现代码 : github.com/ikenchina/octopus/rm/saga

事务参与者，需要在本地数据库创建一个事务表来保存子事务的状态。

事务表请依照 octopus/rm/deployment下面的SQL语句来创建。



对于RM的实现，需要考虑请求乱序及子事务的创建、更新等等问题，SDK屏蔽了这些细节，只需要开发者实现相关业务逻辑即可。



**处理commit请求**

```
func HandleCommit(ctx context.Context, db *sql.DB, gtid string, branchID int,
	commit func(*sql.Tx) error) error 
```
- db : 开发者传入数据库实例
- gtid：saga事务ID
- branchID：子事务ID
- commit：业务逻辑处理函数
  - 参数*sql.Tx是一个数据库事务
  - 返回值error：如果非nil则代表执行失败，TC可能会进行重试



```
func HandleCommitOrm(ctx context.Context, db *gorm.DB, gtid string, branchID int, requestBody string, 
	commit func(*gorm.DB) error) error 
```
和HandleCommit功能一样，只是数据库使用gorm。



**处理compensation请求**

```
func HandleCompensation(ctx context.Context, db *sql.DB, gtid string, branchID int,
	compensate func(*sql.Tx) error) error 
```
- db : 开发者传入数据库实例
- gtid：saga事务ID
- branchID：子事务ID
- compensate：业务逻辑处理函数







