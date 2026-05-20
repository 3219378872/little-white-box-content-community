package config

import "mqx"

type Config struct {
	MQ     mqx.ConsumerConfig
	Milvus MilvusConfig
}

type MilvusConfig struct {
	Address    string `json:",optional"`
	Collection string `json:",default=xbh_post_embeddings"`
	Dim        int    `json:",default=256"`
	Username   string `json:",optional"`
	Password   string `json:",optional"`
	Database   string `json:",optional"`
}
