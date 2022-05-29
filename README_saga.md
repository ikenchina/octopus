# Orchestration-based saga



## 理论



**论文**

Hector Garcia-Molina和Kenneth Salem在1987年发表Saga论文 [《SAGAS》](https://www.cs.princeton.edu/research/techreps/TR-070-87)，提出了Saga的概念：一个长事务，Long lived transactions (LLTs)。

> A LLT is a saga if it can be written as a sequence of transactions that can be interleaved with other transactions. 



Saga既然定位为一个长生命周期的事务，所以需要执行的操作(也叫分支事务或子事务)就可能非常多，可能执行长达小时级或者天级别。   



**理论**

核心思想是将长事务拆分为多个短事务，由Saga事务协调器协调，如果每个子事务都正常结束那saga事务则成功完成提交，如果某个步骤失败或过期导致需要回滚，则根据相反顺序对每个子事务执行一次补偿操作。   
每个子事务需要满足数据库的一致性，而Saga事务则是relaxing atomic，失败则通过补偿来回滚子事务，来实现最终的一致性。所以整个saga事务的隔离性比较差，但是子事务是满足数据库隔离性的。


> Saga的子事务的执行不需要锁定资源或预留资源，而是通过补偿的方式来进行类似于事务的"回滚"操作，来达到最终一致性。所以相对其他分布式事务来说虽然隔离性差，但是并发更高，也就适合做长生命周期的事务。     



**举例**

举个转账的例子：A和B账户余额都是100元，A转账30元给B。  
这个Saga事务有两个子事务，

- 子事务A：从A的账户减去30元，提交后A的账户余额为70元
- 子事务B：给B的账户添加30元，提交后B的账户余额为130元

如果子事务A执行成功，而子事务B执行失败，那么只需要对子事务A进行补偿即可：给A的账号添加30元，最终余额是100元。事务执行期间，A的账户余额是70元，所以隔离性差(相当于读未提交的隔离级别)，但是不会阻塞资源，性能相对较好。



**使用场景**   
适用于
- 业务流程长、子事务多的分布式事务
- 对隔离性没有要求的事务


**优点**
- 并发度高，不用像XA事务长期锁定资源或TCC对资源进行预留
- 一阶段提交本地事务，无锁，高性能
- 补偿服务实现较为容易



**缺点**

- 不保证隔离性
- 参与者需要实现幂等


---------------

## 设计


### 基础概念

Saga事务由三种角色组成

- Saga事务提交者：事务发起者，在分布式事务中统称为AP
- Saga事务协调者：接受AP的事务请求，管理Saga事务，也称为`Saga Execution Coordinator (SEC)`，在分布式事务中统称为事务协调者(TC : Transaction Coordinator)
- Saga子事务参与者：协调者将子事务提交给参与者执行， 在分布式事务中统称为资源管理器(RM：Resource Manager)。参与者需要包证接口的幂等性



事务状态

- committed : 已经提交。最终态
- aborted ： 已经回滚。最终态
- failed ： 回滚中或提交失败(可能重试中)。非终态
- prepared：提交中。非终态



### Saga事务协调者的实现

**Saga Service**

- 提供RESTful API，AP通过API进行事务提交或查询
- Saga service将请求封装成Saga事务对象，提交给Saga Executor执行
- API结果响应：
  - 如果是同步请求，则等待Saga Executor处理完成，再将结果封装成`SagaResponse`给AP
  - 如果是异步请求，则马上返回给AP
- 通知：TC通知 AP Saga事务执行的结果



**Saga Executor**

- Saga事务真正的执行者，管理Saga事务的状态转换，重试，持久化等等，不涉及与RM和AP的交互
- Saga子事务处理和回滚：Saga Executor并不处理子事务，而是将子事务操作写入`Channel`，由外层去向RM发送子事务请求
- Saga事务通知：Saga Executor不执行具体的通知操作，而是将操作写入`Channel`，由外层去执行AP
- 数据持久化：Saga Executor将Saga数据存储到RDBMS中来保证数据的持久化



### 异常情况处理

分布式事务实现的一个难点就是时序问题，主要体现在：

- 服务器的时钟不同步
- 请求乱序

因此会产生一些不可预测的异常。



**回滚异常**

异常流程如下：

- TC向RM提交子事务：commit操作
- 由于网络原因commit操作仍然处于发送中，没有到达RM
- TC等待请求响应超时，于是向RM发送rollback操作
- 异常点1 ：RM收到rollback操作，发现没有执行过此事务，产生异常
- 异常点2 ：当RM收到rollback操作后，之前由于网络原因阻塞的commit操作到达RM，如果RM执行这个commit，则会产生数据不一致的异常。
- 异常点3：由于重试策略，导致TC向RM发送了多于1次的commit或者rollback请求



所以，为了避免上面异常情况，对于commit和rollback操作，需要进行如下检查

- RM收到rollback操作， 检查是否有commit记录
  - 如果有记录：如果没有执行过rollback，则执行rollback操作；执行过rollback则直接返回rollback成功
  - 如果没有记录：将此rollback操作记录下来，表示此子事务需要回滚
- RM收到commit操作，查看此子事务是否有rollback记录，
  - 如果有rollback记录，则不执行commit；
  - 如果没有rollback记录，且此子事务没有执行过commit操作，则执行commit操作

RM在执行commit和rollback操作，事务的状态更改需要保证原子性和隔离性，最好依赖数据库的事务来实现。


---------------


## 开发示例

以公司给员工发薪水作为示例，演示如何通过SDK来实现一个saga事务。
此演示使用了SDK的高级API，屏蔽了各种异常情况的处理，提高了开发的效率。

[开发示例](README_saga_demo.md)



## client SDK

client sdk实现了AP和RM的sdk封装，方便进行saga开发。

[client SDK](README_saga_sdk.md)



## API 设计

[API设计](README_saga_api.md)



