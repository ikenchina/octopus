
# SQL


Create account table on testdtx database.

```
CREATE SCHEMA testdtx;

CREATE TABLE IF NOT ExISTS testdtx.account(
	id BIGSERIAL PRIMARY key,
	balance INT NOT NULL DEFAULT 0,
	balance_freeze INT NOT NULL DEFAULT 0
);
```

根据需要，创建相关的测试用户，
```
insert into testdtx.account(id) select * from generate_series(-10, 10);
```
