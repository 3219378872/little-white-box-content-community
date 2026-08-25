# SQL 资产约定

本目录有两类文件，交付路径不同：

## 基线 schema（`xbh_*.sql`）

由 MySQL 容器挂载为 `/docker-entrypoint-initdb.d`，**只在空数据卷首次初始化时按文件名字典序
执行**。只允许 `CREATE TABLE IF NOT EXISTS` 级别的全量定义；不要把针对已有表的
`ALTER`/`UPDATE` 放在这里——存量卷不会重放它们，新卷上又可能先于依赖的表执行。

## 幂等补丁（`patches/*.sql`）

不进入 initdb.d 挂载（子目录不会被官方 entrypoint 递归执行），由本地编排
（根仓 stack.sh `apply_sql_patches`）在每次 `middleware-up` 时对现有数据卷逐个重放。
因此每个补丁必须自身幂等，推荐两种写法之一：

- 存在性守卫：用 `information_schema` 判断列/索引是否存在，再经预处理语句执行 DDL；
- 数据守卫：`UPDATE ... WHERE <旧缺陷特征>`，重复执行影响 0 行。

补丁合并进 patches/ 后即视为可对任意环境重复执行；不要再依赖「手工跑一次」。
ClickHouse 侧无此拆分：`xbh_analytics.sql` 全文幂等，直接整体重放。
