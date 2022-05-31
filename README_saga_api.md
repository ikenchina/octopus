# API设计



## 1. TC APIs




### 1.1 获取 事务ID

获取Saga的global ID

**请求**

- 路径

```
GET http://octopus_domain/dtx/saga/gtid
```

**响应**

```
{"gtid":"gtid_125"}
```

- `gtid` : saga ID，类型`string`






### 1.2 提交事务

**请求**

- 路径

```
POST http://octopus_domain/dtx/saga $body
```

请在HTTP头携带此`DTX_GTID:gtid_125`header，方便负载均衡器将同一个事务的请求路由到同一个TC以减少事务的冲突概率。


- body

```
{
    "Gtid": "gtid_125",
    "Business": "test_biz",
    "Notify": {
        "URL": "http://localhost:8082/notify",
        "Timeout": 1000000000,  // 纳秒
        "Retry": 5000000000
    },
    "ExpireTime": "2100-02-20T17:51:46.686682918+08:00",
    "SagaCallType": "async",
    "Branches": [
        {
            "BranchId": "1",
            "Payload" : "{\"vip\":true}",
            "Commit" : {
                "Action" : "http://user/dtx/saga/gtid_101/1",
                "Timeout" : 1000000000,
                "Retry" : {
                    "MaxRetry" : 2,  // 总共尝试3次
                    "Constant" : {
                        "Duration" : 1000000000
                    }
                }
            },
            "Compensation" : {
                "Action" : "http://user/dtx/saga/gtid_101/1",
                "Timeout" : 1000000000,
                "Retry" : 1000000000
            }
        },
        {
            "BranchId": "2",
            "Payload" : "{\"id\":\"2\", \"msg\":true}",
            "Commit" : {
                "Action" : "http://msg/dtx/saga/gtid_101",
                "Timeout" : 1000000000  // 没有retry，则不重试
            },
            "Compensation" : {
                "Action" : "http://user/dtx/saga/gtid_101/1",
                "Timeout" : 1000000000,
                "Retry" : 1000000000
            }
        }
    ]
}
```

- Business : 相关业务部门
- Notify : Saga执行结果的通知接口，通知给`事务提交者`
- SagaCallType ： 同步/异步。如果是异步，则会马上返回给AP，再由AP去通过TC查询接口查询结果 或者 TC 来通知AP
- Branches ： 子事务
  - BranchId ： 子事务ID，一个Saga事务内保证唯一性
  - Payload ： commit和compensation的请求body
  - Commit : 
    - Action : commit URL，TC向RM发送 HTTP的`PUT`请求
    - Timeout : 超时时间，纳秒
    - Retry
      - MaxRetry ： 最多重试次数，`总执行次数等于MaxRetry+1`
      - Constant ： 重试策略
        - Duration ： 重试等待时间
  - Compensation
    - Action ： rollback URL，TC向RM发送 HTTP的`DELETE`请求
    - Timeout : 超时时间，纳秒
    - Retry ： 重试等待时间
  - Retry ： 子事务重试策略
    - MaxRetry ： 重试次数，`总执行次数=MaxRetry+1`
  - Compensation：补偿，相当于回滚。HTTP method是`DELETE`



**响应**

```
{
    "Gtid": "gtid_125",
    "State": "committed|aborted",
    "Branches": [
        {
            "BrandId": "1",
            "State": "committed",
            "Body": "{\"user\":123}"
        },
        {
            "BrandId": "2",
            "State": "committed",
            "Body": "{\"chat\":234}"
        }
    ],
    "Msg": ""
}
```

- Branches
  - Body : 子事务执行者返回的body
- Msg : 一般存放错误信息



### 1.3 查询

**请求**

- 路径

```
GET http://octopus_domain/dtx/saga/$saga_id
```

**响应**

```
{
  "Gtid" : "gtid_125",
  "State" : "committed",
  "Branches" : [
    {
      "BranchId" : "1",
      "State" : "committed",
      "Payload" : "{\"user\":123}"
    },
    {
      "BranchId" : "2",
      "State" : "committed",
      "Payload" : "{\"chat\":234}"
    }
  ]
}
```

- Payload : RM响应TC提交请求时的body



## 2. AP APIs

### 2.1 通知

如果`Saga事务提交者`(AP)需要`通知`，则需要实现通知接口：

**请求**

请求由TC 发送给 AP

- 路径

```
POST http://saga_request/$path
```

具体path由AP在请求中定义。

- body

响应body和提交请求里的响应body一样




## 3. RM APIs

### 3.1 提交

**请求**

- 路径

Saga事务请求中定义的`Action`，HTTP method为 `POST`，

- body

Saga事务请求中定义



**响应**

- HTTP status code  
  - 200 : 成功commit
  - 非200 ： 失败

- body

任意字符串，最终会返回给AP



### 3.2 补偿

**请求**

和提交几乎一样，都是用户在Saga请求中定义，只是HTTP method为`DELETE`。
如果补偿失败，则TC会一直尝试直到补偿成功。