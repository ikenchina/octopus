- [部署](#部署)
  - [部署TC](#部署tc)
    - [初始化数据库](#初始化数据库)
    - [部署TC二进制](#部署tc二进制)
    - [TC配置](#tc配置)
    - [TC监控](#tc监控)
  - [部署RM](#部署rm)


# 部署


## 部署TC


### 初始化数据库

先部署好TC所需要的数据库，按照部署文档 github.com/ikenchina/octopus/tc/deployment/README.md


### 部署TC二进制


**运行TC**

编译好tc二进制后，直接部署即可，    
如
```
./tc --config ./config.json
```

或者    
使用systemd管理，见github.com/ikenchina/octopus/tc/deployment/README.md



**停止tc**
```
pkill tc
```
或者     
发送http请求使tc退出
```
curl -XDELETE http://localhost:18089/debug/healthcheck 
```


**健康检查**    

节点健康检查可以通过端口探活来实现，或者通过http请求，返回200即是健康节点
```
curl -XGET http://localhost:18089/debug/healthcheck 
```



### TC配置

其中Node信息可以通过环境变量传入，分别为环境变量`OCTOPUS_TC_DATACENTER_ID`和`OCTOPUS_TC_NODE_ID`。
```
{
    "Node": {                                          // TC节点信息
        "NodeId": 2,                                   // 节点ID
        "DataCenterId": 1                              // 数据中心ID
    },
    "GrpcListen": ":18080",                            // gRPC服务监听的地址
    "HttpListen": ":18089",                            // http服务监听的地址，支持http接口，同时prometheus数据也通过此地址导出
    "MaxConcurrentTask": 1000,                         // 最大并发处理任务数
    "MaxConcurrentBranch": 2000,                       // 最大并发处理子事务数
    "Storages": {                                      // 数据库信息
        "tcc": {                                       // TCC事务的数据库信息
            "Driver": "postgresql",                    // postgresql数据库
            "Dsn": "postgresql://dtx_user:dtx_pass@10.184.21.16:5432/dtx?connect_timeout=3",      // 数据库 DSN
            "MaxConnections": 5,                       // 最大连接数
            "MaxIdleConnections": 5,                   // 最大idle连接数
            "Timeout": 1000000000,                     // 超时时间，类型是time.Duration
            "CleanExpired": 10000000000000,            // 对多久之前的已提交或已回滚事务进行清理，类型time.Duration
            "CheckExpiredDuration": 3000000000,        // 清理时间间隔，类型time.Duration
            "CheckLeaseExpiredDuration": 3000000000    // 检查租约过期时间间隔，类型time.Duration
        },
        "saga": {                                      // saga事务的数据库信息
            "Driver": "postgresql",
            "Dsn": "postgresql://dtx_user:dtx_pass@10.184.21.16:5432/dtx?connect_timeout=3",
            "Timeout": 1000000000,
            "MaxConnections": 5,
            "MaxIdleConnections": 5,
            "CleanExpired": 10000000000000,
            "CheckExpiredDuration": 3000000000,
            "CheckLeaseExpiredDuration": 3000000000
        }
    },
    "Log": {                                           // 日志，zap日志的Config
        "level": "info",
        "development": true,
        "disableCaller": false,
        "disableStacktrace": false,
        "encoding": "json",
        "outputPaths": [
            "stdout"
        ],
        "errorOutputPaths": [
            "stderr"
        ],
        "encoderConfig": {
            "messageKey": "msg",
            "levelKey": "level",
            "levelEncoder": "lowercase",
            "SkipLineEnding": false
        }
    }
}

```


### TC监控

如果配置中配置了"HttpListen"，则可以通过`/debug/metrics`来导出prometheus的数据。

如果使用grafana，可以直接导入 `octopus/tc/deployment/grafana-tc.json`。


## 部署RM


如果开发者不使用rm package提供的`Handle*`系列方法，则不需要创建子事务表。    
开发者可以自己实现`Handle*`相关的事务逻辑，只需要保证幂和避免乱序等异常即可(根据gtid和branch id)。 

如果开发者使用`Handle*`系列方法，则需要创建子事务表。    
如果是PostgreSQL数据库，则根据`octopus/rm/deployment/postgreSQL.sql`来创建。

建议开发者使用`Handle*`系列方法，因为其处理了乱序等异常情况。






