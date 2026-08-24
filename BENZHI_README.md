# MiniDataLake 评测说明

本项目是基于Go语言的面向开发者的流式轻量级数据湖仓分析平台（Mini DuckDB + Web SQL Workspace），旨在解决用户在前端上传几十兆的 CSV、Parquet 或 JSON 文件，后端利用 Go 进行高性能列式分析问题，使用了Go、React、Monaco Editor，功能有湖仓目录树、大数据库渲染表格、列式存储/内存解析引擎、SQL 解析与执行核心。

Go 模块位于 `backend/`。评测入口：在该目录执行 `go test ./...`。
