graph TB
subgraph 数据源层
A[内容生产端<br/>UGC / PGC / 爬虫]
B[用户行为端<br/>点击 / 收藏 / 评论 / 搜索]
end

    subgraph 数据接入层
        C[消息队列<br/>Kafka / Pulsar]
        D[日志采集<br/>Fluentd / Filebeat]
    end

    subgraph 数据存储层
        E[(关系数据库<br/>MySQL / PostgreSQL<br/>内容元数据 / 用户画像)]
        F[(向量数据库<br/>Milvus<br/>内容 Embedding)]
        G[(对象存储<br/>S3 / OSS<br/>图片 / 视频 / 富媒体)]
        H[(分析引擎<br/>ClickHouse / Doris<br/>行为事件 / 时序数据)]
        I[(缓存层<br/>Redis / Valkey<br/>热数据 / 实时特征)]
    end

    subgraph 计算层
        J[离线计算<br/>Spark / Flink Batch<br/>全量索引 / 模型训练样本]
        K[实时计算<br/>Flink Stream<br/>实时特征 / 索引增量更新]
    end

    subgraph 服务层
        L[搜索服务<br/>文本检索 + 向量召回]
        M[推荐服务<br/>多路召回 + 精排引擎]
    end

    A -->|内容发布| C
    B -->|行为事件| D
    B -->|实时反馈| C

    C -->|实时流| K
    D -->|批量导入| H

    K -->|写| E
    K -->|更新| F
    K -->|更新| I

    E -->|批量同步| J
    H -->|行为样本| J
    J -->|重建| F

    E -->|元数据| L
    F -->|向量召回| L
    F -->|相似内容| M
    I -->|用户特征| M
    H -->|统计特征| M

    G -.->|富媒体 URL| L
    G -.->|内容资源| M