# 测试

http_demo为http 服务器，来模拟`事务参与者`和`通知接口`，可以模拟成功，失败等等。

```
cd http_demo
go run main.go 
```


**生成saga ID**
```
curl http://localhost:8080/dtx/saga/gtid -XGET -v
```


**提交事务**
```
curl http://localhost:8080/dtx/saga/ -d '@./test/case1.json' -v
```


**查询事务**
```
curl http://localhost:8080/dtx/saga/$gtid -XGET -v
```

