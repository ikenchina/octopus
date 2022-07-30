# API设计



## TC APIs



### 获取 事务ID

获取事务的global ID

**请求**

- 路径

```
GET http://octopus/dtx/tcc/gtid
```

**响应**

```
{"gtid":"gtid_125"}
```

- `gtid` : TCC ID，类型`string`



### 开启TCC事务

**请求**

- 路径

```
POST http://octopus/dtx/tcc
```

- body

```
{
	"Gtid" : "gtid_125",
	"Business" : "bank",
	"ExpireTime" : "2100-02-20T17:51:46.686682918+08:00"
}
```



**响应**

- 响应码：200代表成功，其他代表失败





### 注册子事务

**请求**

- 路径

```
POST http://octopus/dtx/tcc/gtid_125
```

请在HTTP头携带此`DTX_GTID:gtid_125`header，方便负载均衡器将同一个事务的请求路由到同一个TC以减少事务的冲突概率。

- body

```
{
	"Gtid" : "gtid_125",
	"Branches" : [
		{
			"BranchId" : "1",
			"ActionConfirm" : "http://octopus/dtx/tcc/gtid_125/1",
			"ActionCancel" : "http://octopus/dtx/tcc/gtid_125/1",
			"Timeout" : 1000000000,
			"Retry" : 1000000000
		}
	]
}
```

- BranchId : 子事务ID，字符串，确保TCC事务内唯一性即可
- ActionConfirm ： 执行子事务Confirm请求的路径，TC会以HTTP PUT的方式请求RM的ActionConfirm接口
- ActionCancel ： 执行子事务Cancel请求的路径，TC会以HTTP DELETE的方式请求RM的ActionCancel接口
- Timeout：Confirm和Cancel的请求超时时间
- Retry：Confirm和Cancel的重试时间间隔





### 提交事务

**请求**

- 路径

```
PUT http://octopus/dtx/tcc $body
```

请在HTTP头携带此`DTX_GTID:gtid_125`header，方便负载均衡器将同一个事务的请求路由到同一个TC以减少事务的冲突概率。


- body：无





### 取消事务

**请求**

- 路径

```
DELETE http://octopus/dtx/tcc $body
```

请在HTTP头携带此`DTX_GTID:gtid_125`header，方便负载均衡器将同一个事务的请求路由到同一个TC以减少事务的冲突概率。


- body：无





### 查询

**请求**

- 路径

```
GET http://octopus/dtx/tcc/gtid
```

**响应**

```
{
	"Gtid" : "gtid_125",
	"State" : "committed",
	"Branches" : [
		{
			"BranchId" : "1",
			"State" : "committed"
		}
	]
}
```




----



## RM APIs

### Try API

**请求**

- 路径

- body

RM的Try API由RM自行设计，由AP负责调用





### Confirm API

**请求**

- 路径：由AP通过TC的`register`接口告知TC，TC通过HTTP的`PUT`请求来调用，Confirm API的路径最好遵循以下规则

```
PUT http://octopus/dtx/tcc/$gtid/$branch_id
```

- 响应：
  - 响应码：200代表成功，其他代表失败



### Cancel API

**请求**

- 路径：由AP通过TC的`register`接口告知TC，TC通过HTTP的`DELETE`请求来调用，Cancel API的路径最好遵循以下规则

```
PUT http://octopus/dtx/tcc/$gtid/$branch_id
```

- 响应：
  - 响应码：200代表成功，其他代表失败




